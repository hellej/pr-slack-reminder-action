# Demote approved PRs with failing required checks

date: 2026-10-31
status: draft

Issue: [hellej/pr-slack-reminder-action#100](https://github.com/hellej/pr-slack-reminder-action/issues/100). The work lands on the feature branch `claude/fervent-wozniak-lwnedy` and its PR, not on `main`.

## Requirements

- An approved PR with a failing required check leaves "ready to merge" for "waiting for author", in the message and on the canvas
- Its row says why: a `❌ checks failing` marker
- Pending required checks do not demote. Only a failing one does
- Required means required by branch protection or a ruleset, so a failing optional check never demotes
- A PR with no checks or no required checks keeps today's bucket
- No new action input
- Check status is fetched for approved, non-draft open PRs only

Serves § **Purpose**: "ready to merge" stops naming PRs whose merge button is red, which is the trust the section exists to earn. Sized for § **Reference Deployment**: 0 to 8 open PRs, so the extra request is at most one.

### Non-goals

- No marker on unreviewed PRs, or on approved PRs already in "waiting for author" for another reason
- No demotion for pending checks
- No new workflow trigger for check completion. The README's `update` triggers stay as they are
- No change to "waiting for review": a PR nobody approved stays there, whatever its checks say
- No change to merged, draft or collapsed rows

## Target shape

- `githubclient.PR` gains `HasFailingRequiredCheck bool`
- `FindOpenPRs` gains a phase after review enrichment and snooze exclusion: one more GraphQL request per 25 approved, non-draft PRs, selecting each PR's `statusCheckRollup`
  ```
  pullRequest(number: $numN) {
    number
    statusCheckRollup { contexts(first: 100) { pageInfo { hasNextPage } nodes {
      __typename
      ... on CheckRun { status conclusion isRequired(pullRequestNumber: $numN) }
      ... on StatusContext { state isRequired(pullRequestNumber: $numN) }
    } } }
  }
  ```
  - Each alias carries its own number, so the selection is generated per PR, not shared as a fragment
  - `HasFailingRequiredCheck` is true when a context with `isRequired` true is failing:
    - a `CheckRun` with `status` `COMPLETED` and a `conclusion` other than `SUCCESS`, `NEUTRAL` or `SKIPPED`
    - a `StatusContext` with `state` `FAILURE` or `ERROR`
  - A null rollup, no required context and only pending required contexts read false
- `OpenPRsResult` gains `CheckStatusErr error`: set when the phase failed in whole or in part. The PRs it covers keep `HasFailingRequiredCheck` false, so a failed phase leaves today's bucketing
- `prview.PR.GetNextAction` reads the flag in its first check: a failing required check keeps an approved PR out of `NextActionReadyToMerge`, so it is `NextActionWaitingForAuthor`, as for a conflict
- `prview.PR.IsHeldBackByFailingChecks()` is true when the failing check alone keeps an approved PR out of "ready to merge": approved, no waiting thread, no conflict, flag set. It is what the marker reads
- Marker text: `❌ checks failing`, appended after the reviewer segment on an open row, in the message and on the canvas
- Action inputs: none new. Permissions: none new
  - `statusCheckRollup` and `isRequired` populated under `pull-requests: read` alone on `GITHUB_TOKEN` in a public repository. See docs/third-party-facts.md § `PullRequest.statusCheckRollup` and `isRequired` populate under `pull-requests: read` on `GITHUB_TOKEN`, in a public repository
  - Step 1's checkpoint covers a private repository under the E2E App token

Message and canvas states for an approved PR whose required check fails:

```
Posted at 09:00
*💬 Waiting for author*
  • Fix retry loop   2 days ago   by alice (✅ bob)   ❌ checks failing

Edited by `update` after the check is fixed and a PR event fires
*✅ Ready to merge*
  • Fix retry loop   2 days ago   by alice (✅ bob)

Marked stale by the next post
⚠️ Stale, updated Oct 31 at 09:00
*💬 Waiting for author*
  • Fix retry loop   2 days ago   by alice (✅ bob)   ❌ checks failing
```

The canvas row reads `**[Fix retry loop](url)** _2 days ago_ by alice (✅ bob) ❌ checks failing` under `## 💬 Waiting for author`.

## Breaking or non-breaking

Non-breaking, **minor**. Bucketing and row text change for users with a failing required check, with no input or permission change, as plans 012 and 014 did.

- State: the merged PR cache saves `HasFailingRequiredCheck` beside `Conflicting`. A cache saved before it decodes false, and merged PRs are not fetched for check status, so it stays false
- The first run after upgrading re-buckets every PR, and the canvas rewrites once wherever a PR moves
- A downgrade ignores the key

## Summary

- **R1** Let `buildPullRequestsQuery` take a selection per PR
- **1** Fetch required check status for approved PRs (checkpoint)
- **2** Keep failing PRs out of "ready to merge" in `prview`
- **3** Render the `❌ checks failing` marker in the message and on the canvas
- **4** Warn when the check status fetch fails
- **5** Check the message and canvas live
- **6** Docs

## Steps

### R1. Let `buildPullRequestsQuery` take a selection per PR

Touches `internal/apiclients/githubclient` (`graphqlquery.go`, `openprsfetch.go`, `getprsbyreffetch.go`). Refactor, no behaviour change, ~2 call sites.

- `buildPullRequestsQuery` takes a `pullRequestSelection`, `func(numberVariable string) string`, instead of a `pullRequestFragment`, and builds each alias body from it
- `pullRequestFragment` gains a method returning the `pullRequestSelection` that spreads it, so both existing callers keep their query text byte for byte
- The existing query tests and the snapshots are the regression net

### 1. Fetch required check status for approved PRs (checkpoint)

Touches `internal/apiclients/githubclient`, `testhelpers/mockgithubclient`, `cmd/pr-slack-reminder` (`run.go`).

- `PR.HasFailingRequiredCheck`, set by `client.fetchRequiredCheckStatus`, called at the end of `FindOpenPRs` after `excludeSnoozedPRs`. See [§ Why check status is its own request for approved PRs](#why-check-status-is-its-own-request-for-approved-prs)
- Requested PRs: not draft, `HasCompleteReviewerInfo`, and at least one approver. A PR outside that set keeps the flag false
- The phase batches 25 PRs per request, runs at most `defaultGitHubAPIConcurrencyLimit` requests at a time and goes through `retry.DefaultPolicy` like every GraphQL request
- `hasFailingRequiredCheck` classifies the decoded contexts, as listed in § Target shape
- A failure never fails `FindOpenPRs`:
  - A query, repository or transport error leaves every PR of that batch unflagged
  - A per-PR error (`pullRequestError`, as `failsOnlyOnePullRequest` reads it) leaves only that PR unflagged
  - Either sets `OpenPRsResult.CheckStatusErr`, joining the batches' errors
- A rollup with `hasNextPage` true is classified on its first 100 contexts, and logged
- Log the phase's `rateLimit.cost` through `logRateLimit`, and one line per requested PR with its flag. A failed batch logs its error there too
- `prFetchTimeout` in `run.go` grows from 2 to 3 minutes, and its comment names the third phase: a listing, a wave of enrichment batches and a wave of check status batches, each call retried for up to 52s
- `mockgithubclient`:
  - The transport routes a query containing `statusCheckRollup` before its `pullRequest(number:` branch. The default answer is a null rollup, so existing fixtures with an approved PR keep their buckets
  - `MockGitHubClientOptions.FailingRequiredCheckPRNumbers []int` makes those PRs' required check fail
  - `MockGitHubClientOptions.CheckStatusError error` fails this request only, as `MergedPRsSearchError` does for the merged search
  - `FetchRecording.CheckStatusRequests` records the refs of each request, so a test can assert which PRs were asked for. These requests never enter `EnrichmentRequests`
- Update `githubclient.spec.md`: the flag, the phase, the failure behaviour, and Oddities for the first-100 limit and for a re-run (see § Consequences)
- Verifying it done:
  - `make test`, and the new phase's request carries exactly the approved non-draft refs
  - The repository owner sets up, in a private test repository the E2E runs list, a PR approved by a human other than its author, whose required check fails. The check is required through a ruleset, the case docs/third-party-facts.md does not cover yet. The PR is not authored by `alice` in `pr-slack-reminder-test-repo-1` and carries no `ignore-reminder` label, or the grouped runs drop it
  - The E2E App token keeps its current permissions, which the owner confirmed hold neither `checks` nor `statuses`
  - After pushing the branch, the Build job log shows, for the fixture PR: its flag line reading true, the phase's `rateLimit.cost` with its difference against the last `main` Build run (state it in the PR, per AGENTS.md § GitHub API Budget), and no check status error line
  - If an error line shows, stop and raise it with the user: a permission is needed and the plan has to say which
  - Add what this confirms to docs/third-party-facts.md, and amend the probe entry's "not shown" list

### 2. Keep failing PRs out of "ready to merge" in `prview`

Touches `internal/prview`.

- `GetNextAction` and `IsHeldBackByFailingChecks()` as in § Target shape. Size: one condition, one method
- Update `prview.spec.md`: the check list, the "only demotes" line beside the conflict one, and an Oddities line that a failing check on an unapproved PR moves nothing

### 3. Render the `❌ checks failing` marker

Touches `internal/messagebuilder`, `internal/canvasbuilder`, `cmd/pr-slack-reminder` snapshot tests.

- `messagebuilder.buildOpenPRBulletPoint` appends a plain text element ` ❌ checks failing` after the reviewer elements when `IsHeldBackByFailingChecks()`
- `canvasbuilder.renderOpenPRRow` appends the same text after `renderReviewers`
- Collapsed rows carry no marker, as they carry no reviewers
- Update `messagebuilder.spec.md` and `canvasbuilder.spec.md`
- Add a failing-check PR to three fixture sets, then `make update-test-snapshots`:
  - `snapshotScenarios()` in `cmd/pr-slack-reminder/snapshot_test.go`, which serves the post and marked-stale snapshots
  - The update-mode table of `TestSnapshotsUpdateMode`, for the edited message
  - The fixtures of `TestBuildMarkdownSnapshots` in `internal/canvasbuilder/canvasbuilder_test.go`, for the canvas row
- Expect the snapshots holding those fixtures to change, and every state snapshot with a cached merged PR to gain the new key

### 4. Warn when the check status fetch fails

Touches `cmd/pr-slack-reminder` (`run.go`), `run.spec.md`.

- After `findOpenPRs`, a non-nil `CheckStatusErr` goes through `logWarning`: `Failed to fetch required check status: <error>`. The run goes on with the buckets it has
- `main_test.go`, using `CheckStatusError`: a failed phase warns and leaves the PR in "ready to merge", in post and in update mode
- Update `run.spec.md`'s warning list

### 5. Check the message and canvas live

Touches no files. The step 1 fixture PR is the setup.

- `pr-reminder.yml` lists this repository only, so it cannot show the fixture PR. Read the branch's Build E2E runs instead:
  - The grouped canvas runs ("Run with filters, canvas link and merge stats: post" and its update) show the fixture PR under "Waiting for author" with the marker, in the message and on the canvas
- Done means both surfaces show the marker and neither shows the PR under "Ready to merge"

### 6. Docs

Touches `README.md`.

- The bucket paragraph under [PR Tracker Canvas](#pr-tracker-canvas): "ready to merge" is an approved PR with nothing outstanding and no failing required check, and a failing required check puts an approved PR under waiting for the author, with a `❌ checks failing` marker
- The line under the H1 that lists the three buckets needs no change
- The screenshots in `docs/examples/` stay: no sample shows a failing check
- Say once that check status needs nothing beyond `pull-requests: read`, once step 1 has shown it

## Consequences

### Positive

- "Ready to merge" means the merge button is not red because of required checks
- The author sees the reason on the row
- Cost scales with approved PRs, not open ones: at most one request in § Reference Deployment

### Negative

- None

### Caveats

- Check completion fires none of the README's `update` triggers, so a PR whose check fails after the 09:00 post, or recovers later, shows the old bucket and marker until the next PR, review or comment event or the next post
- Collapsed rows have no marker: a bot PR demoted by a failing check sits in "waiting for author" without saying why
- Pending required checks do not demote, so a PR can read as ready for the minutes its checks run
- `isRequired` reads only contexts that have reported. A required check that never started is not seen, so such a PR stays ready
- More than 100 contexts on a commit: only the first 100 are read
- Not covered by a fixture: a failed check followed by a successful re-run. GitHub's docs say nothing about whether the rollup keeps the failed attempt, and third-party reports disagree. If it does, a fixed PR stays demoted until a new commit
- Not verified for a fine-grained PAT or a classic PAT: both are expected to behave like the App token, and a failure degrades to the warning

### Neutral

- The notification text counts the demoted PR under "waiting for author"

## Justification

### Why check status is its own request for approved PRs

The enrichment query runs for every open PR and a failure scoped to a repository fails the whole fetch. Adding `statusCheckRollup` there would put a token that cannot read it in the way of every run, and no GitHub page documents what such a token receives. A separate request limits the damage to a warning. It also costs nothing for PRs that cannot reach "ready to merge", and leaves `GetPRs` and the merged paths untouched.
