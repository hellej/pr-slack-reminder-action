# PR tracker canvas

date: 2026-08-01
status: draft

The "PR tracker canvas": a Slack canvas this action keeps updated with a live view of open + WIP PRs. Use this name in user-facing text (input description, README).

## Goals

- Motivation: the reminder message is transient and drafts-free by design, and even GitHub itself has no single view of a team's open + WIP PRs (incl. their statuses/staleness/reviews) across multiple repos — the canvas fills both gaps with a persistent, on-demand, cross-repo view in Slack.
- Optional feature: keep a Slack canvas continuously showing open PRs (oldest first) and draft/WIP PRs (most recent activity first).
- Canvas always includes draft PRs (the main reminder message never does — unchanged).
- Each PR row gets an activity-state indicator (circle): grey = stale (no commits in 48h), yellow = semi-active (no commits in 24h), green = active (commits in last 24h).
- Draft PRs inactive for more than 2 months are excluded by default.
- One new input, off by default, no other new configurability.

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
- `prparser`: `PR` gains an `ActivityState()` method (grey/yellow/green, based on `LastActivityAt`) and a most-recent-activity sort, used only by the draft section.
- New `canvascontent` package (mirrors `messagecontent`; Go code stays medium-named, `canvas*`, no need to spell out "PR" internally): builds a canvas-ready `Content` — open PRs (oldest first, grouped/flat per `group-by-repository`) + draft PRs (always flat regardless of that input, most-recent-activity first, >2 months inactive excluded).
- New `canvasbuilder` package (mirrors `messagebuilder`): renders `canvascontent.Content` to Slack canvas markdown, reusing display-text helpers extracted from `messagebuilder` in the pre-refactor.
- `slackclient`: gains a method that fully replaces a canvas's content by ID — one `canvases.edit` call, `replace` operation, `section_id` omitted.
- `run.go`: after the existing post/update logic, if `pr-tracker-canvas-id` is set, runs one independent full PR fetch (open + draft, both run modes) and overwrites the canvas. Failures are logged as warnings, not fatal — same pattern already used for `DeleteMessage` failures in `run.go` — so a canvas hiccup can't take down the core reminder message.
- Permissions: needs `canvases:write` (Slack, new) and `contents: read` (GitHub, new), both canvas-only. Also needs a one-time manual step in Slack: sharing the canvas with the bot (see Step 6) — the scope alone doesn't grant write access to a specific canvas.

## Breaking change classification

Non-breaking / **minor** release. New optional input, default off, no change to existing inputs, outputs, or default behavior.

## Summary of steps

- R1. Refactor: make draft-exclusion in `githubclient` explicit and optional instead of hardcoded
- R2. Refactor: extract repository-grouping into a shared, reusable helper (closes a test-coverage gap)
- R3. Refactor: extract medium-agnostic PR display text out of `messagebuilder` (closes two test-coverage gaps)
1. `action.yml` + `config`: add the new input
2. `githubclient`: add last-activity (head commit) lookup
3. `prparser`: add activity state + most-recent-activity sort
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
  - author display: Slack mention (`<@ID>`) if mapped, else GitHub name
  - reviewers summary text (✅/💬 grouping of approvers/commenters)
  - age text, including the old-PR warning marker vs. plain "N ago"
  - closed-but-not-merged / merged markers
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

- Add `GithubGitDataService` interface wrapping `GetCommit(ctx, owner, repo, sha) (*github.Commit, *github.Response, error)` — matches `(*github.GitService).GetCommit` in `go-github/v78` exactly ([Git Data API, "Get a commit"](https://docs.github.com/en/rest/git/commits?apiVersion=2022-11-28#get-a-commit)); wire into `client`, `NewClient`, `GetAuthenticatedClient` (pass `ghClient.Git`). Use the commit's `committer.date`.
- Add `PRFetchOptions.FetchActivityTimestamps bool`. When set, `addReviewerInfoToPRs`'s per-PR fan-out gets a 4th concurrent call alongside the existing three (reviews, comments, timeline comments): fetch the commit at `pr.Head.SHA` and record its committer date — the true last-push time, unlike `updated_at` (which also changes on comments, labels, etc.).
- Add `PR.LastActivityAt *time.Time` (nil when not fetched, or on lookup failure — treat like the existing "partial failure" pattern for reviews/comments: don't fail the whole PR).
- Update `testhelpers/mockgithubclient` with a mock Git service.
- The new `contents: read` GitHub permission is confirmed: the endpoint's docs page states "Get a commit object" requires the "Contents" repository permission (read).

### 3. `prparser`: activity state + activity sort

- Add an `ActivityState` type (`Stale`, `SemiActive`, `Active`) and `PR.ActivityState() ActivityState`, derived from `LastActivityAt` using hardcoded 48h/24h thresholds. Undefined/zero state if `LastActivityAt` is nil.
- Add an exported sort (most-recent-activity first) for use by the draft section, distinct from `ParsePRs`'s existing oldest-first sort.

### 4. `canvascontent` package

- Mirrors `messagecontent.GetContent`'s shape but for canvas: takes all fetched PRs (open + draft) and content inputs, and produces open-PR content (oldest first, grouped/flat per `group-by-repository`, via the R2 helper) plus draft-PR content (always flat, most-recent-activity first, regardless of `group-by-repository`).
- Excludes draft PRs whose `LastActivityAt` (not creation time) is older than a hardcoded `MaxDraftPRInactivity` (2 months / 60 days).
- Fixed fallback text for the "nothing to show" case.

### 5. `canvasbuilder` package

- Renders `canvascontent.Content` to a Slack canvas markdown string (`slack.DocumentContent{Type: "markdown", ...}`), reusing the R3 display-text helpers.
- Each PR row (open or draft) shows the same fields as its message counterpart — linked title, age text with the old-PR warning marker, author, reviewers, merged/closed marker — with only one addition: an activity-state circle prefix derived from `PR.ActivityState()`.
- Structure: open-PR heading/list (grouped or flat, matching the message's style) followed by a "Work in Progress" heading and the draft list.

### 6. `slackclient`: full canvas content replace

- Add `EditCanvas` to `SlackAPI` (exists in `github.com/slack-go/slack` v0.27.0 — confirmed in source).
- Add `Client.ReplaceCanvasContent(canvasID, markdown string) error`: one `EditCanvas` call (`EditCanvasParams{CanvasID, Changes: []CanvasChange{{Operation: "replace", DocumentContent: ...}}}` — confirmed struct shape in `slack-go/slack` v0.27.0's `canvas.go`), `SectionID` left at its zero value (`""`, omitted on the wire via `omitempty`), `DocumentContent{Type: "markdown", Markdown: markdown}`. Confirmed via [Slack's `canvases.edit` docs](https://docs.slack.dev/reference/methods/canvases.edit/#content-operations): omitting `section_id` on a `replace` operation replaces the entire canvas in one call, and the method needs only the `canvases:write` scope.
- Add mock support in `testhelpers/mockslackclient`.
- **Scope alone isn't sufficient**: canvases have their own access control (read/write/owner), separate from OAuth scopes ([`canvases.access.set` docs](https://docs.slack.dev/reference/methods/canvases.access.set)). Since the user creates the canvas as themselves, the bot has no access to it by default and `canvases.edit` will fail until the user explicitly shares the canvas with the bot (e.g. sharing it into a channel the bot's already a member of). Document this as a required manual setup step (Step 8) — it can't be automated by the action itself.

### 7. `run.go`: wire up the canvas refresh

- `Run()` currently returns directly from inside the `RunMode` switch (`return runPostMode(...)` / `return runUpdateMode(...)`), so there's no code path "after" it today. Restructure to capture the switch's result in an `err` variable; if `err != nil`, return it unchanged (unaffected by this feature); otherwise, if `cfg.PRTrackerCanvasID != ""`, run the canvas refresh step before returning `nil`. This means the canvas only refreshes after a successful post/update — a failed primary run keeps failing the same way it does today, and doesn't attempt a canvas write.
- Canvas refresh step: `FindOpenPRs(ctx, cfg.Repositories, cfg.GetFiltersForRepository, PRFetchOptions{IncludeDrafts: true, FetchActivityTimestamps: true})`, then `canvascontent`/`canvasbuilder`/`slackclient.ReplaceCanvasContent`.
- This always does a fresh full fetch, independent of `update` mode's targeted `GetPRs` (state-tracked refs) — the canvas reflects current reality, not a previously-sent PR set.
- Log and continue (don't fail the run) if this step errors.

### 8. Docs & permissions

- README: new "📋 PR Tracker Canvas" section explaining what it is and how to set it up — create a canvas in Slack, share it with the bot (required, see Step 6), copy its ID into `pr-tracker-canvas-id`. Add `canvases:write` to the Slack scope table (canvas-only). Add `contents: read` to the GitHub permissions block (canvas-only).
- Document the new input in the inputs table.
- Update `.github/workflows/pr-reminder.yml` and `.github/actions/e2e-tests/action.yml` to exercise `pr-tracker-canvas-id`, so the feature gets real end-to-end coverage. No workflow-permission change needed: `pr-reminder.yml` already declares `contents: read` at job level, and the workflows that invoke `e2e-tests` (`build.yml`, `release.yml`) already declare `contents: write` (a superset). Note: the Slack app/bot token used by these workflows needs the new `canvases:write` scope granted manually in Slack, and a canvas needs to be created and shared with the bot by hand to get an ID to test against — call this out, it can't be automated here.

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

- Opted-in runs make one extra GitHub API call per PR (head-commit lookup for activity), adding to rate-limit consumption.
- Canvas access can't be granted by the action itself — the user must manually share the canvas with the bot in Slack, a step that's easy to miss and only surfaces as a silent warning log on failure.
- Two new packages (`canvascontent`, `canvasbuilder`) largely mirror existing ones (`messagecontent`, `messagebuilder`), adding maintenance surface for a feature many users won't enable.

### Neutral

- No canvas URL is linked from the reminder message; users find the canvas in Slack themselves (see Non-goals).
- Activity and draft-staleness thresholds are hardcoded, not configurable, in v1.
