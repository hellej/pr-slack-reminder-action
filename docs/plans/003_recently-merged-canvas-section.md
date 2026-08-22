# Recently merged canvas section

date: 2026-08-22
status: draft

Extends the PR tracker canvas from [002](002_PR-tracker-canvas.md) with a third section listing recently merged PRs. 002's "Room for a recently-closed section" is the starting point; this plan replaces it, including its `pullRequests(states: [CLOSED, MERGED])` sketch, which live checks showed does not work (see Step 1).

## Goals

- Motivation: the canvas answers "what is waiting on us" but not "what landed". A merged list closes that gap in the same place, so nobody has to open GitHub per repository to see the week's output.
- New `## Merged` section, last of the three PR lists, after `## Open` and `## WIP`.
- Ordered most recently merged first.
- Only PRs merged within the last 7 days.
- At most 15 rows.
- Rows carry the merged marker 🚀, as the updated reminder message does for merged PRs.
- The same filters that shape the other sections shape this one: `filters`, `repository-filters`, `github-repositories`.
- The section is the cheapest part of the canvas: one extra GitHub request per refresh, whatever the number of repositories.

## Non-goals

- No new action input. The section is on whenever the canvas is on, with a fixed window and a fixed cap.
- No reviewer names on merged rows. Approvers would cost an enrichment request per refresh for the least important section on the canvas.
- Nothing changes in the reminder message.
- No grouping by repository, even with `group-by-repository` on. The WIP section is flat for the same reason: the list is short and time-ordered.
- No closed-but-not-merged PRs. "Merged" is the whole point of the section.
- No `/snooze` handling. A snooze hides a PR from a review reminder; the PR is merged.
- No paging past the first 100 merges per repository inside the window. A repository merging more than 100 PRs a week gets its list truncated, and a log line saying so.

## Target shape

```markdown
## Open

- **[Add pagination to the PR listing](https://github.com/test-org/test-repo/pull/1)** _5 hours ago_ by Alice Anderson (✅ Dana Davis / 💬 Erin Evans)

## WIP

- **[Spike: replace mux with chi](https://github.com/test-org/test-repo/pull/3)** by Carol Clark `updated 5 hours ago`

## Merged

- **[Bump the Slack SDK](https://github.com/test-org/repo-two/pull/2)** _2 hours ago_ by Bob Brown 🚀
- **[Drop the REST fallback](https://github.com/test-org/test-repo/pull/9)** _3 days ago_ by Alice Anderson 🚀

---

_Updated 2026-08-08 06:15 UTC_
```

- Row: linked title, merged-ago text in italics, author, then 🚀. The marker is trailing, matching `messagebuilder.buildPRBulletPointBlock`.
- Empty section: the heading plus `_No merged PRs_`, as `## Open` and `## WIP` do.
- Merged fetch failed: the heading plus `_Merged PRs could not be fetched_`. The canvas is still written, so Open and WIP stay fresh, and the run still fails, like any other canvas failure.
- No footer cap note. The section is "the 15 newest merges", so a 16th merge is not a surprise omission; the two existing cap lines stay as they are.
- No `action.yml` change beyond the `pr-tracker-canvas-link` description wording.
- Expected to need no new GitHub token permission: the search returns PRs in the same repositories, which `pull-requests: read` already covers, and no enrichment means nothing new touches `issues: read`. GitHub does not document how search visibility into private repositories is granted, so Step 1 checks it live before anything is built on it.
- No new Slack scope.

## Breaking change classification

**Minor.** New optional functionality, no input renamed, removed or redefaulted. Existing canvases gain a section on the next run.

## Summary of steps

1. `githubclient`: merged-PR search, window, cap and a `MergedAt` timestamp
2. `prparser`: merged-ago display text
3. `canvascontent`: the merged section in `Content`
4. `canvasbuilder`: render `## Merged`
5. `run.go` and the GraphQL mock: fetch merged PRs on every canvas refresh
6. Specs, README and `action.yml` wording

## Steps

### 1. `githubclient`: merged-PR search, window, cap and `MergedAt`

Files: `internal/apiclients/githubclient/models.go`, `graphqlmodels.go`, `graphqlfetch.go`, `githubclient.go`.

**Private repositories.** The query reaches them: run live on 2026-08-22 against a private repository, it returned the merged PR with `repository.isPrivate` true, for a classic token holding `repo`. Undocumented is which permission counts as access, and the failure mode is silent, since no-access and no-matches are both `issueCount: 0`. Repeat that query once during this step with a token holding only `pull-requests: read`, as `README.md`'s `permissions:` block documents. Verified done when it returns the merge. If it does not, find the permission that works and update the README's permission block as part of this plan.

**Why not a `pullRequests` listing.** Merge time is not orderable: `Repository.pullRequests` takes an `IssueOrder`, whose `IssueOrderField` is `COMMENTS`, `CREATED_AT`, `UPDATED_AT` only ([IssueOrder input object](https://docs.github.com/en/graphql/reference/issues#input-object-issueorder)). Ordering by `UPDATED_AT DESC` and cutting the window client-side does not work either: any post-merge comment, label or cross-reference bumps `updatedAt`, so the first page fills with old merges. Measured live on 2026-08-22, one 100-node page caught 33 of the 348 PRs `microsoft/vscode` merged in 7 days. Paging past it means walking every PR touched in the window.

**Use `search` instead.** It filters on merge date server-side, so one page per repository is the exact in-window set:

```graphql
query($q0:String!,$q1:String!){
  rateLimit { cost remaining limit }
  s0: search(query:$q0, type: ISSUE, first: 100){
    issueCount
    nodes { ... on PullRequest {
      number title url createdAt mergedAt
      author { login __typename ... on User { name } }
      labels(first: 100){ nodes { name } }
    } }
  }
  s1: search(query:$q1, type: ISSUE, first: 100){ ... }
}
```

- Query string per repository: `repo:<owner>/<name> is:pr is:merged merged:>=<YYYY-MM-DD>`. `is:pr`, `is:merged` and `merged:>=` are documented qualifiers ([searching issues and pull requests](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests)).
- The cutoff is passed at day granularity, so the search returns up to one extra day of merges, and the exact `mergedAt >= mergedSince` cut happens client-side. Day granularity avoids relying on the date-time form of the qualifier.
- One alias per repository, `s0`, `s1`, so each result set maps back to its repository without selecting `repository { nameWithOwner }`. The whole query shape, several aliased searches included, was run live on 2026-08-22. There is always at least one repository and so at least one search: `GetConfig` falls back to the current repository when `github-repositories` is empty.
- No per-repository error handling, because there is none to have: `search` reports a nonexistent repository, one the token cannot read, and an empty window identically, as `{"issueCount": 0, "nodes": []}` with no `errors` key. The `NOT_FOUND` that `repository(owner:,name:)` raises has no counterpart here. Errors that do occur come back with no `data` at all, so any error fails the whole merged fetch and the section renders its unavailable line.
- A misspelled repository is therefore silent on this path, but not overall: every canvas refresh lists open PRs through `repository(owner:,name:)` first, and that call already fails the run on an unreadable repository.
- `labels` is selected because `ignored-labels` and `labels` filters apply to this section.
- No `orderBy`: search cannot sort by merge date either ([sorting search results](https://docs.github.com/en/search-github/getting-started-with-searching-on-github/sorting-search-results) lists comments, created, interactions, reactions, relevance, updated). Ordering is client-side, on `mergedAt`, over the in-window set.
- `issueCount` says how many merges the window really holds. Over 100, log that the repository's list is truncated.
- `assembleQuery` appends a fragment; the search query needs none, since `... on PullRequest` is inline. Let it take an empty fragment rather than growing a second assembler.
- Cost, measured live: **2 points** for a two-repository query, 1 without the `labels` selection. Cost is charged per operation, not per alias, so more repositories barely move it, against 5,000 points an hour. The documented 30 requests/minute search limit is a REST figure and is no constraint at one request per canvas refresh.
- The selection is narrower than `openPullRequestsFragment`: no `updatedAt`, no `isDraft`, no `headRefOid`. Merged PRs therefore carry a zero `UpdatedAt`, `Draft: false` and an empty `HeadSHA` on a struct the other two fetch paths share. Nothing on the canvas reads them: the merged list is re-sorted on `MergedAt` in Step 3, and `LastActivityAt`, `SnoozedUntil` and the reviewer slices stay nil by design. Record it as an oddity in the spec.
- `buildListOpenPRsQuery` and `listOpenPRs` are untouched.

**Timestamp.** `PullRequest` gains `MergedAt *time.Time` with a nil-safe `GetMergedAt()`, next to `LastActivityAt`. `pullRequestNode` gains `MergedAt *time.Time` (`json:"mergedAt"`), and is reused as the search node type; unselected fields decode as zero values. `mergedAt` is a nullable `DateTime` ([PullRequest object](https://docs.github.com/en/graphql/reference/objects#pullrequest)), hence the pointer. `fullPullRequestSelection` also selects it, so `GetPRs` fills it too; nothing reads it there yet.

**Mapper.** `openPullRequestFromNode` hardcodes `State = "open"`, `Merged = false`. Add `mergedPullRequestFromNode`, setting `State = closedPullRequestState`, `Merged = true`, `MergedAt = node.MergedAt`.

**New client method** on the `Client` interface:

```go
FindRecentlyMergedPRs(
	ctx context.Context,
	repositories []models.Repository,
	getFiltersForRepository func(repo models.Repository) config.Filters,
	mergedSince time.Time,
) ([]PR, error)
```

Order of work inside it:

1. Search, with `pullRequestListTimeout`. Any error fails the whole merged fetch, with no per-repository message: `listOpenPRsError`'s "repository not found - check the repository name and permissions" describes an error `search` never returns, and would mislabel the ones it does.
2. Drop PRs whose `MergedAt` is nil or before `mergedSince`.
3. Apply `getPRFilterFunc[PRResult](getFiltersForRepository, false)`. `includePR` reads only title, labels and author, never state, so it needs no change.
4. Sort by `MergedAt` descending and cap at `MaxMergedPRsToFetch` (15), mirroring `capDraftPRResultsToLimit`.
5. Return `PR`s built straight from the search nodes, with `ApprovedByUsers` and `CommentedByUsers` nil. No `enrichPRs` call, and no `excludeSnoozedPRs`: nothing on a merged row needs reviews or comments.

`mergedSince` is a parameter, not a clock read, so the window is deterministic under test and shares one `now` with the canvas footer. The window length lives here as `RecentlyMergedWindow = 7 * 24 * time.Hour`, next to the cap.

### 2. `prparser`: merged-ago display text

File: `internal/prparser/displaytext.go`.

- Add `func (pr PR) GetMergedAgoText() string`: `""` when `GetMergedAt()` is nil, otherwise `durationText(time.Since(*pr.GetMergedAt())) + " ago"`. `durationText` is unexported, so this helper has to live in the package.
- Nothing else is needed: `SortPRsNewestFirst` already takes the timestamp as a parameter, and `GetMergedAt()` is promoted onto `prparser.PR` through `*githubclient.PR`, which embeds `*githubclient.PullRequest`.

### 3. `canvascontent`: the merged section in `Content`

File: `internal/canvascontent/canvascontent.go`.

- `Content` gains `MergedPRs []prparser.PR` and `MergedPRsUnavailable bool`.
- `GetContent` gains a second PR parameter: `GetContent(prs, mergedPRs []prparser.PR, contentInputs, options)`. The Open/WIP split is a `GetDraft()` predicate over one list; merged PRs come from their own fetch and cannot be derived from it.
- `GetContentOptions` gains `MergedPRsUnavailable bool`.
- Merged PRs are sorted newest first on `GetMergedAt()` via `SortPRsNewestFirst`. The sort is repeated here because `prparser.ParsePRs` re-sorts everything oldest first, the same reason the WIP list is sorted here.
- No pruning and no cap in this package: the fetch already applied both. No grouping, whatever `GroupByRepository` says.
- Extend the existing count log with the merged count.

### 4. `canvasbuilder`: render `## Merged`

File: `internal/canvasbuilder/canvasbuilder.go`.

- New constants: `mergedPRsHeading = "## Merged"`, `noMergedPRsText = "_No merged PRs_"`, `mergedPRsUnavailableText = "_Merged PRs could not be fetched_"`.
- `BuildMarkdown` appends one more `renderSection` call after the WIP one and before the `---`, with `renderMergedPRRow` and, as the empty text, `mergedPRsUnavailableText` when `content.MergedPRsUnavailable` and `noMergedPRsText` otherwise.
- `renderMergedPRRow`: `renderTitleLink`, then the merged-ago text in italics, then `renderAuthor`, then `" 🚀"`. No `renderReviewers` call. An empty merged-ago text drops just that segment, as `renderWIPPRRow` does for unknown activity.
- The footer and `getCapText` are untouched.
- All 14 golden files in `internal/canvasbuilder/testdata/` gain the new section, re-recorded with `make update-test-snapshots`. `TestBuildMarkdownHasNoTopLevelHeading` gets a merged PR too.

### 5. `run.go` and the GraphQL mock: fetch merged PRs on every canvas refresh

Files: `cmd/pr-slack-reminder/run.go`, `testhelpers/mockgithubclient/mockgithubclient.go`, `cmd/pr-slack-reminder/main_test.go`, `cmd/pr-slack-reminder/canvas_test.go`.

- `refreshPRTrackerCanvas` takes the GitHub client and does the merged fetch itself, so both run modes keep one canvas code path and one `now`:
  - read `generatedAt := time.Now().UTC()` first, as today,
  - call `FindRecentlyMergedPRs` with `generatedAt.Add(-githubclient.RecentlyMergedWindow)`, under a `prFetchTimeout` context,
  - on error, log it, pass `MergedPRsUnavailable: true`, and carry the error,
  - parse merged PRs with `prparser.ParsePRs` and pass them to `GetContent`,
  - write the canvas, and return the write error joined with the merged-fetch error.
- A failed merged fetch therefore never blocks the canvas write, and still fails the run through the existing `canvasErr` wrapping. A failed open/WIP fetch still skips the refresh entirely, unchanged: that one has no partial rendering.
- No change to the post-mode fetch sharing, to `PRFetchOptions`, or to the message path. The message never sees merged PRs.
- Mock transport, four changes:
  - `Post` dispatches on the query text, and a search query matches neither `"pullRequests("` nor `"pullRequest(number:"`, so it needs a `"search("` branch answering from a new `MergedPRsByRepo` option, with a node JSON emitting `mergedAt` and an `issueCount`.
  - `postedRepositoryNames` reads the repository of alias `rN` off the `nameN` variable. The search query has no such variable and cannot get one: GraphQL rejects an operation declaring a variable it never uses. The branch parses the repository out of the `repo:<owner>/<name>` qualifier in `qN` instead, as a helper next to `postedRepositoryNames`. The type's doc comment listing how aliases bind to variables gains the `sN` case.
  - `ListPRsResponseStatus`, the only failure knob, is read inside `openPRsResponse`. The search branch needs its own, so the failed-merged-fetch test has a lever.
  - No enrichment ref resolution is involved, since merged PRs are never enriched.
- `main_test.go`: `GetTestPROptions` and `getTestPR` carry no merge timestamp. Add one, e.g. `MergedHoursAgo`, so fixtures can sit inside and outside the 7-day window.
- `canvas_test.go`: `canvasTestPRs()` gains merged PRs, and the section assertions gain `## Merged` and `_No merged PRs_`. New cases: a merged PR renders with 🚀 in merge order, a merge older than the window is left out, filters exclude a merged PR, and a failed merged fetch still writes the canvas with the unavailable line while the run reports the error.
- `TestUpdateModeCanvasShowsCurrentlyOpenPRs`'s `assertCanvasDoesNotContain(..., "Tracked merged PR")` stays as it is. Its merged PR reaches the run through `PRsByNumber`, the state-tracked `GetPRs` path that feeds the message, and never through the search. The assertion now says that a merged PR in the message cannot reach the canvas by itself, which is still worth asserting.
- The Slack block snapshots in `cmd/pr-slack-reminder/testdata/snapshots/` must stay byte-identical, guarded by the existing `TestCanvasDoesNotChangeMessageBlocks`.

### 6. Specs, README and `action.yml` wording

Files: the four package specs, `README.md`, `action.yml`, package doc comments.

- `internal/apiclients/githubclient/githubclient.spec.md`: the merged search, its window, cap and client-side ordering, why search rather than the PR listing, no enrichment so no reviewers and no snooze exclusion, the over-100 truncation, the seconds-long search index lag, and the fields a merged PR leaves zero. Two existing lines go stale: "Both PR-reading paths use the GraphQL API" is now three paths, and the per-call timeouts line has to say `pullRequestListTimeout` covers the search too.
- `internal/prparser/prparser.spec.md`: `GetMergedAgoText`.
- `internal/canvascontent/canvascontent.spec.md`: the merged section, its re-sort, no grouping, no pruning.
- `internal/canvasbuilder/canvasbuilder.spec.md`: the merged row is new behaviour to document: merged-ago text instead of age, never reviewers, trailing 🚀, and a fallback line that differs when the fetch failed. Four existing lines flip: the `BuildMarkdown` description, "Both sections render through one `renderSection`", the empty-section line naming the two fallback texts, and "No strike-through and no 🚀: the canvas lists open PRs only".
- `README.md`: the sample canvas markdown, the "Open PRs are listed oldest first" line, the note listing which inputs shape the canvas, the section intro saying the canvas "has a section for work-in-progress PRs too", and the `pr-tracker-canvas-link` row of the inputs table, which repeats the `action.yml` description verbatim.
- Package doc comment: `canvascontent` says "the two sections a PR tracker canvas shows".
- `action.yml`: `pr-tracker-canvas-link` says "open and work-in-progress PRs"; add merged. Description text only, so `go run .github/scripts/check_inputs.go` stays green.
- Verified done by: `make test`, `make check-fmt`, `make check-vet`, and reading the rendered README sample against a fresh golden file.

## Consequences

### Positive

- The canvas answers "what landed this week" without a GitHub visit per repository.
- One extra request per canvas refresh, no matter how many repositories, and no enrichment.
- The cutoff is exact, because GitHub applies it rather than the client.
- A merged-fetch failure degrades one section instead of the whole canvas.

### Negative

- A new query kind in `githubclient`: `search` is the only path that is not `repository(...)`-aliased, so its result decoding and the `... on PullRequest` inline fragment are one-off shapes.
- A repository merging more than 100 PRs a week gets a truncated list, visible only in the log.
- Every canvas golden file changes, so the diff of this change is wide but shallow.

### Neutral

- Merged rows name no reviewers, unlike open rows. The section answers "what landed", not "who reviewed it".
- `MergedAt` is populated on the `GetPRs` path too, where nothing reads it yet. The reminder message's 🚀 keeps coming from `Merged`, not from the timestamp.
- The window and cap are constants. Making them inputs stays additive.
- Search is index-backed and lags a merge by seconds, measured at between 3 and 10 seconds on one live merge. Irrelevant to a scheduled reminder.
