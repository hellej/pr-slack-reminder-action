# Cache Merged PRs in State

date: 2026-10-11
status: draft

## Requirements

- Every update run re-enriches the merged PRs it shows, though their reviews rarely change after the merge. In this repository that is 6 of a run's 9 GraphQL points (PR Reminder run 38087695368)
- Goals:
  - A merged PR enriched by one run is not enriched again by a later update run, as long as each run in between used it
  - A cached PR is looked up by its ref alone
  - A PR whose enrichment failed is never cached, so the next run retries it
- Non-goals:
  - Post mode reading the cache
  - Open PRs: `mergeable` changes as the base branch moves, without the PR changing
  - Any GraphQL query change
- Serves AGENTS.md § Purpose: event-driven update runs spend fewer points of a token budget shared with the team's other workflows. In the reference deployment, about 6 of about 20 points per update run
- Lands on a feature branch, through a PR

## Target Shape

- **Merged PR cache**: the merged PRs the last run used, each as enriched, saved in state under the `mergedPRCache` key
  - Holds every merged PR with complete reviewer info out of the run's merged fetch and tracked PRs, whether enriched that run or itself taken from the cache. Deduplicated by ref
  - Replaced by each run that saves it, so it never holds more than one run's merged and tracked PRs
- **Complete reviewer info** (`PR.HasCompleteReviewerInfo`): true when the PR's enrichment returned its node with no error at or under it. A PR left without reviewer info, or with a field error under it, has it false and is never cached
- Update mode loads state before the merged fetch, then:
  - `FindRecentlyMergedPRs` returns the cached PR for each capped result whose ref is in the cache, and enriches only the rest. No enrichment request when every one is cached
  - `resolveTrackedPRs` resolves a tracked ref from the cache before falling back to `GetPRs`
  - After a successful edit, it saves the cache. A deleted or kept message, or a failed edit, saves the loaded state unchanged, as today
- Post mode passes no cache to the merged fetch, and saves the cache with its new state
- The merged search still runs every time: it finds new merges and applies the filters
- No change to action inputs or permissions, nor to a run without a cache

### GitHub API Budget

- No query changes
- Merged enrichment costs about 1 point per newly merged PR, instead of about 1 per shown merged PR on every update run. See docs/third-party-facts.md § The GraphQL cost formula overestimated a search, but matches the enrichment query's logged cost of about 1 per PR: read `rateLimit.cost`
- A tracked PR past the merged cap costs about 1 point once, then 0

## Breaking Change

- Non-breaking, minor: an enhancement with no input change
- State keeps its schema version. The first run after upgrading reads no `mergedPRCache` key, enriches every merged PR as today, and saves the cache

## Summary

1. `PR.HasCompleteReviewerInfo`, set by both enrichment paths
2. `FindRecentlyMergedPRs` returns cached PRs instead of enriching them
3. `State.MergedPRCache` and `WithMergedPRCache`
4. Run wiring: update mode loads state before the merged fetch and reads the cache on both paths; both modes save it

## Steps

### Step 1: Mark PRs with complete reviewer info

- `internal/apiclients/githubclient/`: add `PR.HasCompleteReviewerInfo`
  - `enrichedPR` (open and merged enrichment) and `getPRBatchByRef` (`GetPRs`) set it when the alias returned a node and no error for that alias
  - `prWithoutReviewers` leaves it false
  - Open PRs carry it too, unread
- Update githubclient.spec.md § Behaviour

### Step 2: Return cached merged PRs instead of enriching them

- `FindRecentlyMergedPRs` takes `mergedPRCache []PR`
  - After the cap, a result whose ref is in the cache returns the cached PR as saved. The rest go to `enrichPRsWithReviewInfo`
  - Skip that call when nothing is left: on no results it logs a fetch it never sends
  - The merge order stays. The failed-enrichment fallback (`prWithoutReviewers`) applies to the uncached results only
  - Log how many came from the cache
- The `Client` interface and its callers take the new parameter
- Step 4's integration test pins this step's behaviour
- Update githubclient.spec.md

### Step 3: Merged PR cache in state

- `internal/state/`: add `State.MergedPRCache []githubclient.PR`, JSON key `mergedPRCache`
- `WithMergedPRCache(state, prs)` returns a copy with the cache replaced by the merged PRs out of `prs` that have complete reviewer info, deduplicated by ref (`utilities.UniqueFunc`)
- An entry stores `githubclient.PR` as is, with no JSON tags, so its keys are Go field names, as `models.Repository`'s already are. The state snapshots pin them
  - `HasCompleteReviewerInfo` saves with it, so an entry taken from the cache passes `WithMergedPRCache` again
- Update state.spec.md: the intro and § Behaviour list the cache among what state persists. § Oddities: the line on what an update run rewrites names the cache too, plus the field added without a schema bump and the capitalised keys

### Step 4: Wire the cache into the run

- `cmd/pr-slack-reminder/run.go`:
  - `Run` loads state before the merged fetch in update mode, and hands the loaded state or its error to `runUpdateMode`. A load error still fails update mode, after the canvas refresh, as today
  - Update mode passes the loaded cache to `FindRecentlyMergedPRs`. Post mode passes nil
  - `resolveTrackedPRs` takes the cache and resolves an unresolved tracked ref from it before `GetPRs`
  - Post mode saves `WithMergedPRCache` over its merged PRs. Update mode, after a successful edit, over its merged and tracked PRs
- `testhelpers/mockgithubclient/`: `FetchRecording` records each enrichment request's refs, as it records `GetPRsRequests`
- `main_test.go` pins with `FetchRecording` that a cached merged PR is neither enriched nor in a `GetPRs` request, and that a PR whose enrichment failed is not saved
- The state snapshots re-record with the cache. Read the diff before re-recording
- Update run.spec.md: § Behaviour for the load order and the cache read and write. § Oddities for the Caveats on cached rows and on filters
- Live check: on the PR, the Build E2E step "Run with filters and canvas link: update" loads the state its post step saved in the same job
  - Needs at least one merge in the E2E repositories inside the 7-day window
  - Done when the update step logs `Found N recently merged pull requests` with N ≥ 1, and no `Fetching pull request reviews and comments` after it. The PR states the update step's merged-path cost next to the post step's

## Consequences

### Positive

- Fewer points per update run, per the GitHub API Budget under § Target Shape
- An update run with every merged PR cached sends one request fewer

### Negative

None

### Caveats

- A merged row doesn't show reviews, comments or title edits made after it was cached
- Post mode still re-enriches, about 6 points a day
- A tracked PR taken from the cache skips the current filters until the next post
- A failed merged search drops its PRs from the saved cache, so the next run re-enriches them
- Renaming a `githubclient.PR` field decodes it empty from entries saved before the rename, until they leave the cache

### Neutral

- State grows by one entry per cached PR
- Update mode loads state earlier in the run, which changes the log line order
