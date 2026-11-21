# Show PRs in merge queue

date: 2026-11-21
status: draft

## Requirements

- Problem: in a repository with GitHub's merge queue, a queued PR shows under "Ready to merge", so the team reads it as waiting for someone to press merge
- Problem: a PR the queue removed, such as for a failing merge group check, still has passing checks on its own head commit, so it lands back under "Ready to merge" with nothing saying it needs its author
- Goals:
  - A new open section, "In merge queue", in the message and the PR tracker canvas, holding the PRs in a merge queue right now
  - A PR removed from the queue with no push since the removal shows under "Waiting for author", with a "removed from queue" merge blocker on its row
  - The README's update mode example also runs on a PR entering or leaving a queue
- Non-goals:
  - A queue position or entry state on the row
  - `merge_group` as a trigger
  - Telling removal reasons apart: every removal counts the same
  - A live E2E check of a queued or removed PR. No E2E repository can have a merge queue
- Hard constraint: a repository without a merge queue sees no change at all: the same message, canvas, state and run page as before. A token that cannot read the new fields loses nothing it has today
- Work lands on the `plan/merge-queue-section` feature branch, through a PR
- Purpose: a queued PR no longer asks the team for a merge that is already under way, and a dropped one asks its author instead of nobody. In the § Reference Deployment, with 0 to 8 open PRs, that is one fewer false ask per queued PR

## Target Shape

- New concept, `MergeQueueStatus` on `githubclient.PR`, with three values:
  - `MergeQueueStatusUnknown` (`""`): not fetched, the fetch failed, or neither of the two below
  - `MergeQueueStatusQueued` (`"queued"`): `isInMergeQueue` is true
  - `MergeQueueStatusRemoved` (`"removed"`): not queued, and the PR's newest `RemovedFromMergeQueueEvent` is later than both its head commit's `committedDate` and its newest `HeadRefForcePushedEvent`
  - "No push since the removal" reads timestamps, since GitHub records no push time for a plain push and `beforeCommit` points at the merge group commit. See docs/third-party-facts.md § A PR's timeline orders commits by branch order, not push time, and `Commit.pushedDate` is gone, and § `RemovedFromMergeQueueEvent.beforeCommit` is undocumented, and live reports point it at the merge group commit, not the PR head
- New fetch phase, the merge queue status fetch: a fourth phase of `FindOpenPRs`, after the required check status fetch, over the same PRs that fetch reads (approved, not draft, complete reviewer info, not snoozed)
  - Selection per PR: `isInMergeQueue`, `commits(last: 1){ nodes { commit { committedDate } } }`, and two aliased `timelineItems(..., last: 1)` reads of `REMOVED_FROM_MERGE_QUEUE_EVENT` and `HEAD_REF_FORCE_PUSHED_EVENT`, each selecting `createdAt`. See docs/third-party-facts.md § `PullRequest.isInMergeQueue` is a non-null `Boolean!`, and GitHub CLI selects it on every `gh pr merge` whatever the repository
  - Its own phase, not more fields on an existing query, so a field error on the new fields leaves reviewer info and check status as they are today. See [§ A phase of its own](#a-phase-of-its-own)
  - Fails like the check status fetch: never fails `FindOpenPRs`, leaves the PRs it failed for `MergeQueueStatusUnknown`, and joins its errors into the new `OpenPRsResult.MergeQueueStatusErr`
  - The run warns `Failed to fetch merge queue status: <error>`
- New next action, `prview.NextActionInMergeQueue`, checked first in `GetNextAction`: a queued PR is in the merge queue whatever else holds
- New merge blocker, `prview.MergeBlockerRemovedFromQueue` (`"removed from queue"`), listed after `MergeBlockerConflicts`. A removed PR is `NextActionWaitingForAuthor`, approved or not
- New section, "In merge queue", after "Ready to merge" in both views:
  - Message: `🚂 In merge queue`, between `✅ Ready to merge` and `🚀 Recently merged`. Empty renders nothing
  - Canvas: `## 🚂 In merge queue`, between `## ✅ Ready to merge` and `## 🚀 Merged`. Hides when empty, like the other open sections, and counts toward the all-empty `## Open` case
  - Rows render with the open PR row renderer
- Summary text gains a last part, `in merge queue`: `"1 PR to review, 2 to merge, 1 in merge queue 👀"`, `"1 PR in merge queue 👀"`
- No change to action inputs, to required token permissions, or to persisted state. Existing state keys and the merged PR cache stay byte for byte the same: the new PR field is `omitempty` and only open PRs get it set

Message sketches, ungrouped. Posted by the 09:00 post, while PR #41 is queued:

```
👀 Waiting for review
• Fix login redirect  3 days ago  @alice (💬 bob)
✅ Ready to merge
• Add dark mode  5 hours ago  @erin (✅ frank)
🚂 In merge queue
• Cache API tokens  2 days ago  @gina (✅ hank)
🚀 Recently merged
• Tidy README  merged 1 hour ago  @ivan (✅ judy)
Summary: "1 PR to review, 1 to merge, 1 in merge queue 👀"
```

Edited by `update` on `dequeued`, after the queue dropped #41 for a failing check:

```
👀 Waiting for review
• Fix login redirect  3 days ago  @alice (💬 bob)
💬 Waiting for author
• Cache API tokens  2 days ago  @gina (✅ hank / removed from queue)
✅ Ready to merge
• Add dark mode  5 hours ago  @erin (✅ frank)
🚀 Recently merged
• Tidy README  merged 1 hour ago  @ivan (✅ judy)
Live, updated 10:42
Summary: "1 PR to review, 1 waiting for author, 1 to merge 👀"
```

Marked stale by the next post: the staleness warning, then the blocks as last sent, the `🚂 In merge queue` section included when it was there.

## Breaking Change

- Non-breaking, minor: a new section and summary part only where a queue is in use, and no input, permission or state change
- First run after upgrading: reads the state the previous version wrote as before. A repository without a queue renders the same message and canvas, so the canvas hash matches and the canvas write is skipped

## Summary of Steps

- R1: Extract the approved PR batch fetch from the required check status fetch
- 1 (checkpoint): Merge queue status fetch in `githubclient`
- 2: `NextActionInMergeQueue` and `MergeBlockerRemovedFromQueue` in `prview`
- 3: "In merge queue" section in the message
- 4: "In merge queue" section in the canvas
- 5: README

## Steps

### R1: Extract the approved PR batch fetch

- `internal/apiclients/githubclient/`, `checkstatusfetch.go`: split `fetchRequiredCheckStatus` into a generic part (pick the PRs, batch them by `enrichBatchSize`, run the batches under `defaultGitHubAPIConcurrencyLimit`, keep per batch and per PR errors apart, join them) and the check status part (selection, classify, log)
- No behaviour change. The existing check status tests are the regression net, and `checkstatusfetch_test.go` already pins the query text, per PR errors and failed batches

### 1: Merge queue status fetch (checkpoint)

- `internal/apiclients/githubclient/`: new `MergeQueueStatus` type and the `PR.MergeQueueStatus` field, tagged `json:",omitempty"`
- New `mergequeuestatusfetch.go`, the phase from § Target Shape on R1's batch fetch, run in `FindOpenPRs` after the check status fetch
  - Logs a status line per PR, as the check status phase does
  - New `OpenPRsResult.MergeQueueStatusErr`
- `cmd/pr-slack-reminder/run.go`: warn on `MergeQueueStatusErr`, as for `CheckStatusErr`. No permission hint: no permission is known to help
- `testhelpers/mockgithubclient/`: carry `MergeQueueStatusErr`
- Specs: `githubclient.spec.md` (Behaviour, Doesn't Do: an unapproved PR is never read; Oddities: the `committedDate` reading below), `run.spec.md` (the warning)
- Oddity to record: a commit made before the removal but pushed after it, without a force push, reads as no push, so the PR stays removed until the next push or queue entry
- Verify done:
  - The Build E2E job log of this branch's PR: the merge queue status phase's line for each approved PR in the private `hellej/pr-slack-reminder-test-repo-1` and `-2`, and in the public `-3`, no field error, and status unknown. Its App token lacks the "Merge queues" permission
  - `rateLimit.cost` of the new request, and the run's total against the last run on `main`, stated in the PR
  - `gh workflow run pr-reminder.yml --ref plan/merge-queue-section -f run-mode=post -f build-first=true` reads the same with `GITHUB_TOKEN`, on this public repository
  - A field error in either check stops the loop: the target shape goes back to the user
- Add the private repository `GITHUB_TOKEN` gap to the `isInMergeQueue` facts entry, or confirm it, with what the checks showed

### 2: Next action and merge blocker in `prview`

- `internal/prview/prview.go`: `NextActionInMergeQueue` first in `GetNextAction`; `MergeQueueStatusRemoved` keeps a PR out of `NextActionReadyToMerge` and puts it in `NextActionWaitingForAuthor`; `MergeBlockerRemovedFromQueue` last in `MergeBlockers()`
- Table cases in the existing `GetNextAction` and `MergeBlockers` tests: queued over conflicting, over failing checks and over a waiting thread; removed with and without an approver; removed with conflicts lists both blockers in order
- `prview.spec.md`: the next actions and blockers

### 3: "In merge queue" section in the message

- `internal/messagecontent/`: `Content.InMergeQueue`, filled and grouped like `ReadyToMerge`; `HasPRs` counts it; the summary part
- `internal/messagebuilder/`: the `🚂 In merge queue` section, between `ReadyToMerge` and `Merged`
- `cmd/pr-slack-reminder/`: a new snapshot scenario with a queued PR and a removed PR, post and update. Every existing snapshot stays as recorded: that is the no-queue check
- Specs: `messagecontent.spec.md`; `messagebuilder.spec.md`, including the grouped block cap Oddity: with all five sections, 4 repositories fit (45 content blocks), not 5

### 4: "In merge queue" section in the canvas

- `internal/canvascontent/`: `Content.InMergeQueue`, bucketed and logged like the other open buckets
- `internal/canvasbuilder/`: `## 🚂 In merge queue` after `## ✅ Ready to merge`, hiding while empty, and part of the all-empty `## Open` check
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
- A PR the queue dropped asks its author, with the reason on its row

### Negative

- None

### Caveats

- A queued PR without an approval is never read, so in a repository that queues without requiring a review it shows by its review state
- No live check of a queued or removed PR: unit and snapshot tests carry those paths
- Unverified: that a queue-initiated `dequeued` starts an Actions run. Without it, the removal shows at the next run another event starts
- `dequeued` also fires on each queue merge, beside `closed`: one more update run per merged PR, and one per enqueue. Within § GitHub API Budget at the § Reference Deployment's PR counts
- One more GraphQL request per run with approved PRs, and per 25 of them

### Neutral

- Removal reasons are not shown or told apart

## Justification

### A phase of its own

- Field permissions differ per token: an App token without `checks: read` gets `FORBIDDEN` on `statusCheckRollup` in a private repository (docs/third-party-facts.md § `PullRequest.statusCheckRollup` needs `checks: read` in a private repository: an App token without it gets `FORBIDDEN` on the field)
- A field error in a query fails its PR there: in the enrichment query it drops the PR's reviewer info, in the check status query its check status. Whether every token reads the new fields is unverified, so they get a request whose failure only costs the new feature
