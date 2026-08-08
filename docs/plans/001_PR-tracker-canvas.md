# PR tracker canvas

date: 2026-08-08
status: draft

The "PR tracker canvas": a Slack canvas this action keeps updated with a live view of open + WIP PRs. Use this name in user-facing text (input description, README).

## Goals

- Motivation: the reminder message is transient and drafts-free by design, and even GitHub itself has no single "live" view of a team's open + WIP PRs (incl. their statuses/staleness/reviews) across multiple repos — the canvas fills both gaps with a persistent, on-demand, cross-repo view in Slack.
- New optional feature: keep a Slack canvas continuously showing open PRs (oldest first) and draft/WIP PRs (most recent activity first).
- Canvas always includes draft PRs (the main reminder message never does — unchanged).
- Open PR rows carry the same fields in the same order as the reminder message's rows, except how authors and reviewers are rendered (see "Canvas content format").
- WIP rows show last-push activity instead of age: drafts are ordered by activity, as the last push predicts which draft gets opened for review next better than its age does.
- Draft PRs inactive for more than 2 months are always excluded.
- One new input, off by default, no other new configurability.
- The input takes the canvas link the user copies from Slack; the action parses the ID out of it.
- The reminder message links to the canvas, so the transient message is a way in to the persistent view.

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

The two section headings are fixed strings, rendered as `## Open PRs` and `## Work in Progress` (shown bold above only so this example renders). When grouping is on, each repository is an `###` sub-heading under `## Open PRs` — no repeated top-level heading per repository.

The canvas closes with a blank line, a `---` divider and a `_Updated <YYYY-MM-DD HH:MM UTC>_` line, preceded by a `_Showing the newest 50 …_` line when the fetch cap trimmed a section (Step 5). A canvas nobody has refreshed — action disabled, failing, or not yet run — otherwise looks identical to a current one, which defeats the point of a live view. UTC because there is no timezone input.

No top-level heading. The canvas title lives on the tab, not in the document, and survives a full content replace (verified on `#pr-reminders-test`, 2026-08-08).

An empty section keeps its heading and shows one italic line instead of rows (Step 5).

Authors and reviewers are plain names — GitHub name, falling back to username — never Slack mentions. Every run replaces the whole canvas, and each replace would re-notify everyone mentioned.

A WIP row is: linked title, author, reviewers, activity chip (Step 3), then 💤 if idle for +48h. Never the 🚨 old-PR marker — it nags about review latency, which doesn't apply to work nobody has been asked to review yet.

"updated" is reader-facing wording only: the chip is backed by the head commit's committer date, not GitHub's `updated_at` (Step 2). Don't "fix" the mismatch by switching the data source — `updated_at` serves only as a fallback when the head commit can't be fetched.

### Existing inputs and the canvas

Which of today's inputs shape canvas content:

| Input | Canvas | Note |
| --- | --- | --- |
| `filters`, `repository-filters` | applies | same fetch path as the message (Step 7) |
| `github-repositories`, `GITHUB_REPOSITORY` | applies | same repository set |
| `github-user-slack-user-id-mapping` | ignored | no mentions on the canvas, names come from GitHub |
| `old-pr-threshold-hours` | applies | drives 🚨 on open rows; never on WIP rows |
| `group-by-repository` | applies to the open section only | WIP list is always flat |
| `pr-list-heading` | ignored | canvas headings are fixed (see "Canvas content format") |
| `no-prs-message` | ignored | fixed fallback text (see Non-goals) |
| `run-mode`, `state-artifact-name` | ignored | canvas always shows all currently open + WIP PRs, never the state-tracked set `update` mode's message re-fetches (Step 7) |
| `slack-channel-name`, `slack-channel-id` | ignored | the canvas is addressed by ID (parsed from `pr-tracker-canvas-link`), not by channel |

Not an input, but same question: `/snooze ... for N days` comments apply to the canvas too, drafts included — snoozed PRs are dropped inside `githubclient`, before any content package sees them.

## Non-goals

- Configurable thresholds (activity windows, draft staleness) — hardcoded for now.
- Auto-creating or discovering a canvas — the user creates it themselves in Slack and pastes its link; the action never creates, deletes, or looks one up by channel.
- Splitting canvas content if it exceeds Slack's canvas size limits. Deferred: an oversized canvas fails the write, and so the run, until the PR count drops. The logged markdown length (Step 6) is the clue. Revisit if it ever happens — the fetch caps at 50 per kind (R1).
- Persisting canvas identity in `state` — the ID is supplied fresh via input every run, nothing to persist.
- A "no PRs" message input for the canvas (a fixed fallback string is used instead).
- A canvas-only mode. The message path always runs, and `pr-list-heading` stays required when `group-by-repository` is false.
- Closed PRs on the canvas, struck through or otherwise — the canvas refresh fetches open PRs only. Deferred, not rejected: Steps 3-5 keep the seams for a third section (see "Room for a recently-closed section").

### Room for a recently-closed section

A future third canvas section listing recently closed/merged PRs stays additive. What it would need, and what this plan already covers:

- `githubclient`: a new list path — `PullRequestListOptions{State: "closed", Sort: "updated", Direction: "desc"}`, stopping at a time window. This is genuinely new code, not a `PRFetchOptions` flag: open PRs are bounded by "still open", closed PRs need a cutoff. Inherent to the feature; nothing here makes it worse. `FetchActivityTimestamps` stays off for it — `ClosedAt`/`MergedAt` are already on the fetched PR.
- `prparser`: reuses Step 3's keyed newest-first sort with `ClosedAt`.
- `canvascontent`/`canvasbuilder`: one more section field and one more `renderSection` call (Steps 4-5).
- Row markers: `canvasbuilder` reads the existing `PR.IsMerged()`/`PR.IsClosedButNotMerged()` booleans, and canvas markdown has `~~strike~~` for the closed case (verified 2026-08-08, Step 5). R3 leaving the message's strike-through and 🚀 rendering in `messagebuilder` costs nothing here — only the marker *elements* are Block Kit-specific, not the conditions.
- `run.go`: a closed-PR list is its own fetch either way, so it lands next to the canvas refresh without touching the message path's fetch sharing (Step 7).

## Target shape

- `action.yml`: new optional input `pr-tracker-canvas-link` (string, no default). Empty/unset → feature off, matching the "empty means unused" requirement literally. The user creates and owns the canvas entirely themselves (see Non-goals).
- `config`: parses the link once into a canvas ID + canvas URL, so no other package parses it again. Both are always known when the feature is on.
- `githubclient`: fetching gains an explicit `PRFetchOptions{IncludeDrafts, FetchActivityTimestamps}` struct (zero value = today's behavior exactly). When set, drafts survive the fetch and each draft gets a `LastActivityAt *time.Time` from its head commit (one extra GitHub API call per draft — see Step 2).
- `prparser`: `PR` gains activity display helpers (based on `LastActivityAt`) and a most-recent-activity sort, used only by the draft section.
- New `canvascontent` package (mirrors `messagecontent`; Go code stays medium-named, `canvas*`, no need to spell out "PR" internally): builds a canvas-ready `Content` — open PRs (oldest first, grouped/flat per `group-by-repository`) + draft PRs (always flat regardless of that input, most-recent-activity first, >2 months inactive excluded).
- New `canvasbuilder` package (mirrors `messagebuilder`): renders `canvascontent.Content` to Slack canvas markdown, reusing display-text helpers extracted from `messagebuilder` in the pre-refactor.
- `messagecontent`/`messagebuilder`: the reminder message gains a trailing canvas link row whenever the feature is on.
- `slackclient`: gains a method that fully replaces a canvas's content by ID — one `canvases.edit` call, `replace` operation, `section_id` omitted.
- `run.go`: if `pr-tracker-canvas-link` is set, gets open + draft PRs and overwrites the canvas. `post` mode shares the message's fetch (with drafts and activity timestamps switched on); `update` mode fetches separately, since its message path is state-tracked (Step 7). The canvas refresh and the message path are independent attempts — either can fail without stopping the other — and their errors are joined at the end, so the action fails if either did.
- Permissions:
  - Slack: one new scope, `canvases:write` (canvas-only). On top of it, a one-time manual step — the user creates the canvas as a tab in the reminder channel (see Step 6), since the scope alone doesn't grant write access to a specific canvas.
  - GitHub: no change. The activity lookup runs on `pull-requests: read`, already required.

## Breaking change classification

Non-breaking / **minor** release. New optional input, default off, no change to existing inputs, outputs, or default behavior.

## Summary of steps

- R1. Refactor: make draft-exclusion in `githubclient` explicit and optional instead of hardcoded
- R2. Refactor: extract repository-grouping into a shared, reusable helper (closes a test-coverage gap)
- R3. Refactor: extract medium-agnostic PR display text out of `messagebuilder` (closes two test-coverage gaps)
- R4. Refactor: log a warning when the PR fetch times out, instead of degrading silently
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
- `capPRsToLimit` must cap drafts and non-drafts separately when `IncludeDrafts` is on (each up to `MaxPRsToFetch`), not the combined slice. The cap keeps the newest by creation date, so a shared capped slice would let drafts displace open PRs the message would otherwise show. Untouched when `IncludeDrafts` is off.
- `FindOpenPRs` returns `(OpenPRsResult, error)` instead of `([]PR, error)`: `OpenPRsResult{PRs []PR; OpenPRsCapped, DraftPRsCapped bool}`, each flag set by `capPRsToLimit` when it actually trimmed that bucket. The canvas footer note (Steps 4-5) needs to know the cap fired, and no downstream length can tell: `excludeSnoozedPRs` runs after the cap (`githubclient.go`), and `canvascontent` prunes stale drafts on top, so a capped fetch routinely arrives as fewer than 50. `GetPRs` keeps `([]PR, error)` — nothing reads its cap.
- The draft bucket sorts by `updated_at` desc, not creation date, so an over-50 draft fetch keeps the most recently active ones — the set the canvas orders and prunes by (Steps 3-4). Not the head-commit date: capping runs before `addReviewerInfoToPRs`, so `LastActivityAt` doesn't exist yet, and `updated_at` is the best proxy available at that point. The non-draft bucket keeps today's creation-date sort.
- `logFoundPRs`'s "Found %d open pull requests" gains a draft count when `IncludeDrafts` is on. Log text only.
- Test coverage check: already good — `githubclient_test.go` has an explicit "draft PR should be filtered out" case. No gap here. Its five `FindOpenPRs`/`GetPRs` call sites take the new argument, and the `FindOpenPRs` ones read `.PRs` off the result, but no assertion changes. Add table-driven cases: `IncludeDrafts: true` lets a draft PR through, `IncludeDrafts: true` with over `MaxPRsToFetch` PRs of both kinds keeps the same non-draft set as `IncludeDrafts: false` would, and both cap flags set only for the bucket that overflowed.
- `testhelpers/mockgithubclient` needs no change: it mocks the GitHub *services*, not `githubclient.Client`, so the interface change flows through `NewClient` on its own.

### R2. Refactor: shared repository-grouping helper

- Extract `messagecontent.groupPRsByRepositories`'s bucketing (by repository path, alphabetical) into an exported helper on `prparser`, since `canvascontent` needs identical grouping for its open-PR section.
- The helper returns `[]prparser.RepositoryPRs` (`{Repository models.Repository; PRs []PR}`) and no display strings. `HeadingPrefix` and `RepositoryLinkLabel` stay in `messagecontent`, which maps the groups into its existing `PRsOfRepository`. `messagecontent.PRsOfRepository` and `messagebuilder` are untouched, and Block Kit wording stays out of `prparser`.
- Move the pulls-page URL onto `models.Repository` as `GetPullsURL()` (`https://github.com/<path>/pulls`), next to `GetPath()`. `messagecontent` and `canvasbuilder` (Step 5) both call it, so the format string isn't written out in two packages.
- Test coverage gap found: there is no `messagecontent_test.go` at all — grouping is only exercised indirectly through `messagebuilder_test.go`'s fixed two-repository example and `main_test.go`'s integration cases. Add a direct unit test for the extracted helper (alphabetical ordering with out-of-order input, link format, single- and multi-repository bucketing) before/while moving it, so the extraction has a real safety net instead of only indirect coverage.
- `messagecontent` calls the extracted helper; behavior and existing tests unchanged.

### R3. Refactor: shared PR display text

- Extract from `messagebuilder` into methods on `prparser.PR` (or a small shared helper file), as plain strings/booleans instead of `slack.RichTextSectionElement`s:
  - reviewers summary text (✅/💬 grouping of approvers/commenters)
  - age text: `N days old` when `PR.IsOldPR`, `N days ago` otherwise. Text only — the 🚨 marker stays in each builder, which places it outside its own styled element (bold+code in `messagebuilder`, a code span in `canvasbuilder`). Open PR rows only; the WIP section must not call it (see Goals)

  The closed-but-not-merged and merged markers stay in `messagebuilder` — only the message ever renders a closed or merged PR (see Step 5).
- Author display is not extracted: the message prefers a `<@ID>` mention and falls back to `GetGitHubName()`, the canvas always uses `GetGitHubName()` (see Step 5). Both fields are exported, so each builder reads them directly.
- Unit coverage to add in `messagebuilder_test.go` before extracting, so a regression is caught at the package that owns the rendering:
  - the old-PR warning-marker path (`IsOldPR: true` → 🚨 + bold/code age text)
  - the author-fallback path (no mapped `SlackUserID` → GitHub display name instead of a mention)
  - the merged 🚀 and closed-but-not-merged strike-through markers
  - Both the first two are asserted end to end today (`main_test.go`'s `"🚨 1 days old by U3234567890"` and `"🚨 2 days old by Jim"`), so the extraction has a safety net either way — these tests make the failure land in the package that broke.
- Unit coverage to add for the extracted helpers themselves, table-driven: reviewers summary text across no reviewers / approvers only / commenters only / both, and age text on either side of the old-PR threshold.
- `messagebuilder` wraps the extracted plain strings in its Block Kit elements; behavior and existing tests unchanged. This lets `canvasbuilder` (Step 5) reuse the exact same text instead of re-deriving it.

### R4. Refactor: name the timed-out PR fetch in the log

- `addReviewerInfoToPRs` swallows every inner error: each `prProcessingGroup.Go` returns nil, and per-PR failures are only logged (`models.go`'s "Unable to fetch reviews/comments for PR #%d"). `prProcessingGroup.Wait()` therefore never returns an error, and a blown deadline looks the same as PRs that genuinely have no reviewers.
- An expired *outer* fetch context hits every PR at once: they come back with no reviewers **and no timeline comments**, so `excludeSnoozedPRs` stops excluding and snoozed PRs reappear in the reminder message, with nothing in the log naming the cause.
- Fix the silence, not the outcome — in `addReviewerInfoToPRs` directly after `prProcessingGroup.Wait()`:

  ```go
  if ctx.Err() != nil {
      log.Printf(
          "Warning: PR fetch deadline exceeded (%v) while fetching reviewer data for %d PRs - "+
              "reviewer data is incomplete and snoozed PRs may reappear in this reminder",
          ctx.Err(), len(prResults),
      )
  }
  ```

  `len(prResults)` is what went in, not what came back — every goroutine sends a result either way, so a "processed" count would be the same number and say nothing. Word it as the size of the attempt.

  `ctx` is the caller's fetch context (`prFetchTimeout`), not the errgroup's derived `prProcessingCtx` — the errgroup cancels its own child on `Wait`, never `ctx` — so this fires only when the budget actually ran out.
- Deliberately not fatal: a reminder with missing reviewers beats no reminder at all. The log line is the whole diagnosis.
- Independent of the canvas, but the canvas makes the path more likely by doubling the fetch (Step 2).
- Per-PR failures unrelated to the deadline (a single 403, one flaky call) are untouched, still one "Unable to fetch reviews/comments" line each.
- Test: a mock PR service that blocks past a short fetch timeout → `FindOpenPRs` still returns its PRs, and the warning is logged. Capture via `log.SetOutput` to a buffer.

### 1. New input: `pr-tracker-canvas-link`

- `action.yml`: add optional string input, no default. Description states the intent plainly, e.g. "Link to a Slack canvas to keep updated with a live tracker of open and work-in-progress pull requests. Open the canvas in Slack → ⋮ → Copy link. Leave empty to disable (default)."
- `internal/config/config.go`: add constant `InputPRTrackerCanvasLink`, read via `inputhelpers.GetInput`, add `Config.PRTrackerCanvasID` and `Config.PRTrackerCanvasURL`, both empty when the input is unset.
- `config.ContentInputs` gains `CanvasURL`, the same value as `Config.PRTrackerCanvasURL`. `messagecontent.GetContent` receives only `ContentInputs`, never the whole `Config`, and Step 8 needs the URL there. The ID stays off `ContentInputs` — only `run.go` reads it.
- Link shape, confirmed against the real canvas tab in `#pr-reminders-test`: `https://<workspace>.slack.com/docs/<TEAM_ID>/<CANVAS_ID>`, i.e. `https://hellej.slack.com/docs/T08SGDGNB2B/F0BMEPVR1DL`.
- Parse: `url.Parse`, then require a `docs` segment in `URL.Path` and take the first segment after it matching `^F[A-Z0-9]+$`. Scanning survives a trailing title slug; `TEAM_ID` starts with `T` and slugs are lowercase, so neither can match. No length bound — real IDs already run to 13 characters. Reading `URL.Path` drops query params for free.
  - Requiring `docs` rejects other Slack links that carry an `F…` file ID (`/files/…`), which would otherwise parse and then fail at the `canvases.edit` call.
  - Assumed, on one verified link: every copy path gives a `docs` segment. Accepted for v1.
  - Must be an absolute URL with a host; a bare `F…` ID is rejected. `PRTrackerCanvasURL` is the input as given, never rebuilt, and Step 8 puts it straight into the message footer — a bare ID there is a dead link.
  - A non-empty input that doesn't parse is a hard config error, joined like the other input validation. A typo would otherwise mean a silently missing canvas. The message names the expected shape and the Copy link path.
- Test cases: the real link above, trailing slash, query params, a trailing title slug, `http`, an unrelated URL, a Slack `/files/F…` link (rejected), a bare `F…` ID (rejected), empty (feature off).
- `testhelpers/confighelpers.go`: mirror the new input.
- `go run .github/scripts/check_inputs.go` must still pass.

### 2. `githubclient`: last-activity lookup

- Add `ListCommits(ctx, owner, repo string, number int, opts *github.ListOptions) ([]*github.RepositoryCommit, *github.Response, error)` to the existing `GithubPullRequestsService` interface — matches `(*github.PullRequestsService).ListCommits` in `go-github/v78` exactly. No new service to wire in: `client`, `NewClient` and `GetAuthenticatedClient` are unchanged.
- No new GitHub permission: [the endpoint's docs](https://docs.github.com/en/rest/pulls/pulls?apiVersion=2022-11-28#list-commits-on-a-pull-request) state it needs `"Pull requests" repository permissions (read)`, which the action already requires.
- Add `PRFetchOptions.FetchActivityTimestamps bool`, and thread `PRFetchOptions` into `addReviewerInfoToPRs` too — R1 stops at `FindOpenPRs`/`GetPRs`/`getPRFilterFunc`, and the fan-out below is where this flag is read. When set, **drafts only** get a 4th concurrent call in `addReviewerInfoToPRs`'s per-PR fan-out, alongside the existing three (reviews, comments, timeline comments): resolve the PR's newest commit and record its `commit.committer.date` — the true last-push time, unlike `updated_at` (which also changes on comments, labels, etc.). In Go: `RepositoryCommit.Commit.Committer.Date`. Not `RepositoryCommit.Committer` — that's a `*User`, whose `CreatedAt` is the account's, so the wrong reach compiles and yields a nonsense timestamp. Add the resolved timestamp to `models.go`'s existing per-PR "Found %d reviews, %d PR comments…" line rather than logging a new one per PR.
- Gate on `pr.GetDraft()` in the fan-out. Only WIP rows read `LastActivityAt` (Steps 4-5), so fetching it for open PRs would be up to 50 wasted calls per run. Filtering and capping already ran by then, so filtered-out PRs cost nothing. Snoozed drafts still cost one call — the snooze isn't known until their timeline comments come back in the same fan-out.
- Resolution, one call per PR:
  - `ListCommits` with `ListOptions{PerPage: 100}`. If `response.NextPage != 0` (over 100 commits), refetch at `response.LastPage`. `LastPage` is 0 when the `Link` header carries no `last` rel — refetching page 0 then returns page 1 again, its last element isn't the head commit, and the SHA check below catches it. Don't add a separate guard.
  - Take that page's last element: the endpoint returns commits oldest first, head commit last (verified on PRs of 2-79 commits across four repositories).
  - Confirm its SHA equals `pr.GetHead().GetSHA()`, since the order isn't documented and the endpoint caps at 250 commits (above which the head commit is never returned). Getters, not `pr.Head.SHA` — `Head` is nil on every PR built by today's `getTestPR`, and the field reach panics there.
  - Fall back to `pr.GetUpdatedAt()`, already on the fetched PR, whenever the head commit can't be pinned down: SHA mismatch, an absent head SHA, a failed or empty `ListCommits` call. It overstates freshness, but it's an upper bound on the last push, so a busy long-lived draft never drops off the canvas via the staleness rule. Log a warning naming the repository, PR number and reason, then carry on — the existing "partial failure" pattern for reviews/comments (`models.go`'s "Unable to fetch reviews/comments for PR #%d"), never failing the whole PR.
  - Leave the timestamp unknown when `updated_at` is zero too. `GetUpdatedAt()` returns a zero `Timestamp` when the field is absent, which would render as `idle 700000 days` and drop the draft as inactive. Today's `getTestPR` fixtures set neither `Head` nor `UpdatedAt`, so this is the default in tests, not a corner case.
- Add `PR.LastActivityAt *time.Time`: head-commit date, else `updated_at`, else nil. Also nil for every non-draft PR, and for every PR when `FetchActivityTimestamps` is off. Carry it on `FetchReviewsResult` too — `asPR()` builds the `PR`.
- Fixtures and mocks: `mockPullRequestService` gains per-PR commits, and PRs used in canvas tests get `Head.SHA` plus a matching commit. Existing fixtures stay as they are — they exercise the nil path.
- Extend `testhelpers/mockgithubclient`'s `mockPullRequestService` with `ListCommits`.
- `ReviewsFetchTimeout` (10s) is unchanged — the 4th call runs concurrently with the other three.
- `run.go`'s `prFetchTimeout` must be raised when the canvas is on: 60s → 120s. R1's per-kind cap doubles the fetch to up to 100 PRs, each with the same 3-call fan-out plus a 4th for drafts, all at `DefaultGitHubAPIConcurrencyLimit` (3) — ~34 sequential rounds instead of ~17. Doubling keeps the headroom 60s already leaves at 50 PRs, rather than adding new slack. Wiring in Step 7.
- Overrunning it still degrades rather than fails, but no longer silently: R4 logs a warning naming the deadline. The degradation itself — PRs with no reviewers and no timeline comments, snoozed PRs back in the message — is what Step 7's equivalence test cannot catch, since that test runs on mocks.

### 3. `prparser`: activity text + activity sort

- Extract `GetPRAgeText`'s minutes/hours/days formatting into a duration → text helper. It reads `CreatedAt` directly today, so the activity chip can't reuse it as is. Wording stays as it is, including the `1 days` it produces at 24h — the chip inherits it as `idle 1 days`, matching the message's `1 days old`.
- Add `PR.GetActivityText() string`, from `LastActivityAt` via that helper: `updated N minutes/hours ago` under 24h, `idle N days` at 24h and above.
- Add `PR.IsIdle() bool`: true when `LastActivityAt` is older than a hardcoded 48h.
- `LastActivityAt` can be nil even with the canvas on (Step 2). Nil means unknown, not stale: empty chip text, not idle, never dropped by the staleness rule (Step 4), sorted after every PR with a real timestamp.
- Add an exported newest-first sort for use by the draft section, distinct from `ParsePRs`'s existing oldest-first sort. Take the timestamp as a key function (`func(PR) *time.Time`) rather than reading `LastActivityAt` directly, so sorting by any other timestamp needs no second sort. Nil keys sort last.
- Tests: the duration helper's three magnitudes and their boundaries, both chip wordings, `IsIdle` around 48h, and the sort with a nil key mixed in.

### 4. `canvascontent` package

- Mirrors `messagecontent.GetContent`'s shape but for canvas: takes all fetched PRs (open + draft) and content inputs, and produces open-PR content (oldest first, grouped/flat per `group-by-repository`, via the R2 helper) plus draft-PR content (always flat, most-recent-activity first, regardless of `group-by-repository`).
- `GetContent` takes one `[]prparser.PR` and splits it itself on `pr.GetDraft()` — drafts to the WIP section, the rest to the open section. `run.go` passes the fetch result unsplit, so the split lives in one place and the caller stays the same in both run modes. Alongside it, the two cap flags and `GeneratedAt` — group them in one `GetContentOptions` struct rather than growing the parameter list per section.
- `Content` carries `GeneratedAt time.Time` for the footer line, set by the caller (Step 7) rather than read from the clock here, so `canvasbuilder`'s output is deterministic under test (Step 5).
- Keep the two sections as separate named fields on `Content`, not one merged list with a per-PR kind flag, so a third section is a field rather than a rework.
- Section headings are fixed strings owned by `canvasbuilder` (Step 5), so `contentInputs.PRListHeading` is unread here — no `<pr_count>` substitution, no "required when `group-by-repository` is false" coupling.
- Excludes draft PRs whose `LastActivityAt` (not creation time) is older than a hardcoded `MaxDraftPRInactivity` (2 months / 60 days). A nil `LastActivityAt` is kept — unknown is not stale (Step 3).
- No whole-canvas "nothing to show" case: each section falls back on its own, so the both-empty canvas is just both fallbacks (see Step 5).
- `Content` carries `OpenPRsCapped` / `WIPPRsCapped bool` for the footer note (Step 5), passed in by the caller (Step 7) from `githubclient.OpenPRsResult` (R1). Never derived from `len(section)` (R1) — the staleness prune above shrinks the WIP list further still.
- Log the counts put on the canvas: open PRs, drafts, and drafts dropped as inactive — otherwise a missing draft has no explanation.
- Tests: the staleness cutoff on both sides, a nil-activity draft kept, drafts ordered newest-activity first, open PRs grouped and flat, and each cap flag reaching `Content` while its section holds fewer than 50 PRs.

### 5. `canvasbuilder` package

- Renders `canvascontent.Content` to a Slack canvas markdown string (`slack.DocumentContent{Type: "markdown", ...}`), reusing the R3 display-text helpers.
- Canvas `document_content` takes real markdown, not Slack `mrkdwn`: `**bold**`, `_italic_`, `[label](url)`, backtick code spans, `~~strike~~`, `##`/`###` headings, `-` bullets, `---` dividers ([Canvases docs](https://docs.slack.dev/surfaces/canvases/)). All of it rendered correctly in live `canvases.edit` replaces against the `#pr-reminders-test` canvas tab, 2026-08-08 — including this plan's own fixed strings (`_No open PRs_`, `_Showing the newest 50 open PRs_`, the `---` divider and the italic `_Updated …_` footer).
- No Slack mentions: `canvasbuilder` never reads `pr.Author.SlackUserID`, and renders authors and reviewers through `GetGitHubName()`. Canvas markdown does support `![](@USERID)` — don't switch to it (see "Canvas content format").
- Escape every GitHub-sourced string before it goes into markdown — PR titles, author and reviewer names, repository paths. Block Kit keeps text and styling in separate fields, so nothing in a title can be read as formatting there; a canvas row is one string, where it can. `Add _debug_ flag` italicizes, `**WIP** rewrite` bolds, `` Use `make test` `` code-spans, and `Fix [ABC-123] crash` breaks the link label.
  - Add `escapeMarkdown(string) string` in `canvasbuilder`, backslash-escaping `\`, `` ` ``, `*`, `_`, `[`, `]`, `~`. Backslash first, or it double-escapes what the later replacements add.
  - Link targets need no escaping: they come from GitHub (`GetHTMLURL()`, `GetPullsURL()`) and can't contain a space or a `)`.
  - **Verified in the same probe**, raw rows next to escaped ones. Slack's canvas parser accepts [CommonMark](https://spec.commonmark.org/0.31.2/#backslash-escapes) backslash escapes even though it documents none: every one of `\`, `` ` ``, `*`, `_`, `[`, `]`, `~` was consumed and its character survived, bold and code spans were suppressed, and escapes held inside link labels too. No code-span fallback needed.
  - Delimiter behaviour, same probe, for anyone tempted to trim the escape set:
    - `_` is emphasis only at word boundaries: `Add _debug_ flag` italicizes, a raw `fix_the_thing_now` renders as-is. Keep `_` — the first case is a real PR title.
    - Strikethrough is `~~`, not `~`: `Remove ~~legacy~~ shim` strikes, a lone `~strike~` renders literally. Keep `~`.
    - Spaced delimiters are inert (`a * b * c`, `1 _ 2 _ 3`, `50% * 2` all render as typed). Escaping them anyway costs nothing and keeps the helper free of context rules.
  - Escaping belongs to `canvasbuilder`, not `canvascontent`: it's a property of the output format, and a third renderer would want its own rules.
- Open PR rows: linked title, age text with the old-PR warning marker, author, reviewers. No strike-through, no 🚀 — the canvas fetch lists open PRs only, so a closed or merged PR can never reach a row here (see Non-goals). Those two markers stay message-only.
- WIP rows: linked title, author, reviewers, `PR.GetActivityText()` as a code span, then 💤 if `PR.IsIdle()`. No age text, no 🚨, no 🚀 (see Goals). Empty activity text (Step 3) renders no chip and no 💤, leaving the row title-author-reviewers.
- Structure: a fixed `## Open PRs` heading and its list, then a fixed `## Work in Progress` heading and the draft list. Both headings always render, `group-by-repository` or not.
  - Grouped: each repository is an `###` sub-heading under `## Open PRs`, linking to the repository's pulls page. `canvasbuilder` builds that heading from `RepositoryPRs.Repository`, taking the URL from `Repository.GetPullsURL()` (R2) — no "Open PRs in " prefix, which would repeat the parent heading.
- Render both through one internal `renderSection(heading string, prs []prparser.PR, renderRow func(prparser.PR) string, emptyText string)`, not two bespoke paths. The two sections differ only in their row renderer and empty text, and a third section then costs one call.
- Empty section: heading still renders, followed by one italic line — `_No open PRs_` / `_No work in progress_`. An empty section means "nothing here right now", which a missing heading can't say; it would read as a broken render instead. Grouped mode with no open PRs renders the same single line, no repository sub-headings.
- Footer, after both sections: blank line, `---`, then `_Updated <YYYY-MM-DD HH:MM UTC>_` from `Content.GeneratedAt` (see "Canvas content format").
- Above the `Updated` line, when either cap flag is set (Step 4), one italic line naming what was cut: `_Showing the newest 50 open PRs_` / `_Showing the newest 50 WIP PRs_` / both in one line. Otherwise a capped canvas silently misses PRs, and only the run log says why.

#### Snapshot tests

The canvas is one markdown string, so golden files cover its formatting more cheaply and more completely than element-level assertions.

- `internal/canvasbuilder/testdata/*.md` hold the expected output, one file per case: grouped, flat, empty open section, empty WIP section, both empty, an old PR, an idle draft, a draft with unknown activity, a capped section, and a PR whose title and author name carry `_`, `*`, `[`, `]`, `~`, `` ` `` and `\`.
- The test compares byte-for-byte, and rewrites the golden file instead when `-update` is passed (`flag.Bool("update", false, …)`). A deliberate format change is then `make update-canvas-snapshots` plus a reviewable diff, not a hand-edited expectation.
- Add that Makefile target: `go test ./internal/canvasbuilder -update`.
- Determinism: `Content.GeneratedAt` is fixed by the test (Step 4), and PR fixtures set `CreatedAt`/`LastActivityAt` as offsets from `time.Now()` — `prparser` reads the clock directly, and offsets keep the rendered age and activity text stable.
  - Keep those offsets clear of every boundary the rendering rounds or thresholds on: 1h and 24h (`GetPRAgeText`), 48h (`IsIdle`), 60 days (`MaxDraftPRInactivity`), and any half-unit `math.Round` flips on. `30m`, `5h`, `3d`, `10d` are safe; `24h` and `1h30m` flake, since the clock advances between fixture construction and render.
  - Give canvas fixtures a real `HTMLURL`. `getTestPR` sets none, so a fixture copied from it renders `[title]()` into the golden file.

### 6. `slackclient`: full canvas content replace

- Add `EditCanvas` to `SlackAPI` (exists in `github.com/slack-go/slack` v0.27.0 — confirmed in source).
- Add `Client.ReplaceCanvasContent(canvasID, markdown string) error`: one `EditCanvas` call (`EditCanvasParams{CanvasID, Changes: []CanvasChange{{Operation: "replace", DocumentContent: ...}}}` — confirmed struct shape in `slack-go/slack` v0.27.0's `canvas.go`), `SectionID` left at its zero value (`""`, omitted on the wire via `omitempty`), `DocumentContent{Type: "markdown", Markdown: markdown}`. Confirmed via [Slack's `canvases.edit` docs](https://docs.slack.dev/reference/methods/canvases.edit/#content-operations): omitting `section_id` on a `replace` operation replaces the entire canvas in one call, and the method needs only the `canvases:write` scope.
- Two mocks to extend, for two different interfaces:
  - `testhelpers/mockslackclient.MockSlackAPI` implements `slackclient.Client` despite its name — it gains `ReplaceCanvasContent`, recording the canvas ID and markdown so `main_test.go` can assert on canvas content end to end.
  - `mockSlackAPI` in `internal/apiclients/slackclient/slackclient_test.go` is the `SlackAPI` mock — it gains `EditCanvas`, capturing the params and returning a configurable error.
- Log the canvas ID and markdown length before the call, confirm on success — matching `SendMessage`/`UpdateMessage`. The length is the only clue if content hits Slack's canvas size limit.
- On error, wrap it with a concise hint: `"canvas update failed: check that the bot has canvases:write permission and is invited to the channel where the canvas is"` — this is what surfaces in the failed run's log (Step 7), so it carries the whole diagnosis.
- **Scope alone isn't sufficient for a canvas kept elsewhere**: canvases have their own access control (read/write/owner), separate from OAuth scopes ([`canvases.access.set` docs](https://docs.slack.dev/reference/methods/canvases.access.set)). A canvas the user creates outside the reminder channel gives the bot no access, and `canvases.edit` fails until it is shared. The channel tab below is what avoids that.
- **Intended setup**: create the canvas as a tab in the reminder channel itself. Verified end to end on 2026-08-08: a canvas the user created as a tab in `#pr-reminders-test`, edited by the `pr_bot` token carrying `canvases:write` and nothing canvas-related beyond it. Both `insert_at_end` and a full `replace` with no `section_id` returned `ok: true`, with no sharing step and no `canvases.access.set` call. The tab's own title survived the replace.
- Also in favour of a channel canvas: `canvases.edit` lists `free_teams_cannot_edit_standalone_canvases`, so standalone canvases are a dead end on free workspaces.
- Document the channel-tab path as *the* setup (Step 9), not one option among several. It can't be automated by the action either way.

### 7. `run.go`: wire up the canvas refresh

- `Run()` currently returns directly from inside the `RunMode` switch (`return runPostMode(...)` / `return runUpdateMode(...)`), so there's no code path "after" it today. Restructure so the message path and the canvas refresh are two independent attempts whose errors are collected, not short-circuited:
  1. capture the switch's result in `messageErr` instead of returning it
  2. if `cfg.PRTrackerCanvasID != ""`, run the canvas refresh regardless of `messageErr`, capturing `canvasErr`
  3. `return errors.Join(messageErr, canvasErr)`
- What still fails fast, before the switch and so before the canvas: config errors and channel resolution by name. The canvas needs neither, but both are one-time setup errors, not per-run conditions.
- The canvas refresh runs even when post/update failed. A failed message send says nothing about whether the canvas can be written, and a stale canvas is the thing this feature exists to prevent. The one exception is `post` mode's shared fetch (below): if that fails, neither path has PRs to work with, so both are skipped and only the fetch error is returned.
- Both errors reach the action's exit code. A canvas that can't be written is a real failure of an opted-in feature — usually the one-time access setup (Step 6) — and a warning in a green run is easy to miss for a surface nobody watches. Wrap `canvasErr` so the log names the canvas as the failing part, not the reminder.
- Message-path semantics are unchanged: the same conditions fail the run as today, with the same errors.
- Canvas refresh step: PRs (see fetch sharing below), then `prparser.ParsePRs`, then `canvascontent` (with `GeneratedAt: time.Now().UTC()` and the cap flags off the fetch result)/`canvasbuilder`/`slackclient.ReplaceCanvasContent`.
- `ParsePRs` is what applies `old-pr-threshold-hours`; the Slack user IDs it resolves go unread on the canvas. In `post` mode it runs twice per run — once on the message path's non-draft subset, once here on the full set. It touches no API and no clock beyond `time.Now()`, so the second pass is a copy, not a re-fetch.
- Failure-isolation tests: canvas failure + message success → run fails, message still sent and state still saved; message failure + canvas success → run fails, canvas still written; both fail → both errors reported.
- End-to-end tests in `main_test.go`, asserting the markdown `mockslackclient` recorded (Step 6) — nothing else covers the wiring from input to canvas:
  - `post` mode with the canvas on, grouped and flat, with drafts in the fixtures: the canvas carries both sections, the message carries only the non-draft PRs.
  - `update` mode with the canvas on: the canvas lists currently open PRs, not the state-tracked set, and the message still updates.
  - canvas off: `ReplaceCanvasContent` never called.

#### Fetch sharing

`post` mode's message path already calls `FindOpenPRs` over the same repositories with the same filters; only the options differ. Share that one fetch instead of running it twice:

- `post`: one `FindOpenPRs(..., PRFetchOptions{IncludeDrafts: canvasOn, FetchActivityTimestamps: canvasOn})`, where `canvasOn` is `cfg.PRTrackerCanvasID != ""`. The message path takes the non-draft subset (filter on `pr.GetDraft()`, the same predicate `getPRFilterFunc` applies today); the canvas step takes the full set.
- `update`: two fetches, inherently. The message re-fetches state-tracked refs via `GetPRs`, which includes PRs now closed or merged (rendered struck-through / 🚀) — those can never come from a list of open PRs. The canvas gets a second, separate `FindOpenPRs` with both options on.
- Saves roughly one repository list call plus 3 calls per open PR (reviews, PR comments, timeline comments) per `post` run.
- Placement: the canvas refresh takes a `githubclient.OpenPRsResult` as an argument and never fetches for itself, so it has one code path in both modes.
  - `post`: `runPostMode` keeps its own fetch and hands the result back, so `Run()` can pass it to the canvas refresh. Not lifted into `Run()` ahead of the mode switch: the fetch belongs to `post` mode only, so hoisting it would mean an `if cfg.RunMode == RunModePost` immediately followed by `switch cfg.RunMode`, branching on the mode twice.
  - Return a small result type, not a bare slice: `runPostMode(...) (postModeResult, error)` with `postModeResult{fetched githubclient.OpenPRsResult; prsFetched bool}`. `prsFetched` is the canvas's go/no-go. A bare slice can't carry it — "fetch failed" and "fetch succeeded, found nothing" would both be an empty slice, and only the second may reach the canvas. Refreshing on the first would wipe the canvas to `_No open PRs_` every time GitHub is unreachable. `OpenPRsResult` (R1) carries the cap flags the footer note needs (Step 4).
  - `runPostMode` returns `prsFetched: true` from every path after the fetch, its no-PRs early return included, so the canvas still refreshes when the message path stops early.
  - Timeout: `prFetchTimeout` (60s) is a function-local const in `runPostMode` and `runUpdateMode` today. Both `Run()`-level calls need their own `context.WithTimeout`, so lift it to package scope rather than adding a third copy, as two consts — 60s, and 120s when the canvas is on (Step 2). The canvas-on value applies to `post` mode's shared fetch and to `update` mode's canvas fetch; `update` mode's `GetPRs` keeps 60s, since its PR count is unchanged.
  - `update`: `Run()` calls `FindOpenPRs` for the canvas only, and only when the canvas is on. Not before the switch like `post` — it belongs to the canvas attempt (step 2 above), after `runUpdateMode` has already updated the message. A failed fetch becomes `canvasErr`, never an early return, so it cannot stop the message update. `runUpdateMode` is untouched.

What keeps `post` mode's message byte-identical when the canvas is on:

- The GitHub call itself is unchanged. `prService.List` returns drafts either way — today's exclusion is client-side in `getPRFilterFunc` — so `IncludeDrafts` cannot change which PRs the single fetched page holds.
- Drafts are removed by exactly the predicate that excludes them today, before `prparser.ParsePRs` — so PR count, ordering, summary text and state saving all see the same set.
- The `MaxPRsToFetch` cap is applied per kind (R1), so drafts can't displace open PRs on a >50-PR fetch.
- Filters, snooze exclusion and every other fetch-path step are untouched — `IncludeDrafts` only removes the draft check.
- `FetchActivityTimestamps` adds no fatal path: a failed `ListCommits` falls back to `updated_at` with a warning (Step 2), and the message never reads `LastActivityAt`.
- Canvas off → zero-value options → today's call exactly.
- The raised `prFetchTimeout` (Step 2) keeps the bigger fetch inside its budget. This is the one thing that can still break the equivalence, and it degrades the message rather than failing it — R4 names the cause in the log.
- Regression test: a `post` run with the canvas on produces the same message blocks as the same fixtures with it off, drafts present in both. It runs on mocks, so it proves the wiring, not the timing.

### 8. Reminder message: link to the canvas

- `messagecontent.Content` gains `CanvasURL string`, from `contentInputs.CanvasURL` (Step 1). Set it in **all three** branches of `GetContent`'s switch, the `len(openPRs) == 0` one included — that branch carries the `no-prs-message` case below. Empty → nothing changes anywhere, so existing tests stay untouched.
- `messagebuilder.BuildMessage` appends a trailing context block, `<URL|📋 PR tracker canvas>` — a context block, not rich text, so it reads as a subdued footer rather than another list row. The label matches the feature name in the README.
- Append it **after** `limitMaximumMessageSize`, not before: that function truncates at 50 blocks, and a link appended earlier would be the first thing dropped on a large message.
- `limitMaximumMessageSize` must reserve the block: one limit value, 48 when the footer follows and 50 when it doesn't, used for **both** the comparison and the slice. Slicing at 48 while comparing against 50 still yields 51 blocks whenever the message is exactly 50 (17 repositories: 3×17−1), which `slackclient.SendMessage` rejects (`len(blocks) > 50`) — a large message would fail to send instead of gaining a link. Update the constant's comment for the reserved block.
- 48, not 49. Grouped blocks run heading, list, spacer per repository, so block 49 is the 17th repository's heading and block 50 its list. Cutting at 49 leaves that heading with no list under it — the exact case the existing 50 was chosen to avoid. 48 ends on the 16th repository's spacer, which then reads as spacing before the footer.
- The `no-prs-message` path gets the footer too: `BuildMessage`'s `!content.HasPRs()` branch appends it itself. That branch returns before `limitMaximumMessageSize`, so the reserved-block rule above doesn't apply to it (two blocks).
- Never a footer-only message. `runPostMode`'s early return (`!content.HasPRs() && content.SummaryText == ""`) is unchanged: with `no-prs-message` unset and no open PRs, the run still sends nothing, canvas on or off. A message carrying only a link would notify the channel on every scheduled run with no news, and `state.SavePostState` would store zero PRs behind it, which `runUpdateMode`'s "No PRs to update in state" early return then never cleans up.
- `runUpdateMode`'s matching branch, which deletes the message, is unchanged. It fires when every PR the reminder tracked is closed or merged, and a message reduced to a bare link is worse there than no message.
- The link is rendered optimistically, before the canvas refresh runs (Step 7). It stays correct either way: the canvas is the user's and exists whether or not our write succeeded.
- No preview card: add `slack.MsgOptionDisableLinkUnfurl()` and `slack.MsgOptionDisableMediaUnfurl()` to `PostMessage` and `UpdateMessage` in `slackclient`, neither of which sets an unfurl option today. A `slack.com` link is exactly what Slack expands into a card, and the card would dwarf the one-line footer. Set both, because the default is to expand: "By default, we unfurl all links in messages posted by users and Slack apps" ([unfurling docs](https://docs.slack.dev/messaging/unfurling-links-in-messages/)) — no bot-token exemption is documented for either parameter.
- [`chat.update`](https://docs.slack.dev/reference/methods/chat.update/) documents neither option; `chat.postMessage` documents both. slack-go sends them anyway — no send-mode filtering (v0.27.0 `chat.go`) — so passing them on update is safe but presumed inert. Test only that both options are passed.
- `slack.MsgOption` is an opaque `func(*sendConfig) error`, so a captured option can't be compared to anything. Have `mockSlackAPI` (`slackclient_test.go`) record the `options` it receives, then apply them and read the resulting form values:

  ```go
  _, values, err := slack.UnsafeApplyMsgOptions("", "", "", mockAPI.postedOptions...)
  // values.Get("unfurl_links") == "false" && values.Get("unfurl_media") == "false"
  ```

  Both option constructors do exactly `config.values.Set(...)` (v0.27.0 `chat.go`), so this reads what would go on the wire. The `Unsafe` name is about API stability, not correctness.
- Existing messages are unaffected — PR titles are links inside rich text blocks, which Slack never unfurls.
- Tests: link present in the grouped, flat and `no-prs-message` cases; absent when `CanvasURL` is empty; a grouped message over the limit ends at 49 blocks, the link last and the block before it not a repository heading; the same message without the link still ends at 50; a `post` run with no PRs and no `no-prs-message` sends nothing with the canvas on; both unfurl options passed on post and update.

### 9. Docs & permissions

- README: new "📋 PR Tracker Canvas" section explaining what it is and how to set it up — add a canvas tab to the reminder channel, then ⋮ → Copy link and paste it into `pr-tracker-canvas-link`. State the access requirement behind it (Step 6), so a canvas kept elsewhere can still be made to work. Add `canvases:write` to the Slack scope table (canvas-only). The GitHub permissions block stays as is.
- README: warn that the action owns the whole canvas — every run replaces all content (Step 6), so hand-typed notes are lost. Suggest a second canvas for those.
- State that the canvas notifies nobody (see "Canvas content format").
- Mention that the reminder message then carries a footer link to the canvas (Step 8).
- Document the new input in the inputs table.
- Exercise both the on and off paths end to end. No workflow-permission change needed.
  - The two workflows post to different channels, so they take **different** canvas links — each channel's own tab, not one shared canvas. Both would otherwise overwrite each other every run.
  - `.github/workflows/pr-reminder.yml` (posts to `#github`): set `pr-tracker-canvas-link` to the `#github` canvas, giving the scheduled runs a real, continuously refreshed canvas in both `post` and `update` mode.
  - `.github/actions/e2e-tests/action.yml` (posts to `#pr-reminders-test`): set the `#pr-reminders-test` canvas on the "Run with filters" step only — the richest case (multi-repository, `group-by-repository: true`, filters), covering grouped open PRs next to the always-flat draft list.
  - Leave it unset on the "Basic run" and "Multi-repository run" steps, so every release also verifies the action still behaves exactly as before when the input is absent.
  - The links go in as plain literals, like `slack-channel-name` — they aren't secrets, though they do carry the workspace name and team ID.
- Manual prerequisites, not automatable here. All confirmed in place on 2026-08-08:
  - `canvases:write` on the `pr_bot` app. One app covers both workflows — `pr-reminder.yml`, `build.yml` and `release.yml` all pass `secrets.DEV_SLACK_TOKEN`.
  - a "PR tracker" canvas tab in each channel: `#github` and `#pr-reminders-test`, both currently empty. The `#pr-reminders-test` link is the one quoted in Step 1; copy the `#github` one the same way (⋮ → Copy link) when writing the workflow.

### 10. Spec sync

- Update `githubclient.spec.md`, `slackclient.spec.md`, `prparser.spec.md`, `config.spec.md`, `messagecontent.spec.md`, `messagebuilder.spec.md` and `models.spec.md` (R2's `Repository.GetPullsURL()`) for the changes above.
- Add `canvascontent.spec.md` and `canvasbuilder.spec.md`.

## Consequences

### Positive

- An always-current PR/WIP view lives directly in Slack, with no need to open GitHub.
- Draft/WIP visibility, absent from the main reminder message by design, becomes available to whoever wants it.
- Every scheduled reminder carries a footer link into the canvas, so the transient message becomes the entry point to the persistent view — at no extra scope, since the user supplies the URL.
- Fully additive and off by default: existing users see zero behavior change (zero-value fetch options, early-exit in `run.go`).
- The R1-R3 refactors close pre-existing test-coverage gaps (draft filter, repository grouping, old-PR/author-fallback display paths) independent of whether the canvas feature itself is used.
- R4 gives a pre-existing silent failure mode a log line, whether or not the canvas is used.

### Negative

- Opted-in runs fetch drafts on top of open PRs, and make one extra GitHub API call per draft (commit lookup for activity), adding to rate-limit consumption. `update` mode also fetches twice; `post` mode shares one fetch (Step 7).
- The PR fetch timeout doubles when the canvas is on (Step 2), so a slow or unreachable GitHub takes twice as long to surface.
- `post` mode's message path now runs a fetch shaped by a canvas input. Kept safe by an explicit equivalence test and per-kind capping (Step 7), but it is a coupling that didn't exist before.
- Canvas access can't be granted by the action itself. The intended setup (a canvas tab in the reminder channel) makes it implicit, but a canvas kept anywhere else needs manual sharing.
- The canvas is a golden-file surface: any formatting change shows up as a snapshot diff to regenerate (Step 5). Intended, but it does make cosmetic tweaks a two-step change.
- A canvas row is one markdown string, so every GitHub-sourced value needs escaping (Step 5) — a class of bug the Block Kit message can't have, and one that only shows up on titles containing markdown characters.
- Opting in adds a way for the run to fail: a canvas write that can't be done fails the action even though the reminder was posted (Step 7). Deliberate — the alternative is a feature that silently stops working — but it means a Slack-side access change turns scheduled runs red.
- Two new packages (`canvascontent`, `canvasbuilder`) largely mirror existing ones (`messagecontent`, `messagebuilder`), adding maintenance surface for a feature many users won't enable.

### Neutral

- Activity and draft-staleness thresholds are hardcoded, not configurable, in v1.
- The canvas names people instead of mentioning them, so it notifies nobody. The reminder message stays the notifying surface.
- Link unfurling is now explicitly disabled for the reminder message. No visible change today, but any future link in the message won't preview either.
