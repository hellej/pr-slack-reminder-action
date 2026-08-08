# PR tracker canvas

date: 2026-08-02
status: draft

The "PR tracker canvas": a Slack canvas this action keeps updated with a live view of open + WIP PRs. Use this name in user-facing text (input description, README).

## Goals

- Motivation: the reminder message is transient and drafts-free by design, and even GitHub itself has no single "live" view of a team's open + WIP PRs (incl. their statuses/staleness/reviews) across multiple repos — the canvas fills both gaps with a persistent, on-demand, cross-repo view in Slack.
- New optional feature: keep a Slack canvas continuously showing open PRs (oldest first) and draft/WIP PRs (most recent activity first).
- Canvas always includes draft PRs (the main reminder message never does — unchanged).
- Open PR rows render exactly like the reminder message's rows.
- WIP rows show last-push activity instead of age: drafts are ordered by activity, as the last push predicts which draft gets opened for review next better than its age does.
- Draft PRs inactive for more than 2 months are excluded by default.
- One new input, off by default, no other new configurability.
- The input takes the canvas link the user copies from Slack; the action parses the ID out of it.
- The reminder message links to the canvas, so the transient message is a way in to the persistent view.

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
- Auto-creating or discovering a canvas — the user creates it themselves in Slack and pastes its link; the action never creates, deletes, or looks one up by channel.
- Splitting canvas content if it exceeds Slack's canvas size limits.
- Persisting canvas identity in `state` — the ID is supplied fresh via input every run, nothing to persist.
- A "no PRs" message input for the canvas (a fixed fallback string is used instead).
- Closed PRs on the canvas, struck through or otherwise — the canvas refresh fetches open PRs only. Deferred, not rejected: Steps 3-5 keep the seams for a third section (see "Room for a recently-closed section").

### Room for a recently-closed section

A future third canvas section listing recently closed/merged PRs stays additive. What it would need, and what this plan already covers:

- `githubclient`: a new list path — `PullRequestListOptions{State: "closed", Sort: "updated", Direction: "desc"}`, stopping at a time window. This is genuinely new code, not a `PRFetchOptions` flag: open PRs are bounded by "still open", closed PRs need a cutoff. Inherent to the feature; nothing here makes it worse. `FetchActivityTimestamps` stays off for it — `ClosedAt`/`MergedAt` are already on the fetched PR.
- `prparser`: reuses Step 3's keyed newest-first sort with `ClosedAt`.
- `canvascontent`/`canvasbuilder`: one more section field and one more `renderSection` call (Steps 4-5).
- Row markers: `canvasbuilder` reads the existing `PR.IsMerged()`/`PR.IsClosedButNotMerged()` booleans. R3 leaving the message's strike-through and 🚀 rendering in `messagebuilder` costs nothing here — only the marker *elements* are Block Kit-specific, not the conditions.
- `run.go`: the canvas refresh already does its own fetch, so a second one is contained there.

## Target shape

- `action.yml`: new optional input `pr-tracker-canvas-link` (string, no default). Empty/unset → feature off, matching the "empty means unused" requirement literally. The user creates and owns the canvas entirely themselves (see Non-goals).
- `config`: parses the link once into a canvas ID + canvas URL, so no other package parses it again. Both are always known when the feature is on.
- `githubclient`: fetching gains an explicit `PRFetchOptions{IncludeDrafts, FetchActivityTimestamps}` struct (zero value = today's behavior exactly). When set, drafts survive the fetch and each PR gets a `LastActivityAt *time.Time` from its head commit (one extra GitHub API call per PR — see Step 2).
- `prparser`: `PR` gains activity display helpers (based on `LastActivityAt`) and a most-recent-activity sort, used only by the draft section.
- New `canvascontent` package (mirrors `messagecontent`; Go code stays medium-named, `canvas*`, no need to spell out "PR" internally): builds a canvas-ready `Content` — open PRs (oldest first, grouped/flat per `group-by-repository`) + draft PRs (always flat regardless of that input, most-recent-activity first, >2 months inactive excluded).
- New `canvasbuilder` package (mirrors `messagebuilder`): renders `canvascontent.Content` to Slack canvas markdown, reusing display-text helpers extracted from `messagebuilder` in the pre-refactor.
- `messagecontent`/`messagebuilder`: the reminder message gains a trailing canvas link row whenever the feature is on.
- `slackclient`: gains a method that fully replaces a canvas's content by ID — one `canvases.edit` call, `replace` operation, `section_id` omitted.
- `run.go`: after the existing post/update logic, if `pr-tracker-canvas-link` is set, runs one independent full PR fetch (open + draft, both run modes) and overwrites the canvas. Failures are logged as warnings, not fatal — same pattern already used for `DeleteMessage` failures in `run.go` — so a canvas hiccup can't take down the core reminder message.
- Permissions:
  - Slack: one new scope, `canvases:write` (canvas-only). On top of it, a one-time manual step — the user creates the canvas as a tab in the reminder channel (see Step 6), since the scope alone doesn't grant write access to a specific canvas.
  - GitHub: no change. The activity lookup runs on `pull-requests: read`, already required.

## Breaking change classification

Non-breaking / **minor** release. New optional input, default off, no change to existing inputs, outputs, or default behavior.

## Summary of steps

- R1. Refactor: make draft-exclusion in `githubclient` explicit and optional instead of hardcoded
- R2. Refactor: extract repository-grouping into a shared, reusable helper (closes a test-coverage gap)
- R3. Refactor: extract medium-agnostic PR display text out of `messagebuilder` (closes two test-coverage gaps)
1. `action.yml` + `config`: add the new input, parse link → ID + URL
2. `githubclient`: add last-activity (head commit) lookup
3. `prparser`: add activity text + most-recent-activity sort
4. `canvascontent`: build canvas content
5. `canvasbuilder`: render canvas content to markdown
6. `slackclient`: fully replace canvas content by ID
7. `run.go`: wire up the canvas refresh
8. Reminder message: link to the canvas
9. Docs & permissions: README, `pr-reminder.yml`, `e2e-tests/action.yml`
10. Spec sync

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

  The closed-but-not-merged and merged markers stay in `messagebuilder`: only the message ever renders a closed or merged PR (see Step 5).
- Author display is not extracted: `pr.Author.SlackUserID` and `pr.Author.GetGitHubName()` are already exported, and the mention format itself differs per medium (`<@ID>` for Block Kit, `![](@ID)` for canvas markdown — see Step 5), so each builder wraps them directly.
- Test coverage gaps found in `messagebuilder_test.go`, both worth closing before extracting so a regression in either path is actually caught:
  - the old-PR warning-marker path (`IsOldPR: true` → 🚨 + bold/code age text) has no test at all today
  - the author-fallback path (no mapped `SlackUserID` → GitHub display name instead of a mention) has no test at all today
- `messagebuilder` wraps the extracted plain strings in its Block Kit elements; behavior and existing tests unchanged. This lets `canvasbuilder` (Step 5) reuse the exact same text instead of re-deriving it.

### 1. New input: `pr-tracker-canvas-link`

- `action.yml`: add optional string input, no default. Description states the intent plainly, e.g. "Link to a Slack canvas to keep updated with a live tracker of open and work-in-progress pull requests. Open the canvas in Slack → ⋮ → Copy link. Leave empty to disable (default)."
- `internal/config/config.go`: add constant `InputPRTrackerCanvasLink`, read via `inputhelpers.GetInput`, add `Config.PRTrackerCanvasID` and `Config.PRTrackerCanvasURL`, both empty when the input is unset.
- Link shape, confirmed against a real channel canvas: `https://<workspace>.slack.com/docs/<TEAM_ID>/<CANVAS_ID>`, e.g. `https://hellej.slack.com/docs/T08SLKDVNB2B/F0BMEBEXVR1DL`.
- Parse: `url.Parse`, then the last non-empty path segment, which must match `^F[A-Z0-9]+$`. No length bound — real IDs already run to 13 characters. Reading `URL.Path` drops query params for free.
  - `PRTrackerCanvasURL` is the input string as given, never rebuilt.
  - A non-empty input that doesn't parse is a hard config error, joined like the other input validation. A typo would otherwise mean a silently missing canvas. The message names the expected shape and the Copy link path.
- Test cases: the real link above, trailing slash, query params, `http`, an unrelated URL, a bare `F…` ID, empty (feature off).
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
- Add an exported newest-first sort for use by the draft section, distinct from `ParsePRs`'s existing oldest-first sort. Take the timestamp as a key function (`func(PR) *time.Time`) rather than reading `LastActivityAt` directly, so sorting by any other timestamp needs no second sort.

### 4. `canvascontent` package

- Mirrors `messagecontent.GetContent`'s shape but for canvas: takes all fetched PRs (open + draft) and content inputs, and produces open-PR content (oldest first, grouped/flat per `group-by-repository`, via the R2 helper) plus draft-PR content (always flat, most-recent-activity first, regardless of `group-by-repository`).
- Keep the two sections as separate named fields on `Content`, not one merged list with a per-PR kind flag, so a third section is a field rather than a rework.
- Excludes draft PRs whose `LastActivityAt` (not creation time) is older than a hardcoded `MaxDraftPRInactivity` (2 months / 60 days).
- Fixed fallback text for the "nothing to show" case.
- Log the counts put on the canvas: open PRs, drafts, and drafts dropped as inactive — otherwise a missing draft has no explanation.

### 5. `canvasbuilder` package

- Renders `canvascontent.Content` to a Slack canvas markdown string (`slack.DocumentContent{Type: "markdown", ...}`), reusing the R3 display-text helpers.
- Canvas `document_content` takes real markdown, not Slack `mrkdwn`: `**bold**`, `[label](url)`, `~~strike~~`, backtick code spans, `#`-`###` headings, `-` bullets — all confirmed supported ([Canvases docs](https://docs.slack.dev/surfaces/canvases/)). User mentions use canvas-specific syntax, `![](@USERID)`, not Block Kit's `<@ID>`; `canvasbuilder` wraps `pr.Author.SlackUserID`/`GetGitHubName()` in this format itself (see R3).
- Open PR rows: linked title, age text with the old-PR warning marker, author, reviewers. No strike-through, no 🚀 — the canvas fetch lists open PRs only, so a closed or merged PR can never reach a row here (see Non-goals). Those two markers stay message-only.
- WIP rows: linked title, author, reviewers, `PR.GetActivityText()` as a code span, then 💤 if `PR.IsIdle()`. No age text, no 🚨, no 🚀 (see Goals).
- Structure: open-PR heading/list (grouped or flat, matching the message's style) followed by a "Work in Progress" heading and the draft list.
- Render both through one internal `renderSection(heading string, prs []prparser.PR, renderRow func(prparser.PR) string)`, not two bespoke paths. The two sections differ only in their row renderer, and a third section then costs one call.

### 6. `slackclient`: full canvas content replace

- Add `EditCanvas` to `SlackAPI` (exists in `github.com/slack-go/slack` v0.27.0 — confirmed in source).
- Add `Client.ReplaceCanvasContent(canvasID, markdown string) error`: one `EditCanvas` call (`EditCanvasParams{CanvasID, Changes: []CanvasChange{{Operation: "replace", DocumentContent: ...}}}` — confirmed struct shape in `slack-go/slack` v0.27.0's `canvas.go`), `SectionID` left at its zero value (`""`, omitted on the wire via `omitempty`), `DocumentContent{Type: "markdown", Markdown: markdown}`. Confirmed via [Slack's `canvases.edit` docs](https://docs.slack.dev/reference/methods/canvases.edit/#content-operations): omitting `section_id` on a `replace` operation replaces the entire canvas in one call, and the method needs only the `canvases:write` scope.
- Add mock support in `testhelpers/mockslackclient`.
- Log the canvas ID and markdown length before the call, confirm on success — matching `SendMessage`/`UpdateMessage`. The length is the only clue if content hits Slack's canvas size limit.
- On error, wrap it with a concise hint: `"canvas update failed: check that the bot has canvases:write permission and is invited to the channel where the canvas is"` — the only actionable feedback the user gets, since `run.go` only logs this as a warning (Step 7).
- **Scope alone isn't sufficient**: canvases have their own access control (read/write/owner), separate from OAuth scopes ([`canvases.access.set` docs](https://docs.slack.dev/reference/methods/canvases.access.set)). The user creates the canvas as themselves, so the bot has no access by default and `canvases.edit` fails until the canvas is shared with it.
- **Intended setup**: create the canvas as a tab in the reminder channel itself. A channel canvas needs no sharing step — per [`conversations.canvases.create` docs](https://docs.slack.dev/reference/methods/conversations.canvases.create/), "there are no access implications nor is it necessary to share a channel canvas to grant access. Access is tied to channel access", and [Slack's help article](https://slack.com/help/articles/21290478840979-Feature-change-notice--Channel-canvases) sets edit access by "a member's permission to post in the channel". The bot already posts the reminder there, so it clears that bar by construction. Neither source names apps explicitly — if `canvases.edit` returns `access_denied`/`canvas_not_found` on the first run, fall back to sharing the canvas with the bot.
- Also in favour of a channel canvas: `canvases.edit` lists `free_teams_cannot_edit_standalone_canvases`, so standalone canvases are a dead end on free workspaces.
- Document the channel-tab path as *the* setup (Step 9), not one option among several. It can't be automated by the action either way.

### 7. `run.go`: wire up the canvas refresh

- `Run()` currently returns directly from inside the `RunMode` switch (`return runPostMode(...)` / `return runUpdateMode(...)`), so there's no code path "after" it today. Restructure to capture the switch's result in an `err` variable; if `err != nil`, return it unchanged (unaffected by this feature); otherwise, if `cfg.PRTrackerCanvasID != ""`, run the canvas refresh step before returning `nil`. This means the canvas only refreshes after a successful post/update — a failed primary run keeps failing the same way it does today, and doesn't attempt a canvas write.
- Canvas refresh step: `FindOpenPRs(ctx, cfg.Repositories, cfg.GetFiltersForRepository, PRFetchOptions{IncludeDrafts: true, FetchActivityTimestamps: true})`, then `canvascontent`/`canvasbuilder`/`slackclient.ReplaceCanvasContent`.
- This always does a fresh full fetch, independent of `update` mode's targeted `GetPRs` (state-tracked refs) — the canvas reflects current reality, not a previously-sent PR set.
- Log and continue (don't fail the run) if this step errors, carrying the hint from Step 6 — the run still exits 0, so this warning is the user's only signal.

### 8. Reminder message: link to the canvas

- `messagecontent.Content` gains `CanvasURL string`, filled from `cfg.PRTrackerCanvasURL`. Empty → nothing changes anywhere, so existing tests stay untouched.
- `messagebuilder.BuildMessage` appends a trailing context block, `<URL|📋 PR tracker canvas>` — a context block, not rich text, so it reads as a subdued footer rather than another list row. The label matches the feature name in the README.
- Append it **after** `limitMaximumMessageSize`, not before: that function truncates at 50 blocks, and a link appended earlier would be the first thing dropped on a large message. Bump the constant's budget by one in its comment.
- Add it on the no-PRs path too. That path is exactly when the link earns its place — no open PRs to report, but the canvas may still hold WIP work.
- The link is rendered optimistically, before the canvas refresh runs (Step 7). It stays correct either way: the canvas is the user's and exists whether or not our write succeeded.
- No preview card: add `slack.MsgOptionDisableLinkUnfurl()` and `slack.MsgOptionDisableMediaUnfurl()` to `PostMessage` and `UpdateMessage` in `slackclient`, neither of which sets an unfurl option today. A `slack.com` link is exactly what Slack expands into a card, and the card would dwarf the one-line footer. Set both: bot tokens default `unfurl_links` to false but `unfurl_media` to true.
- Existing messages are unaffected — PR titles are links inside rich text blocks, which Slack never unfurls.
- Tests: link present in the grouped, flat and no-PRs cases; absent when `CanvasURL` is empty; surviving a message that hits the block limit; both unfurl options passed on post and update.

### 9. Docs & permissions

- README: new "📋 PR Tracker Canvas" section explaining what it is and how to set it up — add a canvas tab to the reminder channel, then ⋮ → Copy link and paste it into `pr-tracker-canvas-link`. State the access requirement behind it (Step 6), so a canvas kept elsewhere can still be made to work. Add `canvases:write` to the Slack scope table (canvas-only). The GitHub permissions block stays as is.
- Mention that the reminder message then carries a footer link to the canvas (Step 8).
- Document the new input in the inputs table.
- Exercise both the on and off paths end to end. No workflow-permission change needed.
  - `.github/workflows/pr-reminder.yml`: set `pr-tracker-canvas-link`, giving the scheduled runs a real, continuously refreshed canvas in both `post` and `update` mode.
  - `.github/actions/e2e-tests/action.yml`: set it on the "Run with filters" step only — the richest case (multi-repository, `group-by-repository: true`, filters), covering grouped open PRs next to the always-flat draft list.
  - Leave it unset on the "Basic run" and "Multi-repository run" steps, so every release also verifies the action still behaves exactly as before when the input is absent.
  - The link goes in as a plain literal, like `slack-channel-name` — it isn't a secret, though it does carry the workspace name and team ID.
- Manual prerequisites, not automatable here: grant `canvases:write` to the Slack app/bot token these workflows use, then add the canvas tab to the channel.

### 10. Spec sync

- Update `githubclient.spec.md`, `slackclient.spec.md`, `prparser.spec.md`, `config.spec.md`, `messagecontent.spec.md`, `messagebuilder.spec.md` for the changes above.
- Add `canvascontent.spec.md` and `canvasbuilder.spec.md`.

## Consequences

### Positive

- An always-current PR/WIP view lives directly in Slack, with no need to open GitHub.
- Draft/WIP visibility, absent from the main reminder message by design, becomes available to whoever wants it.
- Every scheduled reminder carries a footer link into the canvas, so the transient message becomes the entry point to the persistent view — at no extra scope, since the user supplies the URL.
- Fully additive and off by default: existing users see zero behavior change (zero-value fetch options, early-exit in `run.go`).
- The R1-R3 refactors close pre-existing test-coverage gaps (draft filter, repository grouping, old-PR/author-fallback display paths) independent of whether the canvas feature itself is used.

### Negative

- Opted-in runs make one extra GitHub API call per PR (commit lookup for activity), adding to rate-limit consumption.
- Canvas access can't be granted by the action itself. The intended setup (a canvas tab in the reminder channel) makes it implicit, but a canvas kept anywhere else needs manual sharing — easy to miss, and it only surfaces as a warning log on failure.
- Two new packages (`canvascontent`, `canvasbuilder`) largely mirror existing ones (`messagecontent`, `messagebuilder`), adding maintenance surface for a feature many users won't enable.

### Neutral

- Activity and draft-staleness thresholds are hardcoded, not configurable, in v1.
- Link unfurling is now explicitly disabled for the reminder message. No visible change today, but any future link in the message won't preview either.
