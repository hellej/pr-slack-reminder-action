# PR tracker canvas

date: 2026-08-19
status: draft

The "PR tracker canvas": a Slack canvas this action keeps updated with a live view of open + WIP PRs. Use this name in user-facing text (input description, README).

## Goals

- Motivation: the reminder message is transient and drafts-free by design, and even GitHub itself has no single "live" view of a team's open + WIP PRs (incl. their statuses/staleness/reviews) across multiple repos. The canvas fills both gaps with a persistent, on-demand, cross-repo view in Slack.
- New optional feature: keep a Slack canvas continuously showing open PRs (oldest first) and draft/WIP PRs (most recent activity first).
- Canvas always includes draft PRs (the main reminder message never does, unchanged).
- Open PR rows carry the same fields in the same order as the reminder message's rows, except how authors and reviewers are rendered (see "Canvas content format").
- WIP rows show last-push activity instead of age. The last push predicts which draft gets opened for review next better than its age does.
- Draft PRs with no activity for 60 days are always excluded.
- One new input, off by default, no other new configurability.
- The input takes the canvas link the user copies from Slack. The action parses the ID out of it.
- The canvas is a tab in the reminder channel, so it is one click from the message. The message itself is unchanged (see Step 8).

### Canvas content format

**Open PRs**

- **Update CI workflow for faster Go builds** 🚨 `4 days old` by José (✅ yzma, chaca)
- **Revise onboarding guide in README** _2 days ago_ by PR bot (✅ kronk)
- **Implement feature flag system for UI** _19 hours ago_ by José (✅ hellej / 💬 alice, bob)

**Work in Progress**

- **Spike: replace mux with chi** by José `updated 2 hours ago`
- **Refactor state store** by kronk (💬 alice) `idle 3 days` 💤
- **Prototype canvas rendering** by alice `idle 12 days` 💤

---

_Updated 2026-08-08 06:15 UTC_

The two section headings are fixed strings, rendered as `## Open PRs` and `## Work in Progress` (shown bold above only so this example renders). When grouping is on, each repository is an `###` sub-heading under `## Open PRs`, no repeated top-level heading per repository.

The canvas closes with a blank line, a `---` divider and a `_Updated <YYYY-MM-DD HH:MM UTC>_` line, preceded by a `_Fetch limited to the newest …_` line when a fetch cap trimmed a section (Step 6). A canvas nobody has refreshed (action disabled, failing, or not yet run) otherwise looks identical to a current one, which defeats the point of a live view. UTC because there is no timezone input.

No top-level heading. The canvas title lives on the tab, not in the document, and survives a full content replace (verified on `#pr-reminders-test`, 2026-08-08).

An empty section keeps its heading and shows one italic line instead of rows (Step 6).

Authors and reviewers are plain names: GitHub name, falling back to username, never Slack mentions. Every run replaces the whole canvas, and each replace would re-notify everyone mentioned.

A WIP row is: linked title, author, commenters, activity chip (Step 3), then 💤 if idle for +48h. Never approvers and never the 🚨 old-PR marker: nobody has been asked to review a draft yet, so an approval or a review-latency nag doesn't apply. `canvasbuilder` (Step 6) simply doesn't render the ✅ group on WIP rows, even though the underlying fetch now returns approvers for drafts too (see Step 3). Commenters come from the same derivation open PRs use: review comments plus timeline comments, since GraphQL fetches reviews for every enriched PR regardless of draft status.

"updated" is reader-facing wording only: the chip is backed by the head commit's committer date, not GitHub's `updated_at` (Step 3). Don't "fix" the mismatch by switching the data source. `updated_at` serves only as a fallback when the head commit date is unavailable.

### Existing inputs and the canvas

Which of today's inputs shape canvas content:

| Input | Canvas | Note |
| --- | --- | --- |
| `filters`, `repository-filters` | applies | same fetch path as the message (Step 7) |
| `github-repositories`, `GITHUB_REPOSITORY` | applies | same repository set |
| `github-user-slack-user-id-mapping` | ignored | no mentions on the canvas, names come from GitHub |
| `old-pr-threshold-hours` | applies | drives 🚨 on open rows, never on WIP rows |
| `group-by-repository` | applies to the open section only | WIP list is always flat |
| `pr-list-heading` | ignored | canvas headings are fixed (see "Canvas content format") |
| `no-prs-message` | ignored | fixed fallback text (see Non-goals) |
| `run-mode`, `state-artifact-name` | ignored | canvas always shows all currently open + WIP PRs, never the state-tracked set `update` mode's message re-fetches (Step 7) |
| `slack-channel-name`, `slack-channel-id` | ignored | the canvas is addressed by ID (parsed from `pr-tracker-canvas-link`), not by channel |

Not an input, but same question: `/snooze ... for N days` comments apply to the canvas too, drafts included. Snoozed PRs are dropped inside `githubclient`, before any content package sees them.

## Non-goals

- Configurable thresholds (activity windows, draft staleness). Hardcoded for now.
- Auto-creating or discovering a canvas. The user creates it themselves in Slack and pastes its link. The action never creates, deletes, or looks one up by channel.
- Splitting canvas content if it exceeds Slack's canvas size limits. Deferred: an oversized canvas fails the write, and so the run, until the PR count drops. The logged markdown length (Step 1) is the clue. Revisit if it ever happens. The fetch caps at 50 open and 15 WIP PRs (R1).
- Persisting canvas identity in `state`. The ID is supplied fresh via input every run, nothing to persist.
- A "no PRs" message input for the canvas (a fixed fallback string is used instead).
- A canvas-only mode. The message path always runs, and `pr-list-heading` stays required when `group-by-repository` is false.
- Closed PRs on the canvas, struck through or otherwise. The canvas refresh fetches open PRs only. Deferred, not rejected: Steps 4-6 keep the seams for a third section (see "Room for a recently-closed section").

### Room for a recently-closed section

A future third canvas section listing recently closed/merged PRs stays additive. What it would need, and what this plan already covers:

- `githubclient`: a new list path, aliased `repository(...) { pullRequests(states: [CLOSED, MERGED], first: 100, orderBy: {field: UPDATED_AT, direction: DESC}) }`, stopping at a time window, following `buildListOpenPRsQuery`'s alias/variable shape (Step 3 of [001](001_GraphQL-migration.md)). This is genuinely new code, not a `PRFetchOptions` flag: open PRs are bounded by "still open", closed PRs need a cutoff. Inherent to the feature, nothing here makes it worse. Unverified: whether GitHub's `PullRequestOrderField` GraphQL enum includes `UPDATED_AT` alongside the `CREATED_AT` value already used and confirmed live (see 001's "Query size limits"). Documented publicly alongside `CREATED_AT`, not confirmed by introspection in this repository.
- `prparser`: reuses Step 4's keyed newest-first sort with `ClosedAt`, once that field exists. Today's `PullRequest` (`models.go`) doesn't carry a closed/merged timestamp, since REST's `ClosedAt`/`MergedAt` were never migrated. Adding it means selecting `closedAt`/`mergedAt` in that new list query.
- `canvascontent`/`canvasbuilder`: one more section field and one more `renderSection` call (Steps 5-6).
- Row markers: `canvasbuilder` reads the existing `PR.IsMerged()`/`PR.IsClosedButNotMerged()` booleans, and canvas markdown has `~~strike~~` for the closed case (verified 2026-08-08, Step 6).
- `run.go`: a closed-PR list is its own fetch either way, so it lands next to the canvas refresh without touching the message path's fetch sharing (Step 7).

## Target shape

- `action.yml`: new optional input `pr-tracker-canvas-link` (string, no default). Empty/unset means the feature is off, matching the "empty means unused" requirement literally. The user creates and owns the canvas entirely themselves (see Non-goals).
- `config`: parses the link once into a canvas ID, so no other package parses it again.
- `githubclient`: `Client.FindOpenPRs` gains an explicit `PRFetchOptions{IncludeDrafts bool}` parameter (zero value means today's behavior exactly) and returns `OpenPRsResult{PRs []PR; OpenPRsCapped, DraftPRsCapped bool}` instead of a bare slice. Draft staleness is not a fetch-time filter: `LastActivityAt` itself comes from enrichment, so it isn't known until after the fetch completes (Step 3). `canvascontent` prunes stale drafts afterward (Step 5). Every enriched `PR` already carries a head-commit date once Step 3 lands: the GraphQL enrichment query already selects `commits(last: 1)` for every PR (open or draft) as of [001](001_GraphQL-migration.md), so no extra API call is added for it. `PullRequest` gains `LastActivityAt *time.Time`.
- `prparser`: `PR` gains activity display helpers (based on `LastActivityAt`) and a most-recent-activity sort, used only by the draft section.
- New `canvascontent` package (mirrors `messagecontent`, Go code stays medium-named `canvas*`, no need to spell out "PR" internally): builds a canvas-ready `Content`, open PRs (oldest first, grouped/flat per `group-by-repository`) plus draft PRs (always flat regardless of that input, most-recent-activity first, drafts inactive over 2 months excluded).
- New `canvasbuilder` package (mirrors `messagebuilder`): renders `canvascontent.Content` to Slack canvas markdown, reusing display-text helpers extracted from `messagebuilder` in the pre-refactor.
- `messagecontent`/`messagebuilder`: unchanged. The reminder message is byte-identical whether the canvas is on or off (Step 8).
- `slackclient`: gains a method that fully replaces a canvas's content by ID, one `canvases.edit` call, `replace` operation, `section_id` omitted.
- `run.go`: if `pr-tracker-canvas-link` is set, gets open + draft PRs and overwrites the canvas. `post` mode shares the message's fetch (with drafts switched on); `update` mode fetches separately, since its message path is state-tracked (Step 7). The canvas refresh and the message path are independent attempts, either can fail without stopping the other, and their errors are joined at the end, so the action fails if either did.
- Permissions:
  - Slack: one new scope, `canvases:write` (canvas-only). On top of it, a one-time manual step: the user creates the canvas as a tab in the reminder channel (see Step 1), since the scope alone doesn't grant write access to a specific canvas.
  - GitHub: no change. The activity lookup rides the enrichment query the message path already runs, on `pull-requests: read`, already required.

## Breaking change classification

Non-breaking / **minor** release. New optional input, default off, no change to existing inputs, outputs, or default behavior.

## Summary of steps

- R1. Refactor: make draft-exclusion in `githubclient` explicit and optional instead of hardcoded
- R2. Refactor: extract repository-grouping into a shared, reusable helper (closes a test-coverage gap)
- R3. Refactor: extract medium-agnostic PR display text out of `messagebuilder` (closes two test-coverage gaps)
1. `slackclient`: fully replace canvas content by ID
2. `action.yml` + `config`: add the new input, parse link → ID + URL
3. `githubclient`: map the head-commit date already fetched by enrichment onto each PR
4. `prparser`: add activity text + most-recent-activity sort
5. `canvascontent`: build canvas content
6. `canvasbuilder`: render canvas content to markdown
7. `run.go`: wire up the canvas refresh
8. Reminder message: canvas link footer, built and then removed
9. Docs & permissions: README, `pr-reminder.yml`, `e2e-tests/action.yml`
10. Spec sync

`slackclient` leads because it is the only step calling an API this repo has never called, and nothing else depends on it. A scope or canvas-access surprise then surfaces before the content pipeline is built on top of it. Everything after it runs in dependency order.

All steps land on one feature branch and merge as a single PR, not on `main`. This overrides AGENTS.md's work-on-`main` default. Each step is its own commit on that branch, so each review reads one step's diff.

## Steps

### R1. Refactor: explicit, optional draft-exclusion + per-kind capping (`internal/apiclients/githubclient`)

- Add `PRFetchOptions{IncludeDrafts bool}` and thread it through `Client.FindOpenPRs` and `getPRFilterFunc`, replacing the hardcoded `!pullRequest.GetDraft()` in `getPRFilterFunc[T repositoryPullRequest]` (`githubclient.go`). `Client.GetPRs` does not take it: update mode re-fetches state-tracked refs, which are never drafts, since `state.SavePostState` only ever records the message path's non-draft PRs.
- GraphQL's phase-1 listing already returns drafts. `openPullRequestsFragment` (`graphqlfetch.go`) already selects `isDraft` for every `states: OPEN` PR, draft or not: today's exclusion is entirely client-side, in `getPRFilterFunc`. No query change is needed to include drafts in `listOpenPRs`, only a parameterized filter.
- Update the one `FindOpenPRs` call site in `run.go` (line 60) to pass `PRFetchOptions{}` (zero value). The other GitHub call there is `GetPRs`, which doesn't take the options. Behavior must stay byte-for-byte identical to today.
- Add `capOpenPRResultsToLimit(prResults []PRResult, includeDrafts bool) (capped []PRResult, openPRsCapped, draftPRsCapped bool)`, called from `FindOpenPRs` in place of today's `capPRsToLimit(prResults)`. When `includeDrafts` is off, it delegates straight to the existing generic `capPRsToLimit[PRResult]`, unchanged. When on, it splits the input by `pr.GetDraft()` (two `utilities.Filter` calls, `PRResult` carries no partition helper today) and caps each bucket separately: non-drafts through the existing `capPRsToLimit` at `MaxPRsToFetch` (50, unchanged), drafts through a new `capDraftPRResultsToLimit([]PRResult) []PRResult` at `MaxDraftPRsToFetch` (15). `capPRsToLimit` itself is untouched either way, since a shared capped slice would let drafts displace open PRs the message would otherwise show.
  - Drafts get their own capping function instead of a parameterized `capPRsToLimit`: that one hardcodes both its limit and its sort (created desc, then updated desc), and the draft bucket needs a different limit and an updated-desc sort. A second concrete function keeps `capPRsToLimit` literally untouched and adds no limit/sort parameters, at the cost of ~6 duplicated lines (AGENTS.md: minor duplication over speculative parameterization).
  - Three return values rather than a `capFlags` struct, which would be built once and immediately copied into `OpenPRsResult`.
- `Client.GetPRs` keeps calling `capPRsToLimit[PR]` directly, unmodified: it never sees drafts (see above), so it needs no per-kind split.
- 15, not 50: a WIP list that long is unreadable, the section is ordered newest-activity-first precisely because only its top is interesting. This is a readability choice, not a GraphQL cost-saving one (see Consequences).
- The draft bucket sorts by `updated_at` desc, not creation date, so an over-cap draft fetch keeps the most recently active ones (the set the canvas orders and prunes by, Steps 4-5). Not the head-commit date: capping runs on `PRResult`, before enrichment (Step 3), so `LastActivityAt` doesn't exist yet, and `updated_at` is the best proxy available at that point. The non-draft bucket keeps today's `capPRsToLimit` sort (creation date, then update date).
- `FindOpenPRs` returns `(OpenPRsResult, error)` instead of `([]PR, error)`: `OpenPRsResult{PRs []PR; OpenPRsCapped, DraftPRsCapped bool}`, each flag set by `capOpenPRResultsToLimit` when it actually trimmed that bucket. The canvas footer note (Steps 5-6) needs to know the cap fired, and no downstream length can tell: `excludeSnoozedPRs` runs after the cap, and `canvascontent` prunes stale drafts on top, so a capped fetch routinely arrives as fewer PRs than its cap.
- `logFoundPRs`'s "Found %d open pull requests" gains a draft count when `IncludeDrafts` is on. Log text only.
- Test coverage check: `githubclient_test.go` already has an explicit "draft PR should be filtered out" case (per `githubclient.spec.md`'s "draft PRs are always excluded" bullet). No gap here. `FindOpenPRs`'s call sites take the new argument and read `.PRs` off the result; no assertion changes there. Add table-driven cases: `IncludeDrafts: true` lets a draft PR through, `IncludeDrafts: true` with both buckets over their caps keeps the same non-draft set as `IncludeDrafts: false` would, drafts are cut at the lower `MaxDraftPRsToFetch`, and both cap flags set only for the bucket that overflowed.
- `testhelpers/mockgithubclient` needs no change for this step: it renders `MockGitHubClientOptions` into GraphQL JSON regardless of `IncludeDrafts`, since the fetch options never reach the mock transport, only the query text and variables do, and neither is shaped by this option.

### R2. Refactor: shared repository-grouping helper

- Extract `messagecontent.groupPRsByRepositories`'s bucketing (by repository path, alphabetical) into an exported helper on `prparser`, since `canvascontent` needs identical grouping for its open-PR section.
- The helper returns `[]prparser.RepositoryPRs` (`{Repository models.Repository; PRs []PR}`) and no display strings. `HeadingPrefix` and `RepositoryLinkLabel` stay in `messagecontent`, which maps the groups into its existing `PRsOfRepository`. `messagecontent.PRsOfRepository` and `messagebuilder` are untouched, and Block Kit wording stays out of `prparser`.
- Move the pulls-page URL onto `models.Repository` as `GetPullsURL()` (`https://github.com/<path>/pulls`), next to `GetPath()`. `messagecontent` and `canvasbuilder` (Step 6) both call it, so the format string isn't written out in two packages. `messagecontent.groupPRsByRepositories` builds this URL inline today (`fmt.Sprintf("https://github.com/%s/pulls", repo.GetPath())`); this step is where it moves.
- Test coverage gap found: there is no `messagecontent_test.go` at all. Grouping is only exercised indirectly through `messagebuilder_test.go`'s fixed two-repository example and `main_test.go`'s integration cases. Add a direct unit test for the extracted helper (alphabetical ordering with out-of-order input, single- and multi-repository bucketing) before/while moving it, so the extraction has a real safety net instead of only indirect coverage. The helper returns no URL once the format string moves onto `models.Repository`, so the link format is covered on its own in `internal/models/repository_test.go` (`TestRepositoryGetPullsURL`) and no `messagecontent_test.go` is added.
- `messagecontent` calls the extracted helper. Behavior and existing tests unchanged.

### R3. Refactor: shared PR display text

- Extract from `messagebuilder` into methods on `prparser.PR` (or a small shared helper file), as plain strings/booleans instead of `slack.RichTextSectionElement`s:
  - reviewers summary text (✅/💬 grouping of approvers/commenters) as `GetReviewersTextSegments(approvers, commenters []Collaborator) []string`, taking approvers and commenters as explicit parameters rather than reading `pr.Approvers`/`pr.Commenters` directly, so the canvas's WIP rows (Step 6) can call it with an empty approvers slice and get the commenters-only 💬 rendering without a second code path.
    - One text run per segment (`" (✅ "`, `"Alice"`, `", "`, …), not a single joined string. `messagebuilder` emits one Block Kit text element per run today, so a joined string would collapse them into one element and change every payload that lists reviewers, which the `cmd/pr-slack-reminder/testdata/snapshots/` golden files pin. Payloads with no reviewers stay byte-identical either way. Segments are still plain strings. Step 6 joins them, and can escape every segment, since no glue segment contains an escapable character.
  - age text as `PR.GetPRAgeDisplayText()`: `N days old` when `PR.IsOldPR`, `N days ago` otherwise. It sits alongside the existing `GetPRAgeText()`, which still returns the bare magnitude (Step 4 reuses that one). Text only, the 🚨 marker stays in each builder, which places it outside its own styled element (bold+code in `messagebuilder`, a code span in `canvasbuilder`), and the leading/trailing spaces stay there too, being Block Kit element boundaries rather than text. Open PR rows only, the WIP section must not call it (see Goals).

  The closed-but-not-merged and merged markers stay in `messagebuilder`, only the message ever renders a closed or merged PR (see Step 6).
- Author display is not extracted: the message prefers a `<@ID>` mention and falls back to `GetGitHubName()`, the canvas always uses `GetGitHubName()` (see Step 6). Both fields are exported, so each builder reads them directly.
- Unit coverage to add in `messagebuilder_test.go` before extracting, so a regression is caught at the package that owns the rendering:
  - the old-PR warning-marker path (`IsOldPR: true` → 🚨 + bold/code age text)
  - the author-fallback path (no mapped `SlackUserID` → GitHub display name instead of a mention)
  - Both are already asserted end to end in `main_test.go`, so the extraction has a safety net either way. These new tests make the failure land in the package that broke.
- The merged 🚀 and closed-but-not-merged strike-through markers already have unit coverage, `TestMergedAndClosedPRFormatting`. No gap there, safe to extract as is.
- Unit coverage to add for the extracted helpers themselves, table-driven: reviewers summary text across no reviewers / approvers only / commenters only / both, and age text on either side of the old-PR threshold.
- `messagebuilder` wraps the extracted plain strings in its Block Kit elements. Behavior and existing tests unchanged. This lets `canvasbuilder` (Step 6) reuse the exact same text instead of re-deriving it.

### 1. `slackclient`: full canvas content replace

- Add `EditCanvas` to `SlackAPI` (exists in `github.com/slack-go/slack` v0.27.0, the version currently in `go.mod`, confirmed in source).
- Add `Client.ReplaceCanvasContent(canvasID, markdown string) error`: one `EditCanvas` call (`EditCanvasParams{CanvasID, Changes: []CanvasChange{{Operation: "replace", DocumentContent: ...}}}`, confirmed struct shape in `slack-go/slack` v0.27.0's `canvas.go`), `SectionID` left at its zero value (`""`, omitted on the wire via `omitempty`), `DocumentContent{Type: "markdown", Markdown: markdown}`.
- Confirmed via [Slack's `canvases.edit` docs](https://docs.slack.dev/reference/methods/canvases.edit/#content-operations): omitting `section_id` on a `replace` operation replaces the entire canvas in one call, and the method needs only the `canvases:write` scope.
- Two mocks to extend, for two different interfaces:
  - `testhelpers/mockslackclient.MockSlackAPI` implements `slackclient.Client` despite its name, it gains `ReplaceCanvasContent`, recording the canvas ID and markdown so `main_test.go` can assert on canvas content end to end.
  - `mockSlackAPI` in `internal/apiclients/slackclient/slackclient_test.go` is the `SlackAPI` mock, it gains `EditCanvas`, capturing the params and returning a configurable error.
- Log the canvas ID and markdown length before the call, confirm on success, matching `SendMessage`/`UpdateMessage`. The length is the only clue if content hits Slack's canvas size limit.
- On error, wrap it with a concise hint: `"canvas update failed: check that the bot has canvases:write permission and is invited to the channel where the canvas is"`, this is what surfaces in the failed run's log (Step 7), so it carries the whole diagnosis.
- **Scope alone isn't sufficient for a canvas kept elsewhere**: canvases have their own access control (read/write/owner), separate from OAuth scopes ([`canvases.access.set` docs](https://docs.slack.dev/reference/methods/canvases.access.set)). A canvas the user creates outside the reminder channel gives the bot no access, and `canvases.edit` fails until it is shared. The channel tab below is what avoids that.
- **Intended setup**: create the canvas as a tab in the reminder channel itself.
- Verified end to end on 2026-08-08: a canvas the user created as a tab in `#pr-reminders-test`, edited by the `pr_bot` token carrying `canvases:write` and nothing canvas-related beyond it.
- Both `insert_at_end` and a full `replace` with no `section_id` returned `ok: true`, with no sharing step and no `canvases.access.set` call. The tab's own title survived the replace.
- Also in favour of a channel canvas: `canvases.edit` lists `free_teams_cannot_edit_standalone_canvases`, so standalone canvases are a dead end on free workspaces.
- Document the channel-tab path as *the* setup (Step 9), not one option among several. It can't be automated by the action either way.

### 2. New input: `pr-tracker-canvas-link`

- `action.yml`: add optional string input, no default. Description states the intent plainly, e.g. "Link to a Slack canvas to keep updated with a live tracker of open and work-in-progress pull requests. Open the canvas in Slack → ⋮ → Copy link. Leave empty to disable (default)."
- `internal/config/config.go`: add constant `InputPRTrackerCanvasLink`, read via `inputhelpers.GetInput`, add `Config.PRTrackerCanvasID`, empty when the input is unset. Only `run.go` reads it.
- Link shape, confirmed against the real canvas tab in `#pr-reminders-test`: `https://<workspace>.slack.com/docs/<TEAM_ID>/<CANVAS_ID>`, e.g. `https://hellej.slack.com/docs/T08SGDGNB2B/F0BMEPVR1DL`.
- Parse in a new file `internal/config/canvaslink.go` (`getCanvasIDFromLink`), leaving `config.go` with the constant, the fields and the call: `url.Parse`, then require a `docs` segment in `URL.Path` and take the first segment after it matching `^F[A-Z0-9]+$`. Scanning survives a trailing title slug: `TEAM_ID` starts with `T` and slugs are lowercase, so neither can match. No length bound, real IDs already run to 13 characters. Reading `URL.Path` drops query params for free.
  - Requiring `docs` rejects other Slack links that carry an `F…` file ID (`/files/…`), which would otherwise parse and then fail at the `canvases.edit` call.
  - Assumed, on one verified link: every copy path gives a `docs` segment. Accepted for v1.
  - Must be an absolute URL with a host, a bare `F…` ID is rejected. The input is documented as a link copied from Slack, so anything else is a typo, even though only the ID is used.
  - A non-empty input that doesn't parse is a hard config error, joined like the other input validation. A typo would otherwise mean a silently missing canvas. The message names the expected shape and the Copy link path.
- Test cases: the real link above, trailing slash, query params, a trailing title slug, `http`, an unrelated URL, a Slack `/files/F…` link (rejected), a bare `F…` ID (rejected), empty (feature off). They live in `internal/config/config_test.go` (`TestGetConfig_PRTrackerCanvasLink`) and go through `GetConfig`, since the parser is unexported and the config tests are an external `config_test` package.
- `testhelpers/confighelpers.go`: mirror the new input. It needs no `TestConfig` field, every canvas test passes the link through the overrides map, so the helper sets the env var from `""`.
- `go run .github/scripts/check_inputs.go` must still pass.

### 3. `githubclient`: last-activity mapping

The GraphQL migration already changed the ground under this step. `enrichedPullRequestSelection` and `fullPullRequestSelection` (`graphqlfetch.go`) both select `commits(last: 1){ nodes { commit { oid committedDate } } }` today, for every enriched PR, open or draft, with the query comment "commits are selected for the PR tracker canvas and are not read yet." There is no separate REST-style commit-lookup call to add: the data already arrives with every enrichment response, decoded into `pullRequestNode.Commits` and simply unread. This step is a mapping change, not a fetch change.

- Add `PullRequest.LastActivityAt *time.Time` (`models.go`).
- In `prWithReviewers` (`graphqlfetch.go`), read `node.Commits.Nodes`: if non-empty, take its one node's `Commit.CommittedDate` as `LastActivityAt`, raised to `pr.GetCreatedAt()` when that is later. A PR opened today off an old branch has an old head commit, so the raw commit date would understate its freshness and the staleness rule (Step 5) would drop a fresh draft off the canvas. `commits(last: 1)` returns at most one node, so there is nothing to page through and no SHA to cross-check against `headRefOid`, unlike REST's paginated `ListCommits` scan (the SHA check existed there because the endpoint's ordering wasn't documented and it capped at 250 commits with no way to select "just the last one"; GraphQL's `last: 1` argument does that server-side). Unverified: whether GraphQL's `commits(last: 1)` reliably resolves to the PR's actual head commit in every case (e.g. an empty-history PR, a PR on a repository with unusual ref state). [001](001_GraphQL-migration.md)'s live verification (Step 7 there) did not specifically confirm this field; it is carried into this plan as a design assumption made explicit in that plan's Goals ("`commits(last: 1)` … replaces a paginated commit scan with a SHA cross-check … That timestamp is what 002's WIP section sorts, marks and prunes by").
  - `prWithReviewers` doesn't write the field into the `*PullRequest` it is given. `pullRequestWithLastActivity` copies the struct and puts the copy on the returned `PR`, leaving the caller-owned `PRResult.pr` untouched (AGENTS.md: return new structs rather than mutating input pointers).
  - No getter here. Nothing reads the field until Step 4, which adds one.
- Fall back to `pr.GetUpdatedAt()` when `node.Commits.Nodes` is empty (a PR with no commits, or a field-level error on the `commits` connection folding the whole node to its zero value, see 001's error classification). It overstates freshness, but it's an upper bound on the last push, so a busy long-lived draft never drops off the canvas via the staleness rule.
- Leave `LastActivityAt` nil when both `Commits` is empty and `UpdatedAt` is zero. Today's `getTestPR`-style fixtures set neither, so this is the default in tests, not a corner case.
- `LastActivityAt` is populated unconditionally, for every enriched PR, not gated behind a canvas-specific option. The field is already fetched for every PR regardless of whether the canvas is on, so a `FetchActivityTimestamps` toggle would add a flag that gates nothing on the wire. The message path simply never reads the field.
- No branching on `pr.GetDraft()` anywhere in `enrichPRs`/`enrichPRBatch`/`prWithReviewers`: reviews, comments and commits are fetched uniformly for every PR in a batch already, open or draft, since GraphQL batches cost the same (roughly 1-2 points per 25-alias batch, see "Cost model" in Consequences) regardless of which connections are populated for which alias. There is no separate per-draft call to skip. What stays true is the *rendering* choice: `canvasbuilder` (Step 6) still never shows approvers on WIP rows (see "Canvas content format").
- Cover the mapping with one table-driven test, `TestFindOpenPRs_LastActivityAt` (`githubclient_test.go`), over all five outcomes: commit date newer than the creation time, commit date older than it (creation time wins), commit date with an unknown creation time, update-time fallback, nil. No canvas-specific fixtures yet, since the first canvas tests arrive in Step 5; they set `LastActivityAt` on their `PRResult`/`PR`/`PullRequest` fixtures and `Commits` on the mock transport (see below).
- `testhelpers/mockgithubclient.GraphQLTransport.enrichedPullRequestNodeJSON` hardcodes `node["commits"] = connectionJSON([]map[string]any{})` today, an always-empty commits connection. Extend `MockGitHubClientOptions` with a `CommitsByPRNumber map[int]time.Time` (or similar), rendered as a single `commit.committedDate` node, so tests can exercise the non-fallback path. `getPRBatchByRef`'s rendering path (`GetPRs`, same renderer function) picks it up too, for free.
- `run.go`'s `prFetchTimeout` needs no change (see "Cost model" in Consequences): batching at `enrichBatchSize` (25) under `defaultGitHubAPIConcurrencyLimit` (3) means the canvas's extra up-to-15 drafts add at most one more batch, and that batch runs concurrently with the others rather than after them, so worst-case wall time is unchanged from today's 2-batch case.

### 4. `prparser`: activity text + activity sort

- Extract `GetPRAgeText`'s minutes/hours/days formatting into an unexported duration → text helper, `durationText` (`displaytext.go`). It reads `CreatedAt` directly today, so the activity chip can't reuse it as is. Wording stays as it is, including the `1 days` it produces at 24h: the chip inherits it as `idle 1 days`, matching the message's `1 days old`.
- Add a nil-receiver-safe `PullRequest.GetLastActivityAt() *time.Time` in `githubclient` (`models.go`), the accessor Step 3 leaves to this step, matching the other getters there. `githubclient.spec.md`'s "no reader yet" line goes with it.
- Add `PR.GetActivityText() string` (`displaytext.go`), from `GetLastActivityAt()` via that helper: `updated N minutes/hours ago` under 24h, `idle N days` at 24h and above.
- Add `PR.IsIdle() bool` (`prparser.go`): true when `LastActivityAt` is older than a hardcoded `idleThreshold` of 48h.
- `LastActivityAt` can be nil even with the canvas on (Step 3). Nil means unknown, not stale: empty chip text, not idle, never dropped by the staleness rule (Step 5), sorted after every PR with a real timestamp.
- Add `SortPRsNewestFirst(prs []PR, timestamp func(PR) *time.Time) []PR` (`prparser.go`) for the draft section, distinct from `ParsePRs`'s existing oldest-first sort, returning a sorted copy rather than sorting in place. Take the timestamp as a key function rather than reading `LastActivityAt` directly, so sorting by any other timestamp needs no second sort. Nil keys sort last.
- Tests: the duration helper's three magnitudes and their boundaries, both chip wordings and the cutover between them, `IsIdle` around 48h, and the sort with a nil key mixed in. `GetActivityText` carries its own copy of the 24h threshold, so pin it from both sides or it can drift from the helper's. The helper is unexported, so its cases live in an internal test file, `internal/prparser/displaytext_internal_test.go` (`package prparser`), alongside the package's existing external `prparser_test` files.

### 5. `canvascontent` package

- Mirrors `messagecontent.GetContent`'s shape but for canvas: takes all fetched PRs (open + draft) and content inputs, and produces open-PR content (oldest first, grouped/flat per `group-by-repository`, via the R2 helper) plus draft-PR content (always flat, most-recent-activity first, regardless of `group-by-repository`).
- `GetContent` takes one `[]prparser.PR` and splits it itself on `pr.GetDraft()`, drafts to the WIP section, the rest to the open section. `run.go` passes the fetch result unsplit, so the split lives in one place and the caller stays the same in both run modes. Alongside it, the two cap flags and `GeneratedAt`, grouped in one `GetContentOptions` struct rather than growing the parameter list per section.
- `Content` carries `GeneratedAt time.Time` for the footer line, set by the caller (Step 7) rather than read from the clock here, so `canvasbuilder`'s output is deterministic under test (Step 6). It doubles as "now" for the staleness prune below, which is what makes the 60-day cutoff pinnable from both sides in a test: against the wall clock, a fixture built at exactly the cutoff is already past it by the time the comparison runs.
- Keep the two sections as separate named fields on `Content`, not one merged list with a per-PR kind flag, so a third section is a field rather than a rework. The open section mirrors `messagecontent.Content`: `OpenPRs` when flat, `OpenPRsGroupedByRepository []prparser.RepositoryPRs` plus `GroupedByRepository bool` when grouped, only ever one of the two filled. `WIPPRs` is the third field.
- Section headings are fixed strings owned by `canvasbuilder` (Step 6), so `contentInputs.PRListHeading` is unread here, no `<pr_count>` substitution, no "required when `group-by-repository` is false" coupling.
- Excludes draft PRs whose `LastActivityAt` (not creation time) is older than a hardcoded `MaxDraftPRInactivity` (60 days). A nil `LastActivityAt` is kept, unknown is not stale (Step 4). Exported for the spec and for tests. Pruning happens only here, not inside the fetch: `LastActivityAt` comes from enrichment (Step 3), so it isn't known until after the fetch completes, and GraphQL's phase-1 listing already returns every draft regardless of staleness (R1), so there's no cheap way to skip per-draft work at fetch time either.
- No whole-canvas "nothing to show" case: each section falls back on its own, so the both-empty canvas is just both fallbacks (see Step 6).
- `Content` carries `OpenPRsCapped` / `WIPPRsCapped bool` for the footer note (Step 6), passed in by the caller (Step 7) from `githubclient.OpenPRsResult` (R1). Never derived from `len(section)` (R1), the staleness prune above shrinks the WIP list further still.
- Log the counts put on the canvas: open PRs, drafts, and drafts dropped as inactive, otherwise a missing draft has no explanation.
- Tests: the staleness cutoff on both sides and exactly at it (kept), a nil-activity draft kept, drafts ordered newest-activity first with a fixture set whose creation order disagrees with its activity order, drafts kept out of the open section and vice versa, open PRs grouped and flat, the WIP list flat with `group-by-repository` on, `GeneratedAt` taken from the options, and each cap flag reaching `Content` while its section holds fewer PRs than its cap.

### 6. `canvasbuilder` package

- `BuildMarkdown(content canvascontent.Content) string` renders a plain markdown string, reusing the R3 display-text helpers. No slack-go type here: `slackclient.ReplaceCanvasContent` (Step 1) takes the markdown and wraps it in `slack.DocumentContent{Type: "markdown", ...}` itself.
- Canvas `document_content` takes real markdown, not Slack `mrkdwn`: `**bold**`, `_italic_`, `[label](url)`, backtick code spans, `~~strike~~`, `##`/`###` headings, `-` bullets, `---` dividers ([Canvases docs](https://docs.slack.dev/surfaces/canvases/)).
- All of it rendered correctly in live `canvases.edit` replaces against the `#pr-reminders-test` canvas tab, 2026-08-08, including this plan's own fixed strings (`_No open PRs_`, `_Showing the newest 50 open PRs_`, the `---` divider and the italic `_Updated …_` footer).
- No Slack mentions: `canvasbuilder` never reads `pr.Author.SlackUserID`, and renders authors and reviewers through `GetGitHubName()`. Canvas markdown does support `![](@USERID)`, don't switch to it (see "Canvas content format").
- Escape every GitHub-sourced string before it goes into markdown: PR titles, author and reviewer names, repository paths. Block Kit keeps text and styling in separate fields, so nothing in a title can be read as formatting there; a canvas row is one string, where it can. `Add _debug_ flag` italicizes, `**WIP** rewrite` bolds, `` Use `make test` `` code-spans, and `Fix [ABC-123] crash` breaks the link label.
  - Add `escapeMarkdown(string) string` in `canvasbuilder`, backslash-escaping `\`, `` ` ``, `*`, `_`, `[`, `]`, `~`, `<`, `>`, `&`. Backslash first, or it double-escapes what the later replacements add.
  - Link targets need no escaping: they come from GitHub (`GetHTMLURL()`, `GetPullsURL()`) and can't contain a space or a `)`.
  - **Verified in the same probe**, with raw rows placed next to escaped ones. Slack's canvas parser accepts [CommonMark](https://spec.commonmark.org/0.31.2/#backslash-escapes) backslash escapes even though it documents none: every one of `\`, `` ` ``, `*`, `_`, `[`, `]`, `~` was consumed and its character survived, bold and code spans were suppressed, and escapes held inside link labels too. No code-span fallback needed.
  - **`<`, `>` and `&`, verified 2026-08-09** on the same canvas, and the reason each is in the set:
    - Raw `<script>alert(1)</script>`, `<b>bold</b>` and `<!-- old -->` all render literally: the parser does not interpret HTML tags. `<` and `>` are escaped for the autolink case, not for injection.
    - Raw `<https://example.com>` autolinks; escaped, it stays literal text. A PR title carrying a bare URL in angle brackets is the case.
    - Raw `&amp;` decodes to `&`, so HTML entities are live. Without escaping, a title reading `Fix &amp; in output` renders as `Fix & in output`, the wrong text. `\&` renders `&` and blocks the decoding.
    - `\<`, `\>` and `\&` are all consumed, leaving no visible backslash.
  - Delimiter behaviour, same probe, for anyone tempted to trim the escape set:
    - `_` is emphasis only at word boundaries: `Add _debug_ flag` italicizes, a raw `fix_the_thing_now` renders as-is. Keep `_`: the first case is a real PR title.
    - Strikethrough is `~~`, not `~`: `Remove ~~legacy~~ shim` strikes; a lone `~strike~` renders literally. Keep `~`.
    - Spaced delimiters are inert (`a * b * c`, `1 _ 2 _ 3`, `50% * 2` all render as typed). Escaping them anyway costs nothing and keeps the helper free of context rules.
  - Escaping belongs to `canvasbuilder`, not `canvascontent`: it's a property of the output format, and a third renderer would want its own rules.
- Open PR rows: linked title, age text with the old-PR warning marker, author, reviewers.
- No strike-through, no 🚀: the canvas fetch lists open PRs only, so a closed or merged PR can never reach a row here (see Non-goals). Those two markers stay message-only.
- WIP rows: linked title, author, commenters, `PR.GetActivityText()` as a code span, then 💤 if `PR.IsIdle()`. No age text, no 🚨, no 🚀 (see Goals).
- The R3 reviewers helper renders the 💬 group when called with an empty approvers slice and `pr.Commenters` (Step 3), so drafts get their real commenter list (review comments and timeline comments combined) but never their approvers.
- Empty activity text (Step 4) renders no chip and no 💤, leaving the row title-author-reviewers.
- Structure: a fixed `## Open PRs` heading and its list, then a fixed `## Work in Progress` heading and the draft list. Both headings always render, `group-by-repository` or not.
  - Grouped: each repository is an `###` sub-heading under `## Open PRs`, linking to the repository's pulls page. `canvasbuilder` builds that heading from `RepositoryPRs.Repository`, taking the URL from `Repository.GetPullsURL()` (R2). No "Open PRs in " prefix: that would repeat the parent heading.
- Render both through one internal `renderSection(heading string, prs []prparser.PR, renderRow func(prparser.PR) string, emptyText string)`, not two bespoke paths. The two sections differ only in their row renderer and empty text, and a third section then costs one call.
- Empty section: heading still renders, followed by one italic line: `_No open PRs_` / `_No work in progress_`. An empty section means "nothing here right now", which a missing heading can't say; it would read as a broken render instead. Grouped mode with no open PRs renders that same single line, with no repository sub-headings.
- Footer, after both sections: blank line, `---`, then `_Updated <YYYY-MM-DD HH:MM UTC>_` from `Content.GeneratedAt` (see "Canvas content format").
- Above the `Updated` line, when either cap flag is set (Step 5), one italic line naming the fetch limit: `_Fetch limited to the newest 50 open PRs_` / `_Fetch limited to the newest 15 WIP PRs_` / `_Fetch limited to the newest 50 open PRs and the newest 15 WIP PRs_` when both fired, the counts coming from `MaxPRsToFetch` and `MaxDraftPRsToFetch` (R1). Otherwise a capped canvas silently misses PRs, and only the run log says why. The line names the fetch limit, never a count of rows shown: Step 5 prunes inactive drafts after the fetch, so the canvas can carry fewer rows than the cap.

#### Snapshot tests

The canvas is one markdown string, so golden files cover its formatting more cheaply and more completely than element-level assertions.

- `internal/canvasbuilder/testdata/*.md` hold the expected output, one file per case: grouped, flat, grouped with no open PRs, empty open section, empty WIP section, both empty, an old PR, an idle draft, a draft with unknown activity, each cap flag and both together, a `GeneratedAt` outside UTC (the footer label says UTC, so the conversion needs a fixture that would print differently without it), and a PR whose title and author name carry `_`, `*`, `[`, `]`, `~`, `` ` ``, `\`, `<`, `>` and `&amp;`.
- The test compares byte-for-byte, and rewrites the golden file instead when `-update-snapshots` is passed, the flag name `cmd/pr-slack-reminder/snapshot_test.go` already uses. A deliberate format change is then `make update-test-snapshots` plus a reviewable diff, not a hand-edited expectation.
- No new Makefile target: the flag name matches, so `update-test-snapshots` re-records both packages in one `go test` call.
- `escapeMarkdown` is unexported, so its own cases (each escapable character, and a backslash before one of them, which a backslash-last order would double-escape) live in an internal test file, `internal/canvasbuilder/escape_internal_test.go`.
- Determinism: `Content.GeneratedAt` is fixed by the test (Step 5), and PR fixtures set `CreatedAt`/`LastActivityAt` as offsets from `time.Now()`, `prparser` reads the clock directly, and offsets keep the rendered age and activity text stable.
  - Keep those offsets clear of every boundary the rendering rounds or thresholds on: 1h and 24h (`GetPRAgeText`), 48h (`IsIdle`), `MaxDraftPRInactivity`, and any half-unit `math.Round` flips. `30m`, `5h`, `3d`, `10d` are safe; `24h` and `1h30m` flake, since the clock advances between fixture construction and render.
  - Give canvas fixtures a real `HTMLURL`. Fixtures built without one render `[title]()` into the golden file.

### 7. `run.go`: wire up the canvas refresh

- `Run()` currently returns directly from inside the `RunMode` switch (`return runPostMode(...)` / `return runUpdateMode(...)`), so there's no code path "after" it today. Restructure so the message path and the canvas refresh are two independent attempts whose errors are collected, not short-circuited:
  1. capture the switch's result in `messageErr` instead of returning it
  2. if `cfg.PRTrackerCanvasID != ""`, run the canvas refresh regardless of `messageErr`, capturing `canvasErr`
  3. `return errors.Join(messageErr, canvasErr)`
- What still fails fast, before the switch and so before the canvas: config errors and channel resolution by name. The canvas needs neither, but both are one-time setup errors, not per-run conditions.
- The canvas refresh runs even when post/update failed. A failed message send says nothing about whether the canvas can be written, and a stale canvas is the thing this feature exists to prevent.
- The one exception is `post` mode's shared fetch (below): if that fails, neither path has PRs to work with, so both are skipped and only the fetch error is returned.
- Both errors reach the action's exit code. A canvas that can't be written is a real failure of an opted-in feature, usually the one-time access setup (Step 1), and a warning in a green run is easy to miss for a surface nobody watches. Wrap `canvasErr` so the log names the canvas as the failing part, not the reminder.
- Message-path semantics are unchanged: the same conditions fail the run as today, with the same errors.
- Canvas refresh step: PRs (see fetch sharing below), then `prparser.ParsePRs`, then `canvascontent` (with `GeneratedAt: time.Now().UTC()` and the cap flags off the fetch result)/`canvasbuilder`/`slackclient.ReplaceCanvasContent`.
- `ParsePRs` is what applies `old-pr-threshold-hours`; the Slack user IDs it resolves go unread on the canvas.
- In `post` mode it runs twice per run, once on the message path's non-draft subset, once here on the full set. It touches no API and no clock beyond `time.Now()`, so the second pass is a copy, not a re-fetch.
- Failure-isolation tests: canvas failure + message success → run fails, message still sent and state still saved; message failure + canvas success → run fails, canvas still written; both fail → both errors reported. `testhelpers/mockslackclient` has no canvas-failure switch today, so `MockSlackClientOptions` gains `ReplaceCanvasError` alongside the message ones, and `ReplacedCanvas` gains a `Called` flag, so "never attempted" is asserted directly instead of inferred from an empty canvas ID.
- End-to-end tests in `cmd/pr-slack-reminder/canvas_test.go` (a new file, leaving `main_test.go`'s tables alone), asserting the markdown `mockslackclient` recorded (Step 1), nothing else covers the wiring from input to canvas:
  - `post` mode with the canvas on, grouped and flat, with drafts in the fixtures: the canvas carries both sections, the message carries only the non-draft PRs.
  - `update` mode with the canvas on: the canvas lists currently open PRs, not the state-tracked set, and the message still updates. The two fetches get deliberately different fixtures (an open PR absent from state, a state-tracked PR now merged), or the assertion proves nothing.
  - canvas off: `ReplaceCanvasContent` never called.
  - the fetch go/no-go, in three tests: a failed `post` fetch leaves the canvas untouched, a failed `update`-mode canvas fetch does the same while the message still updates, and a successful fetch of zero PRs refreshes the canvas. A wiped canvas is worse than a stale one, and "fetch failed" and "found nothing" are the same empty slice.
  - an over-cap fetch puts the `_Fetch limited to the newest …_` note on the canvas, pinning the cap flags end to end.
  - These assert on the parts of the markdown that matter (sections, rows, the footer line by pattern), not on the whole document: `GeneratedAt` is `time.Now().UTC()`, so an exact match is impossible here. `canvasbuilder`'s golden files (Step 6) cover the formatting.

#### Fetch sharing

`post` mode's message path already calls `FindOpenPRs` over the same repositories with the same filters; only the options differ. Share that one fetch instead of running it twice:

- `post`: one `FindOpenPRs(..., PRFetchOptions{IncludeDrafts: canvasOn})`, where `canvasOn` is `cfg.PRTrackerCanvasID != ""`. The message path takes the non-draft subset (filter on `pr.GetDraft()`, the same predicate `getPRFilterFunc` applies today); the canvas step takes the full set, and `canvascontent` prunes stale drafts from it (Step 5).
- `update`: two fetches, inherently. The message re-fetches state-tracked refs via `GetPRs`, which includes PRs now closed or merged (rendered struck-through / 🚀), those can never come from a list of open PRs. The canvas gets a second, separate `FindOpenPRs` with `IncludeDrafts` on.
- Placement: the canvas refresh takes a `githubclient.OpenPRsResult` as an argument and never fetches for itself, so it has one code path in both modes.
  - `post`: `runPostMode` keeps its own fetch and hands the result back, so `Run()` can pass it to the canvas refresh. Not lifted into `Run()` ahead of the mode switch: the fetch belongs to `post` mode only, so hoisting it would mean an `if cfg.RunMode == RunModePost` immediately followed by `switch cfg.RunMode`, branching on the mode twice.
  - Return a small result type, not a bare slice: `runPostMode(...) (postModeResult, error)` with `postModeResult{fetched githubclient.OpenPRsResult; prsFetched bool}`. `prsFetched` is the canvas's go/no-go.
  - A bare slice can't carry it: "fetch failed" and "fetch succeeded, found nothing" would both be an empty slice, and only the second may reach the canvas. Refreshing on the first would wipe the canvas to `_No open PRs_` every time GitHub is unreachable.
  - `OpenPRsResult` (R1) carries the cap flags the footer note needs (Step 5).
  - `runPostMode` returns `prsFetched: true` from every path after the fetch, its no-PRs early return included, so the canvas still refreshes when the message path stops early.
  - Timeout: `prFetchTimeout` (60s) is a function-local const in `runPostMode` and `runUpdateMode` today. Both `Run()`-level calls need their own `context.WithTimeout`, so lift it to package scope rather than adding a second copy. No separate canvas-on value is needed: see "Cost model" in Consequences for why the canvas's extra drafts don't meaningfully change worst-case fetch time under GraphQL's batching.
  - `update`: `Run()` calls `FindOpenPRs` for the canvas only, and only when the canvas is on. Not before the switch like `post`, it belongs to the canvas attempt (step 2 above), after `runUpdateMode` has already updated the message. A failed fetch becomes `canvasErr`, never an early return, so it cannot stop the message update. `runUpdateMode` is untouched.

What keeps `post` mode's message byte-identical when the canvas is on:

- The GitHub call itself is unchanged in shape: phase 1 (`listOpenPRs`) already returns drafts regardless of `IncludeDrafts` (see R1), so the option cannot change which PRs the single fetched page holds.
- Drafts are removed by exactly the predicate that excludes them today, before `prparser.ParsePRs`, so PR count, ordering, summary text and state saving all see the same set.
- Capping is per kind (R1), and drafts have their own lower cap, so they can't displace open PRs on an over-cap fetch.
- Filters, snooze exclusion and every other fetch-path step are untouched, `IncludeDrafts` only removes the draft check.
- `LastActivityAt` (Step 3) is populated for every PR regardless of the canvas, but the message never reads it, so its presence changes nothing observable.
- Canvas off → zero-value `PRFetchOptions` → today's call exactly.
- Regression test: a `post` run with the canvas on produces the same message blocks as the same fixtures with it off, drafts present in both, comparing the sent blocks JSON element by element. Since Step 8 removed the footer, the two block sets are identical, count included. It runs on mocks, so it proves the wiring, not live query shape (that rests on 001's own live verification of the shared query paths).

### 8. Reminder message: canvas link footer, built and then removed

Built as planned, then taken back out. The footer is gone; the reminder message is byte-identical whether the canvas is on or off.

What was built:

- `messagebuilder.BuildMessage` appended a trailing context block, `<URL|📋 PR tracker canvas>`, carried to it as `messagecontent.Content.CanvasURL` from `config.ContentInputs.CanvasURL`, and appended after truncation so a large message kept it.
- `limitMaximumMessageSize` reserved two blocks for it, capping at 48 instead of 50: grouped blocks run heading, list, spacer per repository, so cutting at 49 would leave the 17th repository's heading with no list under it.
- `slackclient` passed `slack.MsgOptionDisableLinkUnfurl()` and `slack.MsgOptionDisableMediaUnfurl()` on `PostMessage` and `UpdateMessage`, to keep the `slack.com` link from expanding into a preview card.

Why it was removed:

- Slack treats a canvas link as a **file share**, not a link unfurl, and attaches a canvas preview card under the message. Neither unfurl option affects file shares, so the card cannot be suppressed.
- The card lists the same PRs the message just listed, so the reminder said everything twice, with the duplicate the taller half.
- Nothing replaces it. The canvas is a tab in the same channel (Step 1), already one click from the message.

What the removal took out:

- `messagebuilder`: `addCanvasLinkBlock`, `getMaximumBlocks` and `maximumBlocksWithCanvasLink`, back to a single 50-block limit.
- `messagecontent.Content.CanvasURL` and its assignment in all three `GetContent` branches, which left `internal/messagecontent/messagecontent_test.go` (added by this step for exactly that) with nothing to test, so the file went too.
- `config.ContentInputs.CanvasURL`, and `Config.PRTrackerCanvasURL` with it: the footer was its only non-test reader, and it was never a parsed value, just the input echoed back. Its one remaining reader, `testhelpers/confighelpers.go`, passes `""` instead, since tests set the link through the overrides map anyway.
- Both unfurl options in `slackclient`, and the `mockSlackAPI` option recording added for their tests. The message returns to Slack's default unfurling, which is what `main` does today. PR titles are links inside rich text blocks, which Slack never unfurls, but `no-prs-message` reaches the top-level `text` field verbatim, so a URL put there would preview.

`Config.PRTrackerCanvasID` and `internal/config/canvaslink.go` stay untouched. The input still drives the canvas, only not the message.

Tests kept:

- A grouped over-limit message ends at exactly 50 blocks, and never on a repository heading with no PR list under it (`TestLimitMessageSizeByMaxBlocks`).
- Step 7's equivalence test now asserts the canvas-on and canvas-off runs produce identical message blocks, count included, which is the regression the removal protects.

### 9. Docs & permissions

- README: new "📋 PR Tracker Canvas" section, between "Filter Options" and "💡 Tips". It opens with what the canvas is and an example of the rendered markdown (copied from `internal/canvasbuilder/testdata/`, so it matches what the code renders, nothing enforces it), then a "Setup" sub-section (add a canvas tab to the reminder channel, ⋮ → Copy link, paste into `pr-tracker-canvas-link`, grant `canvases:write`) and a "Good to know" bullet list.
- Setup states the access requirement behind the channel-tab path (Step 1), so a canvas kept elsewhere can still be made to work. `canvases:write` added to the Slack scope table, marked "Only with `pr-tracker-canvas-link`". The GitHub permissions block stays as is: no new scope, the activity lookup rides the existing enrichment query (Step 3).
- "Good to know" carries: the action owns the whole canvas, every run replaces all content (Step 1), so hand-typed notes are lost, keep those on a second canvas; the canvas notifies nobody (see "Canvas content format"); one canvas per channel, since two workflows sharing one overwrite each other; which existing inputs shape canvas content (see "Existing inputs and the canvas"); and that a failed canvas write fails the run without stopping the reminder message (Step 7).
- Document the new input in the inputs table, linking to the new section.
- Exercise both the on and off paths end to end. No workflow-permission change needed.
  - The two workflows post to different channels, so they take **different** canvas links, each channel's own tab, not one shared canvas. Both would otherwise overwrite each other every run.
  - `.github/workflows/pr-reminder.yml` (posts to `#github`): `pr-tracker-canvas-link: https://hellej.slack.com/docs/T08SGDGNB2B/F0BPS4FKCEL`, the `#github` canvas, giving the scheduled runs a real, continuously refreshed canvas in both `post` and `update` mode.
  - `.github/actions/e2e-tests/action.yml` (posts to `#pr-reminders-test`): `pr-tracker-canvas-link: https://hellej.slack.com/docs/T08SGDGNB2B/F0BMEPVR1DL`, the `#pr-reminders-test` canvas, on the "Run with filters" step only, the richest case (multi-repository, `group-by-repository: true`, filters), covering grouped open PRs next to the always-flat draft list.
  - Leave it unset on the "Basic run" and "Multi-repository run" steps, so every release also verifies the action still behaves exactly as before when the input is absent.
  - The links go in as plain literals, like `slack-channel-name`, they aren't secrets, though they do carry the workspace name and team ID.
- Manual prerequisites, not automatable here. All confirmed in place on 2026-08-08:
  - `canvases:write` on the `pr_bot` app. One app covers both workflows, `pr-reminder.yml`, `build.yml` and `release.yml` all pass `secrets.DEV_SLACK_TOKEN`.
  - a "PR tracker" canvas tab in each channel: `#github` and `#pr-reminders-test`, both currently empty. The `#pr-reminders-test` link is the one quoted in Step 2; copy the `#github` one the same way (⋮ → Copy link) when writing the workflow.

### 10. Spec sync

- Update `githubclient.spec.md`, `slackclient.spec.md`, `prparser.spec.md`, `config.spec.md`, `messagecontent.spec.md`, `messagebuilder.spec.md` and `models.spec.md` (R2's `Repository.GetPullsURL()`) for the changes above.
- Add `canvascontent.spec.md` and `canvasbuilder.spec.md`.

## Consequences

### Cost model

Restating [001](001_GraphQL-migration.md)'s point-based GraphQL cost model for the canvas. Per-PR call-count figures don't apply to a GraphQL fetch, and no replacement numbers are given here that aren't already backed by 001's measurements, to avoid stating unverified figures as fact.

- Phase 1 (listing) is unaffected: it already returns drafts today, client-filtered out. Turning `IncludeDrafts` on doesn't change the query, so phase-1 cost (~30 points at the 30-repository cap, per 001's measurement) is unchanged.
- Phase 2 (enrichment) batches at 25 aliases per request, ~1-2 points per batch (per 001's measured boundary). Without the canvas, `MaxPRsToFetch` (50) means at most 2 batches. With it, up to `MaxDraftPRsToFetch` (15) more aliases means at most 3 batches, worst case.
- Those batches run concurrently at `defaultGitHubAPIConcurrencyLimit` (3), so 3 batches fit in the same single wave 2 batches already did. Worst-case phase-2 wall time is therefore unchanged (~`reviewsFetchTimeout`, one round trip): there's no per-PR fan-out to serialize.
- Net worst-case addition: roughly one extra 1-2 point batch per `post` run, against the 1,000 documented points/hour/repository (and the far larger budget 001 observed live, see that plan's "Observed budget"). No `prFetchTimeout` change is needed.
- `update` mode's canvas fetch is a second, independent `FindOpenPRs` call, its own phase 1 + phase 2, same shape as a `post` run's shared fetch.

### Positive

- An always-current PR/WIP view lives directly in Slack, with no need to open GitHub.
- Draft/WIP visibility, absent from the main reminder message by design, becomes available to whoever wants it.
- Fully additive and off by default: existing users see zero behavior change (zero-value fetch options, early-exit in `run.go`).
- The R1-R3 refactors close pre-existing test-coverage gaps (draft filter, repository grouping, old-PR/author-fallback display paths) independent of whether the canvas feature itself is used.
- Step 3 is a mapping change instead of a new per-PR API call, and the reviewer/comment fan-out needs no draft/open branching at all.

### Negative

- Opted-in `post` runs add up to one extra phase-2 batch (see "Cost model"); `update` mode also fetches twice for the canvas, its own phase 1 + phase 2.
- WIP rows can't show approvers by rendering choice (Step 6), even though the fetch now returns them for drafts too. Showing them is a rendering change only, no extra fetch needed.
- `post` mode's message path now runs a fetch shaped by a canvas input. Kept safe by an explicit equivalence test and per-kind capping (Step 7), but it is a coupling that didn't exist before.
- Canvas access can't be granted by the action itself. The intended setup (a canvas tab in the reminder channel) makes it implicit, but a canvas kept anywhere else needs manual sharing.
- The canvas is a golden-file surface: any formatting change shows up as a snapshot diff to regenerate (Step 6). Intended, but it does make cosmetic tweaks a two-step change.
- A canvas row is one markdown string, so every GitHub-sourced value needs escaping (Step 6), a class of bug the Block Kit message can't have, and one that only shows up on titles containing markdown characters.
- Opting in adds a way for the run to fail: a canvas write that can't be done fails the action even though the reminder was posted (Step 7). Deliberate, the alternative is a feature that silently stops working, but it means a Slack-side access change turns scheduled runs red.
- Two new packages (`canvascontent`, `canvasbuilder`) largely mirror existing ones (`messagecontent`, `messagebuilder`), adding maintenance surface for a feature many users won't enable.

### Neutral

- Activity and draft-staleness thresholds are hardcoded, not configurable, in v1.
- The canvas names people instead of mentioning them, so it notifies nobody. The reminder message stays the notifying surface.
- `LastActivityAt` is now populated on every enriched PR, canvas on or off, since the underlying GraphQL field is always selected. Unused outside the canvas path.
