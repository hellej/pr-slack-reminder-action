# Retry GitHub API calls

date: 2026-09-26
status: draft

## Goals

- A GitHub call that hits a transient failure is tried again, up to 3 attempts, in both run modes
- Every attempt has its own deadline, so a hung request is cut off and retried rather than using up the whole call's time
- The state load gets the same retry and deadline. Today it has neither
- A retry that recovers leaves a log line only, no annotation
- The work lands on its own branch and PR from `main`

Serves § **Purpose**: the daily `post` is the one notification the team plans around. Today two GraphQL 5xx in a row, or one hang, lose it for the day, and one failed state load costs an `update` its edit. Seen in practice: GraphQL 5xx responses and timeouts. Sized against § **Reference Deployment**: one `post` and a handful of `update` runs a day, each making a few GitHub calls, so waiting a few seconds on a failure costs nothing the team sees.

### Non-goals

- No Slack retries
- No rate-limit handling: `Retry-After` and `x-ratelimit-reset` stay unread. GraphQL keeps retrying a 429 as it does today; the state load does not retry one
- No new action input
- No change to what fails a run, or to which failures are warnings

## Target shape

- **Transient failure**: an attempt that got no response (network error, attempt deadline) or a 5xx. For GraphQL also a 429 or an unparseable body, as today. Only a transient failure is retried
  - Everything else fails at once as today: any other 4xx, a GraphQL errors array, `ErrNoArtifactFound`, a zip or JSON decode error
  - go-github's rate-limit errors carry a 403, so they are not transient. See docs/third-party-facts.md § `go-github` v78 reports rate limits only as a 403, and short-circuits a request while one lasts
- **Retry waits**: 2s before attempt 2, 5s before attempt 3. Three attempts in all
- **Attempt deadline**: 15s per attempt, one value for every GitHub call. See [Justification § One 15s attempt deadline](#one-15s-attempt-deadline)
- The caller's ctx still bounds the whole call. Once it is done, no further attempt starts
- Each retry logs one line naming the API (GraphQL, artifact list, artifact download), the attempt number, the wait and the error
- Worst case per call: 3 × 15s + 7s = 52s
- `prFetchTimeout` in `run.go` rises from 60s to 2 minutes: an open-PR fetch is a listing call then one wave of enrichment batches, so 2 × 52s
- The state load has no outer bound: its two retried units cap it at ~104s
- Retried units in the state load:
  - Listing the artifacts
  - Getting the download URL and downloading the zip, together: a fresh URL per attempt, since it expires after 1 minute. See docs/third-party-facts.md § `go-github` v78 `DownloadArtifact` returns a plain error on a non-302, with the `*Response`
- go-github never retries on its own. See docs/third-party-facts.md § `go-github` v78 never retries a 5xx or network error, and sets no HTTP timeout
- No change to `action.yml` inputs or permissions

## Breaking change

- Non-breaking: patch. No input, output or state format changes

## Steps summary

1. Retry every GraphQL call with the new policy and per-attempt deadline
2. Retry the state artifact load, with a per-attempt deadline

## Steps

### 1. GraphQL retry policy and attempt deadline

- `internal/apiclients/githubclient`: a new `retry.go` holds the retry loop, the retry waits and the attempt deadline, generic over the attempt's result
  - The waits and the deadline stay package `var`s, as `retryDelay` is today, so in-package tests can shrink them
  - The loop stops early when the caller's ctx is done, and logs each retry
- `graphql.go`: `postWithRetry` moves onto the new loop, keeping its transient-failure classification. `graphqlMaxAttempts` and `retryDelay` go
- The ~4 `graphql.Do` callers drop their own `context.WithTimeout`. `pullRequestListTimeout` and `reviewsFetchTimeout` go
- `cmd/pr-slack-reminder/run.go`: `prFetchTimeout` to 2 minutes
- Tests: the retry policy and attempt deadline; existing retry tests in `graphql_test.go` follow the new attempt count
- `githubclient.spec.md`: the timeouts bullet and the retry line in Doesn't Do. `run.spec.md`: Doesn't Do's "Doesn't retry a GitHub or Slack call" becomes Slack only

### 2. State artifact load retry and attempt deadline

- `fetchartifact.go`: the two retried units from the target shape run through the step 1 loop
  - A go-github failure is transient when its `*Response` is nil or its status is 5xx. Read the status off the `*Response`, not the error, since `DownloadArtifact` returns a plain error
  - The zip download is transient on a network error, a 5xx, or a failed body read
  - The download unit reads the whole body inside the attempt, so the deadline covers the body read
- `HTTPClient` changes from `Get(url)` to `Do(*http.Request)`, so the zip download carries the attempt ctx. `*http.Client` satisfies it, so the `httpClient` wrapper goes
  - The ~3 mocks implementing `Get` follow, in `testhelpers/mockgithubclient` and the `githubclient` tests
- Tests: which state-load failures are transient, and that a failed zip download fetches a fresh download URL
- `githubclient.spec.md`: `FetchLatestArtifactByName`'s bullet and the "No retries on the REST artifact calls" line
- Live check, done means both runs green and neither log showing a retry line:
  - `gh workflow run pr-reminder.yml --ref <branch> -f run-mode=post -f build-first=true`
  - then the same with `-f run-mode=update`, which loads the state the post saved

## Consequences

### Positive

- One GitHub blip no longer costs the day's `post`, nor an `update`'s edit
- A hung request fails in 15s and is retried, where today a hung state load waits for the job timeout

### Negative

- None

### Caveats

- During a real outage a run takes longer to fail: ~1 minute per failing call, against seconds today
  - `pr-reminder.yml`'s concurrency group queues runs rather than cancelling them, so a slow failing run delays the next `update`
- A GraphQL request GitHub ends at its 10s limit is retried unchanged, and may time out again. Each such timeout costs extra rate-limit points (docs/third-party-facts.md § GitHub ends a request after 10 seconds of processing, with a 502 or 504 on GraphQL)

### Neutral

- Slack calls keep failing on the first error

## Justification

### One 15s attempt deadline

- GitHub ends a request after 10s of processing with a 502 or 504 (docs/third-party-facts.md § GitHub ends a request after 10 seconds of processing, with a 502 or 504 on GraphQL). A client deadline past 10s lets that server answer arrive, and the margin covers network time
- Today's 10s batch deadline can cut off a request GitHub would still have answered, and today's 30s listing deadline only waits on the network past GitHub's own 10s
- The state load's calls are small, so the same value fits them
