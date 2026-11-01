# Demote approved PRs with failing checks

date: 2026-11-01
status: draft

Issue: [hellej/pr-slack-reminder-action#100](https://github.com/hellej/pr-slack-reminder-action/issues/100). The work lands on the feature branch `claude/fervent-wozniak-lwnedy` and its PR, not on `main`.

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

Serves § **Purpose**: "ready to merge" stops naming PRs whose merge button is red, which is the trust the section exists to earn. A push no longer flips a red PR to ready while its fix is still unproven. Sized for § **Reference Deployment**: 0 to 8 open PRs, so the extra request is at most one.

### Non-goals

- No `❌ CI` entry on unreviewed PRs, or on approved PRs already in "waiting for author" for another reason
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
- `OpenPRsResult` gains `CheckStatusErr error`: set when the phase failed in whole or in part. The PRs it covers stay `RequiredChecksUnknown`
- `state.State` gains `FailingRequiredCheckPRs []models.PullRequestRef` (JSON key `failingRequiredCheckPRs`, omitted when empty): the open PRs the run last saw as failing
- `state.PRsWithRememberedFailingChecks(prs []githubclient.PR, loadedState *state.State) []githubclient.PR` returns copies in which every `RequiredChecksUnknown` PR whose ref the loaded state lists is `RequiredChecksFailing`. A nil state changes nothing
- `state.FailingRequiredCheckPRRefs(prs []githubclient.PR) []models.PullRequestRef` returns the refs of the `RequiredChecksFailing` PRs, in the PRs' order
- `Run` in `run.go`:
  - Right after `loadPreviousState`, applies `PRsWithRememberedFailingChecks` to the open PRs, passing the loaded state, nil when it did not load. Both tracks and both modes then read the same PRs
  - Where it sets the canvas hash on the state to save, sets `FailingRequiredCheckPRs` from those PRs too. So every state saved carries this run's set, in both modes and on every save path
  - A PR that leaves the open fetch (merged, closed, snoozed, filtered out, or turned draft with the canvas off) leaves the set
- `prview.PR.GetNextAction` reads `RequiredChecksFailing` in its first check: it keeps an approved PR out of `NextActionReadyToMerge`, so it is `NextActionWaitingForAuthor`, as for a conflict
- `prview.PR.IsHeldBackByFailingChecks()` is true when the failing check alone keeps an approved PR out of "ready to merge": approved, no waiting thread, no conflict, `RequiredChecksFailing`. It is what the `❌ CI` entry reads
- `❌ CI` is the last group of the reviewer list, after the approvers and commenters, on an open row in the message and on the canvas
  - `prview.GetReviewersTextSegments` gains a `hasFailingChecks bool` parameter and renders ` / ❌ CI` before the closing parenthesis, so both builders share it
  - With no approvers or commenters it renders ` (❌ CI)`. A held-back PR is approved, so no held-back row reaches that case
  - Only open rows pass `IsHeldBackByFailingChecks()`. Draft and merged rows pass false
- Action inputs: none new. Permissions: none new
  - `statusCheckRollup` and `isRequired` populated under `pull-requests: read` alone on `GITHUB_TOKEN` in a public repository. See docs/third-party-facts.md § `PullRequest.statusCheckRollup` and `isRequired` populate under `pull-requests: read` on `GITHUB_TOKEN`, in a public repository
  - Step 1's checkpoint covers a private repository under the E2E App token

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

Marked stale by the next post, when it was posted while red
⚠️ Stale, updated Oct 31 at 09:00
*💬 Waiting for author*
  • Fix retry loop   2 days ago   by alice (✅ bob / 💬 carol / ❌ CI)
```

The canvas row reads `**[Fix retry loop](url)** _2 days ago_ by alice (✅ bob / 💬 carol / ❌ CI)` under `## 💬 Waiting for author`, and follows the same states.

## Breaking or non-breaking

Non-breaking, **minor**. Bucketing and row text change for users with a failing required check, with no input or permission change, as plans 012 and 014 did.

- State gains `failingRequiredCheckPRs`. An artifact saved before it decodes no set, so the first run after upgrading counts a PR without a failing required check as green
- The merged PR cache saves `RequiredChecks` beside `Conflicting`. A cache saved before it decodes `""`, unknown. Merged PRs are not fetched for check status, so it stays unknown
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

- `buildPullRequestsQuery` takes a `pullRequestSelection` instead of a `pullRequestFragment`. It is a small struct holding the fragment definitions text, empty when there is none, and a `body` func from the number variable to the alias body
- `pullRequestFragment` gains a method returning the `pullRequestSelection` that carries its definition and spreads it, so both existing callers keep their query text byte for byte
- The check status selection has no definitions and an inline body
- The existing query tests and the snapshots are the regression net

### 1. Fetch required check status for approved PRs (checkpoint)

Touches `internal/apiclients/githubclient`, `testhelpers/mockgithubclient`, `cmd/pr-slack-reminder` (`run.go`).

- `PR.RequiredChecks`, set by `client.fetchRequiredCheckStatus`, called at the end of `FindOpenPRs` after `excludeSnoozedPRs`. See [§ Why check status is its own request for approved PRs](#why-check-status-is-its-own-request-for-approved-prs)
- Requested PRs: not draft, `HasCompleteReviewerInfo`, and at least one approver. A PR outside that set stays `RequiredChecksUnknown`
- The phase batches 25 PRs per request, runs at most `defaultGitHubAPIConcurrencyLimit` requests at a time and goes through `retry.DefaultPolicy` like every GraphQL request
- `requiredChecksStatus` classifies the decoded contexts, as listed in § Target shape. A table test covers each row of that list, `hasNextPage` with all passing included
- A failure never fails `FindOpenPRs`:
  - A query, repository or transport error leaves every PR of that batch unknown
  - A per-PR error (`pullRequestError`, as `failsOnlyOnePullRequest` reads it) leaves only that PR unknown
  - Either sets `OpenPRsResult.CheckStatusErr`, joining the batches' errors
  - Batches fail independently: unlike `enrichPRsWithReviewInfo`'s `errgroup.WithContext`, one failed batch does not cancel the others
  - A decode error and an expired context count as a failed batch too. Nothing in the phase returns an error from `FindOpenPRs`
- A rollup with `hasNextPage` true is logged
- Log the phase's `rateLimit.cost` through `logRateLimit`, and one line per requested PR with its status. A failed batch logs its error there too
- `prFetchTimeout` in `run.go` grows from 2 to 3 minutes, and its comment names the third phase: a listing, a wave of enrichment batches and a wave of check status batches, each call retried for up to 52s. The merged list, merge stats and by-ref fetches share the constant and get the longer limit too, which the comment says in one line
- `mockgithubclient`:
  - The transport routes a query containing `statusCheckRollup` before its `pullRequest(number:` branch. The default answer is a null rollup, unknown, so existing fixtures with an approved PR keep their buckets
  - `MockGitHubClientOptions.FailingRequiredCheckPRNumbers []int`, `PassingRequiredCheckPRNumbers []int` and `PendingRequiredCheckPRNumbers []int` answer those PRs with one required check run that failed, passed or is in progress
  - `MockGitHubClientOptions.CheckStatusError error` fails this request only, as `MergedPRsSearchError` does for the merged search
  - `FetchRecording.CheckStatusRequests` records the refs of each request, so a test can assert which PRs were asked for. These requests never enter `EnrichmentRequests`
- Update `githubclient.spec.md`: the status, the phase, the failure behaviour, and Oddities for the 100-context limit and for a re-run (see § Consequences)
- Verifying it done:
  - `make test`, and the new phase's request carries exactly the approved non-draft refs
  - The repository owner sets up, in a private test repository the E2E runs list, a PR approved by a human other than its author, whose required check fails. The check is required through a ruleset, the case docs/third-party-facts.md does not cover yet. The PR is not authored by `alice` in `pr-slack-reminder-test-repo-1` and carries no `ignore-reminder` label, or the grouped runs drop it
  - The E2E App token keeps its current permissions, which the owner confirmed hold neither `checks` nor `statuses`
  - After pushing the branch, the Build job log shows, for the fixture PR: its status line reading `failing`, the phase's `rateLimit.cost` with its difference against the last `main` Build run (state it in the PR, per AGENTS.md § GitHub API Budget), and no check status error line
  - If an error line shows, stop and raise it with the user: a permission is needed and the plan has to say which
  - Add what this confirms to docs/third-party-facts.md, and amend the probe entry's "not shown" list

### 2. Keep failing PRs out of "ready to merge" in `prview`

Touches `internal/prview`.

- `GetNextAction` and `IsHeldBackByFailingChecks()` as in § Target shape. Size: one condition, one method
- Update `prview.spec.md`: the check list, the "only demotes" line beside the conflict one, and an Oddities line that a failing check on an unapproved PR moves nothing

### 3. Render `❌ CI` in the reviewer list

Touches `internal/prview`, `internal/messagebuilder`, `internal/canvasbuilder`, `cmd/pr-slack-reminder` snapshot tests.

- `prview.GetReviewersTextSegments` takes `hasFailingChecks`, as in § Target shape. ~4 call sites: the message's `getReviewersElements`, the canvas's `renderReviewers` and its WIP and merged rows
- `messagebuilder.buildOpenPRBulletPoint` and `canvasbuilder.renderOpenPRRow` pass `IsHeldBackByFailingChecks()`. The other callers pass false
- `❌ CI` is part of the glue segments, so neither builder escapes or styles it as a name
- Collapsed rows carry no reviewer list, so no `❌ CI`
- Update `prview.spec.md`'s `GetReviewersTextSegments` line, `messagebuilder.spec.md` and `canvasbuilder.spec.md`
- Add a failing-check PR to three fixture sets, then `make update-test-snapshots`:
  - `snapshotScenarios()` in `cmd/pr-slack-reminder/snapshot_test.go`, which serves the post and marked-stale snapshots
  - The update-mode table of `TestSnapshotsUpdateMode`, for the edited message
  - The fixtures of `TestBuildMarkdownSnapshots` in `internal/canvasbuilder/canvasbuilder_test.go`, for the canvas row
- Expect the snapshots holding those fixtures to change, and every state snapshot with a cached merged PR to gain the `RequiredChecks` key

### 4. Keep a failing status until the required checks pass, through the state

Touches `internal/state`, `cmd/pr-slack-reminder` (`run.go`), `AGENTS.md`.

- `State.FailingRequiredCheckPRs`, `PRsWithRememberedFailingChecks` and `FailingRequiredCheckPRRefs` as in § Target shape. Size: one field, two pure functions
- `Run` applies and saves them as in § Target shape. It logs one line per PR made failing from the state, naming it
- `main_test.go` covers each CI status rule of § Requirements and each `Run` bullet of § Target shape, in post and update mode, and a listed PR this run did not fetch for check status (no longer approved, or a draft). `getTestState` gains an option for the failing refs
- The state snapshots holding the step 3 fixture list the failing PR
- Update `state.spec.md`:
  - The opening sentence and § Behaviour: the field and both functions
  - § Oddities: the field was added without a schema bump, and the "An update run rewrites only ..." line gains the set
- Update `run.spec.md` § Behaviour:
  - Where the set is applied and saved
  - "A deleted or kept message, or a failed edit: it saves the loaded state back unchanged" now says: except for the canvas hash and the set
- Update AGENTS.md § Architecture, the **State** bullet: it also holds the PRs last seen with a failing required check, and both modes apply them to the open PRs before either track

### 5. Warn when the check status fetch fails

Touches `cmd/pr-slack-reminder` (`run.go`), `run.spec.md`.

- After `findOpenPRs`, a non-nil `CheckStatusErr` goes through `logWarning`: `Failed to fetch required check status: <error>`. The run goes on, each PR keeping the status the state remembers
- `main_test.go`, using `CheckStatusError` and no state listing: a failed phase warns and leaves the PR in "ready to merge", in post and in update mode
- Update `run.spec.md`'s warning list

### 6. Check the message and canvas live

Touches no files. The step 1 fixture PR is the setup.

- `pr-reminder.yml` lists this repository only, so it cannot show the fixture PR. Read the branch's Build E2E runs instead:
  - The grouped canvas runs ("Run with filters, canvas link and merge stats: post" and its update) show the fixture PR under "Waiting for author" with `❌ CI` in its reviewer list, in the message and on the canvas
- Done means both surfaces show `❌ CI` and neither shows the PR under "Ready to merge"
- The sticky status is not checked live: the fixture's check would have to run during the E2E job. `main_test.go` covers it

### 7. Docs

Touches `README.md`.

- The bucket paragraph under `README.md` § PR Tracker Canvas: "ready to merge" is an approved PR with nothing outstanding and no failing required check. A failing required check puts an approved PR under waiting for the author, with `❌ CI` in its reviewer list, until every required check passes
- The line under the H1 that lists the three buckets needs no change
- The screenshots in `docs/examples/` stay: no sample shows a failing check
- `README.md` § Required Permissions: one line saying check status needs nothing beyond `pull-requests: read`, once step 1 has shown it
- `README.md` § 3. Update Mode Enabled: the trigger example gains an optional `workflow_run` trigger, commented as optional, listing the workflows that run the required checks:
  ```yaml
    workflow_run:
      workflows: ["CI"] # the workflows running your required checks
      types: [completed]
  ```
  - One line on why: without it, a fixed PR stays red until the next PR, review or comment event or post
  - One line on its limits: it reacts to GitHub Actions workflows only, not to an external CI app's checks, and the reminder workflow file must be on the default branch. See docs/third-party-facts.md § `workflow_run` runs the default branch's workflow file on another workflow's completion, and `check_suite` and `check_run` skip suites GitHub Actions created
  - One line on cost: each completion of a listed workflow runs an `update`, about 1 GraphQL point per open PR plus 1 per repository. See AGENTS.md § GitHub API Budget
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
- Collapsed rows have no reviewer list, so no `❌ CI`: a bot PR demoted by a failing check sits in "waiting for author" without saying why
- Without a loaded state (first run, failed load), pending checks read as green, so a PR can read as ready for the minutes its checks run
- `isRequired` reads only contexts that have reported. A required check that never started is not seen. So a PR reads green once the required checks that reported have passed, before a slower required check in another workflow registers
- More than 100 contexts on a commit: a failure among the first 100 still reads failing, but the PR never reads passing, so a red PR stays red
- A snoozed or filtered-out PR leaves the set, and so does one turned draft while the canvas is off. When it comes back with pending checks, it reads green
- A PR whose required checks are all removed stays red until it leaves the open fetch: no required context reads unknown
- In update mode, a state that fails to load fails the run, but the canvas refresh still runs, without the set
- Not covered by a fixture: a failed check followed by a re-run. GitHub's docs say nothing about whether the rollup keeps the failed attempt, and third-party reports disagree. If the rollup shows only the re-run's pending attempt, the state keeps the PR red until it passes. If it keeps the failed attempt beside the passing re-run, the PR stays red until a new commit. See docs/third-party-facts.md § GitHub documents nothing about re-run attempts in `statusCheckRollup.contexts`, and third-party reports disagree
- Unverified for a fine-grained PAT and a classic PAT. A token lacking the permission either fails the request, which degrades to the warning, or gets a null rollup without an error, which leaves every PR unknown with no warning. See docs/third-party-facts.md § No GitHub page documents the token permission for any GraphQL field

### Neutral

- The notification text counts the demoted PR under "waiting for author"

## Justification

### Why check status is its own request for approved PRs

The enrichment query runs for every open PR and a failure scoped to a repository fails the whole fetch. Adding `statusCheckRollup` there would put a token that cannot read it in the way of every run, and no GitHub page documents what such a token receives. A separate request limits the damage to a warning. It also costs nothing for PRs that cannot reach "ready to merge", and leaves `GetPRs` and the merged paths untouched.

### Why the last status lives in the state

The run already loads and saves the state, so remembering a set of refs costs no request. Reading the status from GitHub's history instead means fetching earlier commits' rollups for every pending PR, which costs points each run. It also misses a re-run on the same commit, if the rollup shows only the pending attempt, which no GitHub page settles. See docs/third-party-facts.md § GitHub documents nothing about re-run attempts in `statusCheckRollup.contexts`, and third-party reports disagree.
