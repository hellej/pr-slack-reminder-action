# Show PRs in merge queue

date: 2026-11-21
status: draft

## Requirements

- Problem: in a repository with GitHub's merge queue, a queued PR shows under "Ready to merge", so the team reads it as waiting for someone to press merge
- Goals:
  - A new open section, "In merge queue", in the message and the PR tracker canvas, holding the PRs in a merge queue right now
  - The README's update mode example also runs on a PR entering or leaving a queue
- Non-goals:
  - Telling that the queue removed a PR. A removed PR shows by its review and check state, as today: usually "Ready to merge" again, since merge group checks never reach the PR's own rollup. See docs/third-party-facts.md § Merge queue checks run on a temporary `gh-readonly-queue/<base branch>` branch with its own SHA, apart from the PR head that `statusCheckRollup` reads
  - A queue position or entry state on the row
  - `merge_group` as a trigger
  - A live check of a queued PR. No E2E repository can have a merge queue: see docs/third-party-facts.md § Merge queue is available only in organization-owned repositories: public ones, or private ones on GitHub Enterprise Cloud
- Hard constraint: a repository without a merge queue sees no change: the same message, canvas, state and run page as before
- Work lands on the `plan/merge-queue-section` feature branch, through a PR
- Purpose: a queued PR stops asking the team for a merge already under way. In the § Reference Deployment, with 0 to 8 open PRs, that is one fewer false ask per queued PR

## Target Shape

- New field `githubclient.PR.InMergeQueue`, from `PullRequest.isInMergeQueue`, tagged `json:",omitempty"`. False when not fetched or when the fetch failed. See docs/third-party-facts.md § `PullRequest.isInMergeQueue` is a non-null `Boolean!`, and GitHub CLI selects it on every `gh pr merge` whatever the repository
- New fetch phase, the merge queue fetch: a fourth phase of `FindOpenPRs`, after the required check status fetch, over the PRs that fetch reads (approved, not draft, complete reviewer info, not snoozed). It selects `isInMergeQueue` alone
  - A request of its own, so a field error on `isInMergeQueue` leaves reviewer info and check status as they are today. See [§ A request of its own](#a-request-of-its-own)
  - Fails like the check status fetch: never fails `FindOpenPRs`, leaves the PRs it failed for out of the queue, and joins its errors into the new `OpenPRsResult.MergeQueueErr`
  - The run warns `Failed to fetch merge queue status: <error>`
- New next action, `prview.NextActionInMergeQueue`, checked first in `GetNextAction`: a queued PR is in the merge queue whatever else holds
- A queued PR's row shows its reviewers and `MergeBlockers()` like any open row
- New section, "In merge queue", after "Ready to merge" in both views:
  - Message: `🚛 In merge queue`, between `✅ Ready to merge` and `🚀 Recently merged`. Empty renders nothing
  - Canvas: `## 🚛 In merge queue`, between `## ✅ Ready to merge` and `## 🚀 Merged`. Hides when empty, like the other open sections, and counts toward the all-empty `## Open` case
  - Rows render with the open PR row renderer
- Summary text gains a last part, `in merge queue`: `"1 PR to review, 2 to merge, 1 in merge queue 👀"`, `"1 PR in merge queue 👀"`
- No change to action inputs, required token permissions or state keys. The merged PR cache stays byte for byte the same: only open PRs get `InMergeQueue` set

Message sketches, ungrouped. Posted by the 09:00 post, while PR #41 is queued:

```
👀 Waiting for review
• Fix login redirect  3 days ago  @alice (💬 bob)
✅ Ready to merge
• Add dark mode  5 hours ago  @erin (✅ frank)
🚛 In merge queue
• Cache API tokens  2 days ago  @gina (✅ hank)
🚀 Recently merged
• Tidy README  merged 1 hour ago  @ivan (✅ judy)
Summary: "1 PR to review, 1 to merge, 1 in merge queue 👀"
```

Edited by `update` on `dequeued`, after the queue merged #41:

```
👀 Waiting for review
• Fix login redirect  3 days ago  @alice (💬 bob)
✅ Ready to merge
• Add dark mode  5 hours ago  @erin (✅ frank)
🚀 Recently merged
• Cache API tokens  merged 1 minute ago  @gina (✅ hank)
• Tidy README  merged 1 hour ago  @ivan (✅ judy)
Live, updated 10:42
Summary: "1 PR to review, 1 to merge 👀"
```

Marked stale by the next post: the staleness warning, then the blocks as last sent, the `🚛 In merge queue` section included when it was there.

## Breaking Change

- Non-breaking, minor: a new section and summary part only where a queue holds a PR, and no input, permission or state change
- First run after upgrading: a repository without a queue renders the same message and canvas, so the canvas hash matches and the canvas write is skipped

## Summary of Steps

- R1: Extract the approved PR batch fetch from the required check status fetch
- 1 (checkpoint): Merge queue fetch in `githubclient`
- 2: `NextActionInMergeQueue` in `prview`
- 3: "In merge queue" section in the message
- 4: "In merge queue" section in the canvas
- 5: README

## Steps

### R1: Extract the approved PR batch fetch

- `internal/apiclients/githubclient/`, `checkstatusfetch.go`: split `fetchRequiredCheckStatus` into a batch fetch both phases share (pick the PRs, batch them by `enrichBatchSize`, run the batches under `defaultGitHubAPIConcurrencyLimit`, keep per batch and per PR errors apart, join them) and the check status part (selection, classify, log)
- Rename `needsRequiredCheckStatus` to `isApprovedForMerge`: it picks the PRs for both phases
- No behaviour change. `checkstatusfetch_test.go` already pins the query text, per PR errors and failed batches

### 1: Merge queue fetch (checkpoint)

- `internal/apiclients/githubclient/`: the `PR.InMergeQueue` field; a new `mergequeuefetch.go`, the phase from § Target Shape on R1's batch fetch, run in `FindOpenPRs` after the check status fetch, logging a line per PR as the check status phase does; `OpenPRsResult.MergeQueueErr`
- `cmd/pr-slack-reminder/run.go`: warn on `MergeQueueErr`, as on `CheckStatusErr`
- `testhelpers/mockgithubclient/`: `GraphQLTransport.Post` routes a query selecting `isInMergeQueue` before the `pullRequest(number:` route, records it in a new `FetchRecording.MergeQueueRequests`, and answers from a new fixture option listing the queued PR numbers, all others false. Without its own route the query would land in the enrichment route and its recording
- `AGENTS.md` § GitHub API Budget: the merge queue fetch adds one more request per 25 approved PRs
- Specs: `githubclient.spec.md` (Behaviour; Doesn't Do: an unapproved PR is never read), `run.spec.md` (the warning)
- Verify done, before step 2:
  - The Build E2E job log of this branch's PR: a merge queue line for each approved PR in the private `hellej/pr-slack-reminder-test-repo-1` and `-2`, and in the public `-3`, reading false, with no error. That job's App token lacks the "Merge queues" permission
  - A probe job on a branch of the private `hellej/pr-slack-reminder-test-repo-1`, set up like runs 41299479257 and 41299564373, with `permissions: pull-requests: read` and nothing else, querying `isInMergeQueue` on PRs 1 and 2 with `GITHUB_TOKEN`: no error. Delete the probe branch after
  - `rateLimit.cost` of the new request, and the run's total against the last run on `main`, stated in the PR
  - A field error in either check stops the loop: the target shape goes back to the user
  - Record what both checks showed in the `isInMergeQueue` facts entry, replacing its unverified permission line

### 2: Next action in `prview`

- `internal/prview/prview.go`: `NextActionInMergeQueue` first in `GetNextAction`
- Table cases in the existing `GetNextAction` test: queued over conflicting, over failing checks and over a waiting thread
- `prview.spec.md`: the next action

### 3: "In merge queue" section in the message

- `internal/messagecontent/`: `Content.InMergeQueue`, filled and grouped like `ReadyToMerge`; `HasPRs` counts it; the summary part
- `internal/messagebuilder/`: the `🚛 In merge queue` section, between `ReadyToMerge` and `Merged`
- `cmd/pr-slack-reminder/`: a new snapshot scenario with a queued PR, post and update. Every existing snapshot stays as recorded: that is the no-queue check
- Specs: `messagecontent.spec.md`; `messagebuilder.spec.md`, both block cap Oddities: ungrouped, the layout spends at most 12 blocks, not 10; grouped, with all five sections, 4 repositories fit (45 content blocks), not 5

### 4: "In merge queue" section in the canvas

- `internal/canvascontent/`: `Content.InMergeQueue`, bucketed and logged like the other open buckets
- `internal/canvasbuilder/`: `## 🚛 In merge queue` after `## ✅ Ready to merge`, hiding while empty, and part of the all-empty `## Open` check
- A new golden file with a queued PR. Existing golden files stay as recorded
- Specs: `canvascontent.spec.md`, `canvasbuilder.spec.md`

### 5: README

- The intro's status list gains "in merge queue"
- The update mode example's `pull_request` types gain `enqueued` and `dequeued`. See docs/third-party-facts.md § `pull_request` activity types `enqueued` and `dequeued` are not defaults, and GitHub publishes `dequeued` for every removal, a merge by the queue included
- This repository's `.github/workflows/pr-reminder.yml` stays as it is: no repository it lists can have a queue
- Verify done: `make check-style` passes

## Consequences

### Positive

- A queued PR stops asking the team to merge it

### Negative

- None

### Caveats

- A PR the queue removed shows under "Ready to merge" again, as today
- A queued PR without an approval is never read, so in a repository that queues without requiring a review it shows by its review state
- No live check of a queued PR: unit and snapshot tests carry that path
- Unverified: that a queue-initiated `dequeued` starts an Actions run. Without it, a queue merge still shows through `closed`, and a removal at the next run another event starts
- `dequeued` also fires on each queue merge, beside `closed`: one more update run per merged PR, and one per enqueue. Within § GitHub API Budget at the § Reference Deployment's PR counts
- One more GraphQL request per run with approved PRs, and per 25 of them

### Neutral

- None

## Justification

### A request of its own

- Field permissions differ per token: an App token without `checks: read` gets `FORBIDDEN` on `statusCheckRollup` in a private repository (docs/third-party-facts.md § `PullRequest.statusCheckRollup` needs `checks: read` in a private repository: an App token without it gets `FORBIDDEN` on the field)
- A field error fails its PR in that query: in the enrichment query it drops the PR's reviewer info, in the check status query its check status. A request of its own makes a failure cost only the new section
