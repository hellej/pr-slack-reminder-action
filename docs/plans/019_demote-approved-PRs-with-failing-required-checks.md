# Demote approved PRs with failing required checks

date: 2026-10-31
status: draft

Issue: [hellej/pr-slack-reminder-action#100](https://github.com/hellej/pr-slack-reminder-action/issues/100). The work lands on the feature branch `claude/fervent-wozniak-lwnedy` and its PR, not on `main`.

## Requirements

- An approved PR with a failing required check leaves "ready to merge" for "waiting for author", in the message and on the canvas
- Its row says why: a `❌ checks failing` marker
- Pending required checks do not demote. Only a failing one does
- Required means required by the base branch's protection, so a failing optional check never demotes
- A PR with no checks or no required checks keeps today's bucket
- No new action input
- Check status is fetched for approved, non-draft open PRs only

Serves § **Purpose**: "ready to merge" stops naming PRs whose merge button is red, which is the trust the section exists to earn. Sized for § **Reference Deployment**: 0 to 8 open PRs, so the extra request is at most one.

### Non-goals

- No marker on unreviewed PRs, or on approved PRs already in "waiting for author" for another reason
- No demotion for pending checks
- No new workflow trigger for check completion. The README's `update` triggers stay as they are
- No change to `waiting for review`: a PR nobody approved stays there, whatever its checks say
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
- `prview.PR.GetNextAction` reads the flag in its first check: approved, no thread waiting for the author, no conflict and no failing required check is `NextActionReadyToMerge`. An approved PR failing that check is `NextActionWaitingForAuthor`, as it already is for a conflict
- `prview.PR.IsHeldBackByFailingChecks()` is true when the failing check alone keeps an approved PR out of "ready to merge": approved, no waiting thread, no conflict, flag set. It is what the marker reads
- Marker text: `❌ checks failing`, appended after the reviewer segment on an open row, in the message and on the canvas
- Action inputs: none new. Permissions: none new. `statusCheckRollup` and `isRequired` populated under `pull-requests: read` alone on `GITHUB_TOKEN` in a public repository. See docs/third-party-facts.md § `PullRequest.statusCheckRollup` and `isRequired` populate under `pull-requests: read` on `GITHUB_TOKEN`, in a public repository. Private repositories and App tokens are covered by step 5

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

Non-breaking, **minor**. Bucketing and row text change for users with a failing required check, with no input, permission or state change, as plans 012 and 014 did.

- State: the merged PR cache saves `HasFailingRequiredCheck` as a key beside `Conflicting`. A cache saved before it decodes false, and merged PRs are not fetched for check status, so it stays false
- The first run after upgrading re-buckets every PR, and the canvas rewrites once wherever a PR moves
- A downgrade ignores the key

## Summary

- **R1** Let `buildPullRequestsQuery` take a selection per PR
- **1** Fetch required check status for approved PRs (checkpoint)
- **2** Keep failing PRs out of "ready to merge" in `prview`
- **3** Render the `❌ checks failing` marker in the message and on the canvas
- **4** Warn when the check status fetch fails
- **5** Prove it live and measure the cost
- **6** Docs

## R1. Let `buildPullRequestsQuery` take a selection per PR

Touches `internal/apiclients/githubclient` (`graphqlquery.go`, `openprsfetch.go`, `getprsbyreffetch.go`). Refactor, no behaviour change, ~2 call sites.

- The generated alias selection gets its body from a per-PR function of the PR's number variable, instead of a `pullRequestFragment` spread
- Both existing callers keep their fragments through a small adapter, so their query text stays byte for byte the same
- The existing query tests and the snapshots are the regression net

## 1. Fetch required check status for approved PRs (checkpoint)

Touches `internal/apiclients/githubclient`, `testhelpers/mockgithubclient`.

- `PR.HasFailingRequiredCheck`, set by a new phase at the end of `FindOpenPRs`, after `excludeSnoozedPRs`
- Requested PRs: not draft, `HasCompleteReviewerInfo`, and at least one approver. A PR outside that set keeps the flag false
- The phase batches 25 PRs per request, runs at most `defaultGitHubAPIConcurrencyLimit` requests at a time and goes through `retry.DefaultPolicy` like every GraphQL request
- Classification is a pure function over the decoded contexts, as listed in § Target shape
- A failure never fails `FindOpenPRs`:
  - A query, repository or transport error leaves every PR of that batch unflagged
  - A field error scoped to one PR leaves that PR unflagged
  - Either sets `OpenPRsResult.CheckStatusErr`, joining the batches' errors
- A rollup with `hasNextPage` true is classified on its first 100 contexts, and logged
- Log `rateLimit.cost` for the phase through `logRateLimit`, and one line per requested PR with its flag
- `mockgithubclient` answers the new query from a fixture option naming the failing required checks per PR number, and records the requested refs on `FetchRecording`, so a test can assert which PRs were asked for
- Update `githubclient.spec.md`: the new flag, the phase, the failure behaviour, and Oddities for the first-100 limit and for a rerun that leaves an old failed run in the rollup (unverified, see § Consequences)
- Update `state.spec.md`: the merged PR cache's new key
- Verifying it done: `make test`, and the new phase's request carries exactly the approved non-draft refs

## 2. Keep failing PRs out of "ready to merge" in `prview`

Touches `internal/prview`, `internal/messagecontent`, `internal/canvascontent` tests.

- `GetNextAction` check 1 gains `&& !pr.HasFailingRequiredCheck`
- `IsHeldBackByFailingChecks()` as in § Target shape
- Both content packages already bucket through `GetNextAction`, so only their test fixtures gain a failing PR
- Update `prview.spec.md`: the check list, the conflict-style "only demotes" line, and the oddity that a failing check on an unapproved PR moves nothing
- Update `messagecontent.spec.md` and `canvascontent.spec.md` where they describe "approved with nothing outstanding"

## 3. Render the `❌ checks failing` marker

Touches `internal/messagebuilder`, `internal/canvasbuilder`, `cmd/pr-slack-reminder` snapshots.

- `messagebuilder.buildOpenPRBulletPoint` appends a plain text element ` ❌ checks failing` after the reviewer elements when `IsHeldBackByFailingChecks()`
- `canvasbuilder.renderOpenPRRow` appends the same text after `renderReviewers`
- Collapsed rows carry no marker, as they carry no reviewers
- Update `messagebuilder.spec.md` and `canvasbuilder.spec.md`
- Add one failing-check PR to the shared snapshot fixtures, then `make update-test-snapshots`. Expect the message and state snapshots holding that fixture to change, and every state snapshot with a cached merged PR to gain the new key

## 4. Warn when the check status fetch fails

Touches `cmd/pr-slack-reminder` (`run.go`), `run.spec.md`.

- After `findOpenPRs`, a non-nil `CheckStatusErr` goes through `logWarning`: `Failed to fetch required check status: <error>`. The run goes on with the buckets it has
- `main_test.go`: a failed phase warns and leaves the PR in "ready to merge", in post and in update mode
- Update `run.spec.md`'s warning list

## 5. Prove it live and measure the cost

Touches `.github/actions/e2e-tests`, the dev test repositories. No Go code.

- Set up, by the repository owner, in a test repository the E2E "Multi-repository run" already lists: a PR approved by a human other than its author, whose required check fails. The check is required through a ruleset, which is the case docs/third-party-facts.md does not yet cover
- The E2E app token keeps `pull-requests: read` only. A passing run then shows no extra permission is needed under an App token
- No new E2E scenario: the existing runs list that repository's PRs, so they reach the new phase
- Verifying it done, from the Build job log and the dev channel:
  - The log shows the check status request with its `rateLimit.cost`. State the difference against the last `main` Build run in the PR, per AGENTS.md § GitHub API Budget
  - The fixture PR sits under "Waiting for author" with the marker, in the message and on the canvas
  - No `Failed to fetch required check status` warning appears. If it does, stop: a permission is needed, and the plan has to say which
- Add the confirmed facts to `docs/third-party-facts.md`, and amend its probe entry's "not shown" list

## 6. Docs

Touches `README.md`, `docs/third-party-facts.md`.

- README § describing the three buckets: "ready to merge" is an approved PR with nothing outstanding and no failing required check, and a failing required check puts an approved PR under waiting for the author
- Mention the marker where the README sketches message rows
- Say once that check status needs nothing beyond `pull-requests: read`, once step 5 has shown it

## Consequences

### Positive

- "Ready to merge" means the merge button is not red because of required checks
- The author sees the reason on the row
- A failed or unreadable check fetch changes nothing: the PR keeps today's bucket
- Cost scales with approved PRs, not open ones: 0 to 3 in the reference deployment, so at most one request

### Negative

- None

### Caveats

- Check completion fires none of the README's `update` triggers, so a PR whose check fails after the 09:00 post, or recovers later, shows the old bucket until the next event or the next day's post
- Collapsed rows have no marker: a bot PR demoted by a failing check sits in "waiting for author" without saying why
- An approved PR that also has a waiting thread or a conflict gets no marker. Its row already sits in "waiting for author"
- Pending required checks do not demote, so a PR can read as ready for the minutes its checks run
- `isRequired` reads only contexts that have reported. A required check that never started is not seen, so such a PR stays ready
- More than 100 contexts on a commit: only the first 100 are read
- Unverified until step 5: whether `isRequired` honours rulesets, whether a failed run followed by a successful rerun of the same check reads as passing, and what a private repository or App token returns
- Re-bucketing on upgrade rewrites the canvas once

### Neutral

- Draft PRs and unapproved PRs are never asked for check status, so their flag stays false
- The notification text counts the demoted PR under "waiting for author"

## Justification

### Why check status is its own request for approved PRs

The enrichment query runs for every open PR and a failure scoped to a repository fails the whole fetch. Adding `statusCheckRollup` there would put a token that cannot read it in the way of every run, and the research found no GitHub page documenting what such a token receives. A separate request limits the damage to a warning. It also costs nothing for PRs that cannot reach "ready to merge", and leaves `GetPRs` and the merged paths untouched.
