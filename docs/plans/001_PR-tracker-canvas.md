# PR tracker canvas

date: 2026-08-02
status: draft

The "PR tracker canvas": a Slack canvas this action keeps updated with a live view of open + WIP PRs. Use this name in user-facing text (input description, README).

## Goals

- Motivation: the reminder message is transient and drafts-free by design, and even GitHub itself has no single "live" view of a team's open + WIP PRs (incl. their statuses/staleness/reviews) across multiple repos — the canvas fills both gaps with a persistent, on-demand, cross-repo view in Slack.
- Optional feature: keep a Slack canvas continuously showing open PRs (oldest first) and draft/WIP PRs (most recent activity first).
- Canvas always includes draft PRs (the main reminder message never does — unchanged).
- Open PR rows render exactly like the reminder message's rows.
- WIP rows show last-push activity instead of age: drafts are ordered by activity, as the last push predicts which draft gets opened for review next better than its age does.
- Draft PRs inactive for more than 2 months are excluded by default.
- One new input, off by default, no other new configurability.

### Canvas content format

**Open PRs**

- **Update CI workflow for faster Go builds** 🚨 `4 days old` by @José (✅ yzma, chaca)
- **Revise onboarding guide in README** _2 days ago_ by @PR bot (✅ kronk)
- **Implement feature flag system for UI** _19 hours ago_ by @José (✅ hellej / 💬 alice, bob)

**Work in Progress**

- **Spike: replace mux with chi** by @José `updated 2 hours ago`
- **Refactor state store** by @kronk (💬 alice) `idle 3 days` 💤
- **Prototype canvas rendering** by @alice `idle 12 days` 💤

A WIP row is: linked title, author, reviewers, activity chip, then 💤 if idle for +48h. Chip wording: `updated N minutes/hours ago` under 24h, `idle N days` at 24h and above. Never the 🚨 old-PR marker — it nags about review latency, which doesn't apply to work nobody has been asked to review yet.

"updated" is reader-facing wording only: the chip is backed by the head commit's committer date (Step 2), not GitHub's `updated_at`, which also moves on comments and labels. Don't "fix" the mismatch by switching the data source. `updated_at` serves only as a fallback when the head commit can't be fetched (Step 2).

## Non-goals

- Configurable thresholds (activity windows, draft staleness) — hardcoded for now.
- Auto-creating or discovering a canvas — the user creates it themselves in Slack and provides its ID; the action never creates, deletes, or looks one up by channel.
- Linking to the canvas from the reminder message — there's no scope-free, officially documented way to get a stable canvas URL (see Step 6); skipped for v1.
- Splitting canvas content if it exceeds Slack's canvas size limits.
- Persisting canvas identity in `state` — the ID is supplied fresh via input every run, nothing to persist.
- A "no PRs" message input for the canvas (a fixed fallback string is used instead).

## Target shape

- `action.yml`: new optional input `pr-tracker-canvas-id` (string, no default). Empty/unset → feature off, matching the "empty means unused" requirement literally. The user creates and owns the canvas entirely themselves (see Non-goals).
- `githubclient`: fetching gains an explicit `PRFetchOptions{IncludeDrafts, FetchActivityTimestamps}` struct (zero value = today's behavior exactly). When set, drafts survive the fetch and each PR gets a `LastActivityAt *time.Time` from its head commit (one extra GitHub API call per PR — see Step 2).
- `prparser`: `PR` gains activity display helpers (based on `LastActivityAt`) and a most-recent-activity sort, used only by the draft section.
- New `canvascontent` package (mirrors `messagecontent`; Go code stays medium-named, `canvas*`, no need to spell out "PR" internally): builds a canvas-ready `Content` — open PRs (oldest first, grouped/flat per `group-by-repository`) + draft PRs (always flat regardless of that input, most-recent-activity first, >2 months inactive excluded).
- New `canvasbuilder` package (mirrors `messagebuilder`): renders `canvascontent.Content` to Slack canvas markdown, reusing display-text helpers extracted from `messagebuilder` in the pre-refactor.
- `slackclient`: gains a method that fully replaces a canvas's content by ID — one `canvases.edit` call, `replace` operation, `section_id` omitted.
- `run.go`: after the existing post/update logic, if `pr-tracker-canvas-id` is set, runs one independent full PR fetch (open + draft, both run modes) and overwrites the canvas. Failures are logged as warnings, not fatal — same pattern already used for `DeleteMessage` failures in `run.go` — so a canvas hiccup can't take down the core reminder message.
- Permissions:
  - Slack: one new scope, `canvases:write` (canvas-only). On top of it, a one-time manual step — the user shares the canvas with the bot (see Step 6), since the scope alone doesn't grant write access to a specific canvas.
  - GitHub: no change. The activity lookup runs on `pull-requests: read`, already required.

## Breaking change classification

Non-breaking / **minor** release. New optional input, default off, no change to existing inputs, outputs, or default behavior.

## Summary of steps

- R1. Refactor: make draft-exclusion in `githubclient` explicit and optional instead of hardcoded
- R2. Refactor: extract repository-grouping into a shared, reusable helper (closes a test-coverage gap)
- R3. Refactor: extract medium-agnostic PR display text out of `messagebuilder` (closes two test-coverage gaps)
1. `action.yml` + `config`: add the new input
2. `githubclient`: add last-activity (head commit) lookup
3. `prparser`: add activity text + most-recent-activity sort
4. `canvascontent`: build canvas content
5. `canvasbuilder`: render canvas content to markdown
6. `slackclient`: fully replace canvas content by ID
7. `run.go`: wire up the canvas refresh
8. Docs & permissions: README, `pr-reminder.yml`, `e2e-tests/action.yml`
9. Spec sync

## Steps

### R1. Refactor: explicit, optional draft-exclusion (`internal/apiclients/githubclient`)

- Add `PRFetchOptions{IncludeDrafts bool}` and thread it through `Client.FindOpenPRs`, `Client.GetPRs`, and `getPRFilterFunc` (replacing the hardcoded `!result.pr.GetDraft()`).
- Update the two call sites in `run.go` to pass `PRFetchOptions{}` (zero value) — behavior must stay byte-for-byte identical to today.
- Test coverage check: already good — `githubclient_test.go` has an explicit "draft PR should be filtered out" case. No gap here; existing tests must pass unchanged, and add one table-driven case that `IncludeDrafts: true` lets a draft PR through.

### R2. Refactor: shared repository-grouping helper

- Extract `messagecontent.groupPRsByRepositories` (bucket-by-repo-path, alphabetical order, GitHub pulls-page link) into an exported helper on `prparser` (operates on `[]prparser.PR`, returns per-repository groups), since `canvascontent` will need identical grouping for its open-PR section.
- Test coverage gap found: there is no `messagecontent_test.go` at all — grouping is only exercised indirectly through `messagebuilder_test.go`'s fixed two-repository example and `main_test.go`'s integration cases. Add a direct unit test for the extracted helper (alphabetical ordering with out-of-order input, link format, single- and multi-repository bucketing) before/while moving it, so the extraction has a real safety net instead of only indirect coverage.
- `messagecontent` calls the extracted helper; behavior and existing tests unchanged.

### R3. Refactor: shared PR display text

- Extract from `messagebuilder` into methods on `prparser.PR` (or a small shared helper file), as plain strings/booleans instead of `slack.RichTextSectionElement`s:
  - reviewers summary text (✅/💬 grouping of approvers/commenters)
  - age text, including the old-PR warning marker vs. plain "N ago" — used by open PR rows only; the WIP section must not call it (see Goals)
  - closed-but-not-merged / merged markers
- Author display is not extracted: `pr.Author.SlackUserID` and `pr.Author.GetGitHubName()` are already exported, and the mention format itself differs per medium (`<@ID>` for Block Kit, `![](@ID)` for canvas markdown — see Step 5), so each builder wraps them directly.
- Test coverage gaps found in `messagebuilder_test.go`, both worth closing before extracting so a regression in either path is actually caught:
  - the old-PR warning-marker path (`IsOldPR: true` → 🚨 + bold/code age text) has no test at all today
  - the author-fallback path (no mapped `SlackUserID` → GitHub display name instead of a mention) has no test at all today
- `messagebuilder` wraps the extracted plain strings in its Block Kit elements; behavior and existing tests unchanged. This lets `canvasbuilder` (Step 5) reuse the exact same text instead of re-deriving it.

### 1. New input: `pr-tracker-canvas-id`

- `action.yml`: add optional string input, no default. Description states the intent plainly, e.g. "ID of a Slack canvas to keep updated with a live tracker of open and work-in-progress pull requests. Leave empty to disable (default)."
- `internal/config/config.go`: add constant `InputPRTrackerCanvasID`, parse via `inputhelpers.GetInput`, add `Config.PRTrackerCanvasID string`.
- `testhelpers/confighelpers.go`: mirror the new input.
- `go run .github/scripts/check_inputs.go` must still pass.

### 2. `githubclient`: last-activity lookup

- Add `ListCommits(ctx, owner, repo string, number int, opts *github.ListOptions) ([]*github.RepositoryCommit, *github.Response, error)` to the existing `GithubPullRequestsService` interface — matches `(*github.PullRequestsService).ListCommits` in `go-github/v78` exactly. No new service to wire in: `client`, `NewClient` and `GetAuthenticatedClient` are unchanged.
- No new GitHub permission: [the endpoint's docs](https://docs.github.com/en/rest/pulls/pulls?apiVersion=2022-11-28#list-commits-on-a-pull-request) state it needs `"Pull requests" repository permissions (read)`, which the action already requires.
- Add `PRFetchOptions.FetchActivityTimestamps bool`. When set, `addReviewerInfoToPRs`'s per-PR fan-out gets a 4th concurrent call alongside the existing three (reviews, comments, timeline comments): resolve the PR's newest commit and record its `commit.committer.date` — the true last-push time, unlike `updated_at` (which also changes on comments, labels, etc.). Add the resolved timestamp to `models.go`'s existing per-PR "Found %d reviews, %d PR comments…" line rather than logging a new one per PR.
- Resolution, one call per PR:
  - `ListCommits` with `ListOptions{PerPage: 100}`. If `response.NextPage != 0` (over 100 commits), refetch at `response.LastPage`.
  - Take that page's last element: the endpoint returns commits oldest first, head commit last (verified on PRs of 2-79 commits across four repositories).
  - Confirm its SHA equals `pr.Head.SHA`, since the order isn't documented and the endpoint caps at 250 commits (above which the head commit is never returned).
  - Fall back to `pr.GetUpdatedAt()`, already on the fetched PR, whenever the head commit can't be pinned down: SHA mismatch, a failed or empty `ListCommits` call. It overstates freshness, but it's an upper bound on the last push, so a busy long-lived draft never drops off the canvas via the staleness rule. Log a warning naming the repository, PR number and reason, then carry on — the existing "partial failure" pattern for reviews/comments (`models.go`'s "Unable to fetch reviews/comments for PR #%d"), never failing the whole PR.
- Add `PR.LastActivityAt *time.Time`, nil only when `FetchActivityTimestamps` is off.
- Extend `testhelpers/mockgithubclient`'s `mockPullRequestService` with `ListCommits`.

### 3. `prparser`: activity text + activity sort

- Add `PR.GetActivityText() string`, derived from `LastActivityAt`, reusing `GetPRAgeText`'s minutes/hours/days magnitude: `updated N minutes/hours ago` under 24h, `idle N days` at 24h and above. Empty string if `LastActivityAt` is nil.
- Add `PR.IsIdle() bool`: true when `LastActivityAt` is older than a hardcoded 48h. False if `LastActivityAt` is nil.
- Add an exported sort (most-recent-activity first) for use by the draft section, distinct from `ParsePRs`'s existing oldest-first sort.

### 4. `canvascontent` package

- Mirrors `messagecontent.GetContent`'s shape but for canvas: takes all fetched PRs (open + draft) and content inputs, and produces open-PR content (oldest first, grouped/flat per `group-by-repository`, via the R2 helper) plus draft-PR content (always flat, most-recent-activity first, regardless of `group-by-repository`).
- Excludes draft PRs whose `LastActivityAt` (not creation time) is older than a hardcoded `MaxDraftPRInactivity` (2 months / 60 days).
- Fixed fallback text for the "nothing to show" case.
- Log the counts put on the canvas: open PRs, drafts, and drafts dropped as inactive — otherwise a missing draft has no explanation.

### 5. `canvasbuilder` package

- Renders `canvascontent.Content` to a Slack canvas markdown string (`slack.DocumentContent{Type: "markdown", ...}`), reusing the R3 display-text helpers.
- Canvas `document_content` takes real markdown, not Slack `mrkdwn`: `**bold**`, `[label](url)`, `~~strike~~`, backtick code spans, `#`-`###` headings, `-` bullets — all confirmed supported ([Canvases docs](https://docs.slack.dev/surfaces/canvases/)). User mentions use canvas-specific syntax, `![](@USERID)`, not Block Kit's `<@ID>`; `canvasbuilder` wraps `pr.Author.SlackUserID`/`GetGitHubName()` in this format itself (see R3).
- Open PR rows: identical fields to the message — linked title (struck through if closed-but-not-merged), age text with the old-PR warning marker, author, reviewers, merged marker.
- WIP rows: linked title, author, reviewers, `PR.GetActivityText()` as a code span, then 💤 if `PR.IsIdle()`. No age text, no 🚨, no 🚀 (see Goals).
- Structure: open-PR heading/list (grouped or flat, matching the message's style) followed by a "Work in Progress" heading and the draft list.

### 6. `slackclient`: full canvas content replace

- Add `EditCanvas` to `SlackAPI` (exists in `github.com/slack-go/slack` v0.27.0 — confirmed in source).
- Add `Client.ReplaceCanvasContent(canvasID, markdown string) error`: one `EditCanvas` call (`EditCanvasParams{CanvasID, Changes: []CanvasChange{{Operation: "replace", DocumentContent: ...}}}` — confirmed struct shape in `slack-go/slack` v0.27.0's `canvas.go`), `SectionID` left at its zero value (`""`, omitted on the wire via `omitempty`), `DocumentContent{Type: "markdown", Markdown: markdown}`. Confirmed via [Slack's `canvases.edit` docs](https://docs.slack.dev/reference/methods/canvases.edit/#content-operations): omitting `section_id` on a `replace` operation replaces the entire canvas in one call, and the method needs only the `canvases:write` scope.
- Add mock support in `testhelpers/mockslackclient`.
- Log the canvas ID and markdown length before the call, confirm on success — matching `SendMessage`/`UpdateMessage`. The length is the only clue if content hits Slack's canvas size limit.
- **Scope alone isn't sufficient**: canvases have their own access control (read/write/owner), separate from OAuth scopes ([`canvases.access.set` docs](https://docs.slack.dev/reference/methods/canvases.access.set)). Since the user creates the canvas as themselves, the bot has no access to it by default and `canvases.edit` will fail until the user explicitly shares the canvas with the bot (e.g. sharing it into a channel the bot's already a member of). Document this as a required manual setup step (Step 8) — it can't be automated by the action itself.

### 7. `run.go`: wire up the canvas refresh

- `Run()` currently returns directly from inside the `RunMode` switch (`return runPostMode(...)` / `return runUpdateMode(...)`), so there's no code path "after" it today. Restructure to capture the switch's result in an `err` variable; if `err != nil`, return it unchanged (unaffected by this feature); otherwise, if `cfg.PRTrackerCanvasID != ""`, run the canvas refresh step before returning `nil`. This means the canvas only refreshes after a successful post/update — a failed primary run keeps failing the same way it does today, and doesn't attempt a canvas write.
- Canvas refresh step: `FindOpenPRs(ctx, cfg.Repositories, cfg.GetFiltersForRepository, PRFetchOptions{IncludeDrafts: true, FetchActivityTimestamps: true})`, then `canvascontent`/`canvasbuilder`/`slackclient.ReplaceCanvasContent`.
- This always does a fresh full fetch, independent of `update` mode's targeted `GetPRs` (state-tracked refs) — the canvas reflects current reality, not a previously-sent PR set.
- Log and continue (don't fail the run) if this step errors. The warning names the canvas-sharing requirement (Step 6) as the likely cause — the run still exits 0, so it's the user's only signal.

### 8. Docs & permissions

- README: new "📋 PR Tracker Canvas" section explaining what it is and how to set it up — create a canvas in Slack, share it with the bot (required, see Step 6), copy its ID into `pr-tracker-canvas-id`. Add `canvases:write` to the Slack scope table (canvas-only). The GitHub permissions block stays as is.
- Document the new input in the inputs table.
- Exercise both the on and off paths end to end. No workflow-permission change needed.
  - `.github/workflows/pr-reminder.yml`: set `pr-tracker-canvas-id`, giving the scheduled runs a real, continuously refreshed canvas in both `post` and `update` mode.
  - `.github/actions/e2e-tests/action.yml`: set it on the "Run with filters" step only — the richest case (multi-repository, `group-by-repository: true`, filters), covering grouped open PRs next to the always-flat draft list.
  - Leave it unset on the "Basic run" and "Multi-repository run" steps, so every release also verifies the action still behaves exactly as before when the input is absent.
  - The ID goes in as a plain literal, like `slack-channel-name` — it isn't a secret.
- Manual prerequisites, not automatable here: grant `canvases:write` to the Slack app/bot token these workflows use, then create the canvas and share it with the bot to get an ID.

### 9. Spec sync

- Update `githubclient.spec.md`, `slackclient.spec.md`, `prparser.spec.md`, `config.spec.md` for the changes above.
- Add `canvascontent.spec.md` and `canvasbuilder.spec.md`.

## Consequences

### Positive

- An always-current PR/WIP view lives directly in Slack, with no need to open GitHub.
- Draft/WIP visibility, absent from the main reminder message by design, becomes available to whoever wants it.
- Fully additive and off by default: existing users see zero behavior change (zero-value fetch options, early-exit in `run.go`).
- The R1-R3 refactors close pre-existing test-coverage gaps (draft filter, repository grouping, old-PR/author-fallback display paths) independent of whether the canvas feature itself is used.

### Negative

- Opted-in runs make one extra GitHub API call per PR (commit lookup for activity), adding to rate-limit consumption.
- Canvas access can't be granted by the action itself — the user must manually share the canvas with the bot in Slack, a step that's easy to miss and only surfaces as a silent warning log on failure.
- Two new packages (`canvascontent`, `canvasbuilder`) largely mirror existing ones (`messagecontent`, `messagebuilder`), adding maintenance surface for a feature many users won't enable.

### Neutral

- No canvas URL is linked from the reminder message; users find the canvas in Slack themselves (see Non-goals).
- Activity and draft-staleness thresholds are hardcoded, not configurable, in v1.
