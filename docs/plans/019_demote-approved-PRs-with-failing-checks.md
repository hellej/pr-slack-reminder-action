# Demote approved PRs with failing checks

date: 2026-11-01
status: draft

Issue: [hellej/pr-slack-reminder-action#100](https://github.com/hellej/pr-slack-reminder-action/issues/100).

## Requirements

- An approved PR with a failing required check leaves "ready to merge" for "waiting for author", in the message and on the canvas
- Its row says why: a last `❌ CI` entry in the reviewer list, as in `(✅ Dana Davis / 💬 Erin Evans / ❌ CI)`
- The CI status changes only on evidence:
  - Red to green only when every required check has completed and passed
  - Green to red only when a required check has failed
  - Pending checks, no checks reported yet, or a failed check status fetch keep the last run's status
- The last run's status comes from the saved state. Without a loaded state (first run, failed load), a PR without a failing required check counts as green
- Required means required by branch protection or a ruleset, so a failing optional check never demotes
- A PR in a repository with no required checks keeps today's bucket
- No new action input
- Check status is fetched for approved, non-draft open PRs only
- The README suggests an optional `update` trigger on CI completion, so a fixed PR turns green without waiting for the next PR event

Serves § **Purpose**: "ready to merge" stops naming PRs whose merge button is red, which is the trust the "ready to merge" section exists to earn. A push no longer flips a red PR to ready while its fix is still unproven. Sized for § **Reference Deployment**: 0 to 8 open PRs, so the extra request is at most one.

### Non-goals

- No `❌ CI` entry on an unapproved PR: its check status is not fetched, only remembered from the state
- No new workflow trigger in this repository's own workflows. The README gains an optional one
- No change to "waiting for review": a PR nobody approved stays there, whatever its checks say
- No change to merged, draft or collapsed rows

## Target shape

- `githubclient.PR` gains `RequiredChecks RequiredChecksStatus`, a string type:
  - `RequiredChecksUnknown` (`""`, the zero value): not fetched, fetch failed, or no evidence either way
  - `RequiredChecksPassing` (`"passing"`)
  - `RequiredChecksFailing` (`"failing"`)
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
  - A required context is failing when it is:
    - a `CheckRun` with `status` `COMPLETED` and a `conclusion` other than `SUCCESS`, `NEUTRAL` or `SKIPPED`
    - a `StatusContext` with `state` `FAILURE` or `ERROR`
  - A required context is passing when it is a completed `CheckRun` with `SUCCESS`, `NEUTRAL` or `SKIPPED`, or a `StatusContext` with `SUCCESS`
  - The PR is `RequiredChecksFailing` when any required context is failing
  - Otherwise `RequiredChecksPassing` when there is at least one required context, every required context is passing, and `hasNextPage` is false
  - Otherwise `RequiredChecksUnknown`: a null rollup, no required context, a pending required context, or more than 100 contexts
- `OpenPRsResult` gains `CheckStatusErr error`: set when the phase failed in whole or in part. The PRs of the failed batches stay `RequiredChecksUnknown`
- `state.State` gains `PRsWithFailingRequiredChecks []models.PullRequestRef` (JSON key `prsWithFailingRequiredChecks`, omitted when empty): the open PRs the run last saw as failing
- `state.PRsWithRememberedFailingChecks(prs []githubclient.PR, loadedState *state.State) []githubclient.PR` returns copies in which every `RequiredChecksUnknown` PR whose ref the loaded state lists is `RequiredChecksFailing`. A nil state changes nothing
- `state.RefsOfPRsWithFailingRequiredChecks(prs []githubclient.PR) []models.PullRequestRef` returns the refs of the `RequiredChecksFailing` PRs, in the PRs' order
- `Run` in `run.go`:
  - Right after `loadPreviousState`, applies `PRsWithRememberedFailingChecks` to the open PRs, passing the loaded state, nil when it did not load. Both tracks and both modes then read the same PRs
  - Where it sets the canvas hash on the state to save, sets `PRsWithFailingRequiredChecks` from those PRs too. So every state saved carries this run's set, in both modes and on every save path
  - A PR that leaves the open fetch (merged, closed, snoozed, filtered out, or turned draft with the canvas off) leaves the set
- `prview.PR.GetNextAction` reads `RequiredChecksFailing` in the first of its three ordered conditions. A failing status keeps an approved PR out of `NextActionReadyToMerge`: the PR gets `NextActionWaitingForAuthor`, as for a conflict
- `prview.MergeBlocker`, a string type naming one thing that blocks a PR's merge. Its value is the text the row shows. This plan adds one, `MergeBlockerFailingChecks` (`"CI"`). [Plan 020](020_show-merge-blockers-on-PR-rows.md) adds conflicts
- `prview.PR.MergeBlockers() []MergeBlocker` returns `MergeBlockerFailingChecks` when the PR has an approver and `RequiredChecks` is `RequiredChecksFailing`, also when a thread or a conflict keeps that PR in "waiting for author". It is what the `❌ CI` entry reads
  - An unapproved PR gets no `CI`: on it, a failing status can only be one the state remembers
  - `MergeBlockers()` returns nil on a PR whose embedded `githubclient.PR` is nil
- `❌ CI` is the last group of the reviewer list, after the approvers and commenters, on an open row in the message and on the canvas
  - `prview.GetReviewersTextSegments` gains a `mergeBlockers []MergeBlocker` parameter and renders ` / ❌ ` and the blockers joined by `, ` before the closing parenthesis, so both builders share it
  - With no approvers or commenters it renders ` (❌ CI)`. A PR with `CI` is approved, so no row reaches that case before plan 020
  - Only open rows pass `MergeBlockers()`. Draft and merged rows pass nil
- Action inputs: none new. Permissions: `checks: read` in a private repository, none new in a public one. See docs/third-party-facts.md § `PullRequest.statusCheckRollup` needs `checks: read` in a private repository: an App token without it gets `FORBIDDEN` on the field
  - `statusCheckRollup` and `isRequired` populate under `pull-requests: read` alone on `GITHUB_TOKEN` in a public repository. See docs/third-party-facts.md § `PullRequest.statusCheckRollup` and `isRequired` populate under `pull-requests: read` on `GITHUB_TOKEN`, in a public repository
  - Step 1's checkpoint covers a required check set by a ruleset in the public `hellej/pr-slack-reminder-test-repo-3`, and an approved PR without required checks in a private repository under the E2E App token

Message states for an approved PR whose required check fails, then gets a fix:

```
Posted at 09:00, required check failed
*💬 Waiting for author*
  • Fix retry loop   2 days ago   by alice (✅ bob / 💬 carol / ❌ CI)

Edited by an `update` after the fix's push, checks still running: no change
*💬 Waiting for author*
  • Fix retry loop   2 days ago   by alice (✅ bob / 💬 carol / ❌ CI)

Edited by `update` once every required check passed
*✅ Ready to merge*
  • Fix retry loop   2 days ago   by alice (✅ bob / 💬 carol)

Marked stale by the next post, after the message was posted while the PR was red
⚠️ Stale, updated Oct 31 at 09:00
*💬 Waiting for author*
  • Fix retry loop   2 days ago   by alice (✅ bob / 💬 carol / ❌ CI)
```

The canvas row reads `**[Fix retry loop](url)** _2 days ago_ by alice (✅ bob / 💬 carol / ❌ CI)` under `## 💬 Waiting for author`, and follows the same states.

## Breaking or non-breaking

Non-breaking, **minor**. Bucketing and row text change for users with a failing required check, with no input change, as plans 012 and 014 did.

- A private repository needs `checks: read` for the check status fetch. A workflow without it still succeeds: each run with an approved PR warns `Failed to fetch required check status`, and a PR the state does not remember as failing files as it does today. Nothing that worked stops working, so it stays minor. The README names the permission

- State gains `prsWithFailingRequiredChecks`. An artifact saved before it decodes no set, so the first run after upgrading counts a PR without a failing required check as green
- `PR.RequiredChecks` carries `omitempty`, so the merged PR cache saves it only when known. Merged PRs are not fetched for check status, so a cache never carries the key, and a cache saved before the field existed decodes `""`, unknown
- The first run after upgrading re-buckets every PR, and the canvas rewrites once wherever a PR moves
- A downgrade ignores both keys

## Summary

- **R1** Let `buildPullRequestsQuery` take a selection per PR
- **1** Fetch required check status for approved PRs (checkpoint)
- **2** Keep failing PRs out of "ready to merge" in `prview`
- **3** Render `❌ CI` in the reviewer list, in the message and on the canvas
- **4** Keep a failing status until the required checks pass, through the state
- **5** Warn when the check status fetch fails
- **6** Check the message and canvas live
- **7** Docs

## Steps

### R1. Let `buildPullRequestsQuery` take a selection per PR

Touches `internal/apiclients/githubclient` (`graphqlquery.go`, `openprsfetch.go`, `getprsbyreffetch.go`). Refactor, no behaviour change, ~2 call sites.

- `buildPullRequestsQuery` takes a `pullRequestSelection` instead of a `pullRequestFragment`
- `pullRequestSelection` is a small struct holding the fragment definitions text (empty when there is none) and a `body` func from the number variable to the alias body
- `pullRequestFragment` gains a method that returns a `pullRequestSelection` holding the fragment's own definition, with a body that spreads the fragment. Both existing callers keep their query text byte for byte
- The check status selection has no definitions and an inline body
- The existing query tests and the snapshots are the regression net

### 1. Fetch required check status for approved PRs (checkpoint)

Touches `internal/apiclients/githubclient`, `testhelpers/mockgithubclient`, `cmd/pr-slack-reminder` (`run.go`).

- `PR.RequiredChecks` is set by `client.fetchRequiredCheckStatus`, which `FindOpenPRs` calls at its end, after `excludeSnoozedPRs`. See [§ Why check status is its own request for approved PRs](#why-check-status-is-its-own-request-for-approved-prs)
- Requested PRs: not draft, `HasCompleteReviewerInfo`, and at least one approver. A PR outside that set stays `RequiredChecksUnknown`
- The phase batches 25 PRs per request, runs at most `defaultGitHubAPIConcurrencyLimit` requests at a time and goes through `retry.DefaultPolicy` like every GraphQL request
- `classifyRequiredChecks` classifies the decoded contexts, as listed in § Target shape. A table test covers each row of that list, including `hasNextPage` true with every context passing
- `PR.RequiredChecks` carries `omitempty`, so no existing state snapshot gains the key. The merged PR cache never holds a known status
- A failure never fails `FindOpenPRs`:
  - A query, repository or transport error leaves every PR of that batch unknown
  - A per-PR error leaves only that PR unknown: a `pullRequestError`, as `failsOnlyOnePullRequest` reads it, a field error below the PR, or a PR missing from the answer. A token lacking the permission could get a field error on `statusCheckRollup`, so that error goes into `CheckStatusErr` and the warning like any other
  - Either sets `OpenPRsResult.CheckStatusErr`, joining the batches' errors
  - A batch error names the repository when it is scoped to one. `enrichPRsWithReviewInfoError` is renamed `pullRequestBatchError`, and the enrichment's error text does not change
  - Batches fail independently: unlike `enrichPRsWithReviewInfo`'s `errgroup.WithContext`, one failed batch does not cancel the others
  - A decode error and an expired context count as a failed batch too
- A rollup with `hasNextPage` true is logged
- Log the phase's `rateLimit.cost` through `logRateLimit`, and one line per requested PR with its status. A failed batch logs its error there too
- `prFetchTimeout` in `run.go` grows from 2 to 3 minutes, and its comment names the third phase: a listing, a wave of enrichment batches and a wave of check status batches, each call retried for up to 52s. The merged list, merge stats and by-ref fetches share the constant and get the longer limit too. The comment says so in one line
- `mockgithubclient`:
  - The transport routes a query containing `statusCheckRollup` before its `pullRequest(number:` branch. The default answer is a null rollup, unknown, so existing fixtures with an approved PR keep their buckets
  - `MockGitHubClientOptions.FailingRequiredCheckPRNumbers []int`, `PassingRequiredCheckPRNumbers []int` and `PendingRequiredCheckPRNumbers []int` answer those PRs with one required check run that failed, passed or is in progress
  - `MockGitHubClientOptions.CheckStatusError error` fails this request only, with a 403. `MergedPRsSearchError` does the same for the merged search
  - `FetchRecording.CheckStatusRequests` records the refs of each request, so a test can assert which PRs were asked for. These requests never enter `EnrichmentRequests`
- Update `githubclient.spec.md`: the status, the phase, the failure behaviour, and Oddities for the 100-context limit and for a re-run (see § Consequences)
- Verifying it done:
  - `make test`, and the new phase's request carries exactly the approved non-draft refs
  - The owner's free GitHub plan cannot require checks through a ruleset in a private repository. See docs/third-party-facts.md § Rulesets need GitHub Pro, Team or Enterprise Cloud in a private repository, and are free in a public one. So the fixture lives in a public repository, `hellej/pr-slack-reminder-test-repo-3`, which the Build E2E grouped canvas runs list. The repository owner sets it up as a standing fixture, PR 14, kept after this plan for [plan 020](020_show-merge-blockers-on-PR-rows.md):
    - A base branch `fixture/required-check-base`, cut from `main`, and a ruleset targeting only that branch that requires a status check named `fixture-check`. That is the ruleset case docs/third-party-facts.md does not cover yet
    - A PR into that base branch, approved by a human other than its author, carrying no `ignore-reminder` label
      - PR 14's author is `github-actions[bot]`, opened by repo-3's `create-fixture-pr` workflow, and `hellej` approved it: `hellej` is repo-3's only human collaborator, and a bot's approval does not count
    - A failing `fixture-check` status on the PR's head commit, posted by hand: `gh api repos/hellej/pr-slack-reminder-test-repo-3/statuses/<head sha> -f state=failure -f context=fixture-check`. No workflow file is added
  - The owner also keeps an approved, non-draft open PR in `hellej/pr-slack-reminder-test-repo-1`, a Dependabot one, so the Build E2E runs read `statusCheckRollup` in a private repository, per AGENTS.md § Reference Deployment. Dependabot closes a PR a newer version supersedes, so the owner replaces it when that happens
  - After pushing the implementation branch, the Build E2E grouped canvas runs ("Run with filters, canvas link and merge stats: post" and "Run with filters and canvas link: update") read PR 14 under the E2E App token. Their job log shows its status line reading `failing` and no check status error line
  - The Build job log, under the E2E App token, shows the Dependabot PR's status line reading `unknown`, the phase's `rateLimit.cost` with its difference against the last `main` Build run (state it in the PR, per AGENTS.md § GitHub API Budget), and no check status error line. The E2E App token holds read-only `checks`, granted after the first run's `FORBIDDEN`, and still no `statuses`
  - If an error line shows in either log, stop and raise it with the user: a permission is needed and the plan has to say which
  - Result: an error line showed in the Build log. The App token without `checks` got `FORBIDDEN` on `statusCheckRollup` for PR 49. With read-only `checks` granted, the re-run read PR 49 as `unknown` with no error. The permission is `checks: read`, now in the README and docs/third-party-facts.md
  - Add what this confirms to docs/third-party-facts.md, and amend the "not shown" list of docs/third-party-facts.md § `PullRequest.statusCheckRollup` and `isRequired` populate under `pull-requests: read` on `GITHUB_TOKEN`, in a public repository

### 2. Keep failing PRs out of "ready to merge" in `prview`

Touches `internal/prview`.

- `GetNextAction`, `MergeBlocker` and `MergeBlockers()` as in § Target shape. Size: one condition, one type, one method
- A package test on `MergeBlockers()` for what the snapshots do not reach: an unapproved PR with a remembered failing status, and a nil embedded PR. Also an approved failing PR with a waiting thread, and an approved failing PR with a conflict. The mock has no waiting thread or conflict row, so no snapshot shows `❌ CI` on either
- Update `prview.spec.md`: a `MergeBlocker` and `MergeBlockers()` line, the list of conditions, the "only demotes" line beside the conflict one, and an Oddities line that a failing check on an unapproved PR moves nothing

### 3. Render `❌ CI` in the reviewer list

Touches `internal/prview`, `internal/messagebuilder`, `internal/canvasbuilder`, `cmd/pr-slack-reminder` snapshot tests.

- `prview.GetReviewersTextSegments` takes `mergeBlockers`, as in § Target shape. ~4 call sites: the message's `getReviewersElements`, the canvas's `renderReviewers`, the canvas's WIP row and the canvas's merged row
- `messagebuilder.buildOpenPRBulletPoint` and `canvasbuilder.renderOpenPRRow` pass `MergeBlockers()`. The other callers pass nil
- `❌ CI` is part of the glue segments, so neither builder styles it as a name
- Collapsed rows carry no reviewer list, so no `❌ CI`
- Update `prview.spec.md`'s `GetReviewersTextSegments` line, `messagebuilder.spec.md` and `canvasbuilder.spec.md`
- Add a failing-check PR to three fixture sets, then `make update-test-snapshots`:
  - `snapshotScenarios()` in `cmd/pr-slack-reminder/snapshot_test.go`, which serves the post and marked-stale snapshots
  - The update-mode table of `TestSnapshotsUpdateMode`, for the edited message
  - The fixtures of `TestBuildMarkdownSnapshots` in `internal/canvasbuilder/canvasbuilder_test.go`, for the canvas row
  - Both cmd fixture sets pass the failing PR through a `prNumbersWithFailingRequiredChecks` field to `FailingRequiredCheckPRNumbers`
- The canvas WIP and merged fixtures, and the message's merged row test, are approved and failing too, with their expected output unchanged, so a row naming the failing check outside an open section fails a test
- Expect the snapshots holding those fixtures to change. No state snapshot gains a `RequiredChecks` key: it is `omitempty`, and a cached merged PR is always unknown

### 4. Keep a failing status until the required checks pass, through the state

Touches `internal/state`, `cmd/pr-slack-reminder` (`run.go`), `AGENTS.md`.

- `State.PRsWithFailingRequiredChecks`, `PRsWithRememberedFailingChecks` and `RefsOfPRsWithFailingRequiredChecks` as in § Target shape. Size: one field, two pure functions
- `Run` applies and saves them as in § Target shape. It logs one line per PR made failing from the state, naming it
- `cmd/pr-slack-reminder/requiredchecks_test.go`, beside `main_test.go`, covers each CI status rule of § Requirements and each `Run` bullet of § Target shape, in post and update mode, and a PR the loaded state lists as failing that this run did not fetch for check status (no longer approved, or a draft). It also covers update mode's deleted and kept message paths saving this run's set. `getTestState` gains an option for the failing refs
- `run.go`'s `withRememberedFailingChecks` calls `state.PRsWithRememberedFailingChecks` and logs each PR it made failing
- The state snapshots built from the step 3 fixture carry the failing PR in `prsWithFailingRequiredChecks`
- Update `state.spec.md`:
  - The opening sentence and § Behaviour: the field and both functions
  - § Oddities: the field was added without a schema bump, and the "An update run rewrites only ..." line gains the set
- Update `run.spec.md` § Behaviour:
  - Where the set is applied and saved
  - "A deleted or kept message, or a failed edit: it saves the loaded state back unchanged" now says: except for the canvas hash and the set
- Update AGENTS.md § Architecture, the **State** bullet: it also holds the PRs last seen with a failing required check, and both modes apply them to the open PRs before either track

### 5. Warn when the check status fetch fails

Touches `cmd/pr-slack-reminder` (`run.go`), `run.spec.md`.

- Right after `findOpenPRs`, before the state load, a non-nil `CheckStatusErr` goes through `logWarning`: `Failed to fetch required check status: <error>`. The run goes on, each PR keeping the status the state remembers
- `requiredchecks_test.go`, using `CheckStatusError` and no state listing: a failed phase warns and leaves the PR in "ready to merge", in post and in update mode
- Update `run.spec.md`'s warning list

### 6. Check the message and canvas live

Touches no files. The step 1 fixture PR is the setup.

- After step 5, the branch's Build E2E grouped canvas runs show PR 14 under "Waiting for author" with `❌ CI` in its reviewer list, in the dev channel's message and on its canvas
- Then the owner posts a pending status on PR 14's head commit: `gh api repos/hellej/pr-slack-reminder-test-repo-3/statuses/<head sha> -f state=pending -f context=fixture-check`
- The next Build E2E post and update pair, within the state artifact's 1-day retention, still shows `❌ CI`, and its job log has the line naming PR 14 as made failing from the state
- Last, the owner posts `-f state=failure` again, so the fixture stays red for plan 020
- Done means both surfaces show `❌ CI` and neither shows the PR under "Ready to merge"

### 7. Docs

Touches `README.md`.

- The bucket paragraph under `README.md` § PR Tracker Canvas: "ready to merge" is an approved PR with nothing outstanding and no failing required check. A failing required check puts an approved PR under waiting for the author, with `❌ CI` in its reviewer list, until every required check passes
- The line under the H1 that lists the three buckets needs no change
- The screenshots in `docs/examples/` stay: no sample shows a failing check
- `README.md` § Required Permissions: `checks: read` in the permissions block, and a line no wider than step 1 and the facts entries show: an App token in a private repository needs `checks: read`, `GITHUB_TOKEN` there is untested, and `GITHUB_TOKEN` in a public repository needs only `pull-requests: read`. The update-mode example's `permissions:` block gains `checks: read` too
- `README.md` § 3. Update Mode Enabled: the trigger example gains an optional `workflow_run` trigger, listing the workflows that run the required checks. Its own comment explains it, with no text under the example:
  ```yaml
    workflow_run: # optional: runs an update when a listed workflow completes
      workflows: ["CI"] # the workflows running your required checks
      types: [completed]
  ```
  - The example's `run-mode` expression and concurrency group already send a `workflow_run` event to `update` and to the non-scheduled group, so neither changes
- Done means `make check-style` passes and the trigger example stays one valid `on:` block

## Consequences

### Positive

- "Ready to merge" means the merge button is not red because of required checks
- A push to a red PR keeps it red until its checks prove the fix
- The author sees the reason on the row
- Cost scales with approved PRs, not open ones: at most one request in § Reference Deployment. The state adds no request

### Negative

- None

### Caveats

- Without the optional `workflow_run` trigger, check completion runs no `update`, so a PR keeps its bucket until the next PR, review or comment event or post: a fixed PR stays red, and one whose check fails after the 09:00 post stays ready
- The fixture PR stays open in `hellej/pr-slack-reminder-test-repo-3`, so it shows in every Build E2E grouped canvas run's message and canvas
- Collapsed rows have no reviewer list, so no `❌ CI`: a bot PR demoted by a failing check sits in "waiting for author" without saying why
- Without a loaded state (first run, failed load), pending checks read as green, so a PR can read as ready for the minutes its checks run
- A context carries `isRequired` only once it has reported. A required check that never started is not seen. So a PR reads green once the required checks that reported have passed, before a slower required check in another workflow registers
- More than 100 contexts on a commit: a failure among the first 100 still reads failing, but the PR never reads passing, so a red PR stays red
- A snoozed or filtered-out PR leaves the set, and so does one turned draft while the canvas is off. When such a PR comes back with pending checks, it reads green
- A PR whose required checks are all removed stays red until it leaves the open fetch: no required context reads unknown
- In update mode, a state that fails to load fails the run. The canvas refresh still runs, but with no remembered failing PRs applied
- Not covered by a fixture: a failed check followed by a re-run. GitHub's docs say nothing about whether the rollup keeps the failed attempt, and third-party reports disagree. If the rollup shows only the re-run's pending attempt, the state keeps the PR red until the re-run passes. If it keeps the failed attempt beside the passing re-run, the PR stays red until a new commit. See docs/third-party-facts.md § GitHub documents nothing about re-run attempts in `statusCheckRollup.contexts`, and third-party reports disagree
- Unverified for a fine-grained PAT and a classic PAT. A token lacking the permission either fails the request, which degrades to the warning, or gets a null rollup without an error, which leaves every PR unknown with no warning. See docs/third-party-facts.md § No GitHub page documents the token permission for any GraphQL field

### Neutral

- The notification text counts the demoted PR under "waiting for author"

## Justification

### Why check status is its own request for approved PRs

The enrichment query runs for every open PR and a failure scoped to a repository fails the whole fetch. Adding `statusCheckRollup` there would put a token that cannot read it in the way of every run, and no GitHub page documents what such a token receives. A separate request limits the damage to a warning. It also costs nothing for PRs that cannot reach "ready to merge", and leaves `GetPRs` and the merged paths untouched.

### Why the last status lives in the state

The run already loads and saves the state, so remembering a set of refs costs no request. Reading the status from GitHub's history instead means fetching earlier commits' rollups for every pending PR, which costs points each run. That approach also misses a re-run on the same commit, if the rollup shows only the pending attempt, which no GitHub page settles. See docs/third-party-facts.md § GitHub documents nothing about re-run attempts in `statusCheckRollup.contexts`, and third-party reports disagree.
