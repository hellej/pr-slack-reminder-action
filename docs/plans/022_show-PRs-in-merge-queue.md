# Show PRs in merge queue

date: 2026-11-21
status: draft

## Requirements

- Problem: in a repository with GitHub's merge queue, a queued PR shows under "Ready to merge", so the team reads it as waiting for someone to press merge
- Goals:
  - A new open section, "In merge queue", in the message and the PR tracker canvas, holding the PRs in a merge queue right now
  - The README's update mode example also runs on a PR entering or leaving a queue
- Non-goals:
  - Telling that the queue removed a PR. A removed PR shows in the section its own state gives it, with its merge blockers, such as conflicts or a failing required check on its head commit. A failed merge group check never reaches the PR's head commit, so the removed PR shows no blocker for it. See docs/third-party-facts.md § Merge queue checks run on a temporary `gh-readonly-queue/<base branch>` branch with its own SHA, apart from the PR head that `statusCheckRollup` reads
  - A queue position or entry state on the row
  - `merge_group` as a trigger
  - A live run against a queued PR. No E2E repository can have a merge queue: see docs/third-party-facts.md § Merge queue is available only in organization-owned repositories: public ones, or private ones on GitHub Enterprise Cloud
- Hard constraint: a repository without a merge queue sees no change: the same message, canvas, state and run page as before
- Work lands on the `plan/merge-queue-section` feature branch, through a PR
- Purpose (AGENTS.md § Purpose): a queued PR stops asking the team for a merge already under way

## Target Shape

- New field `githubclient.PR.InMergeQueue`, from `PullRequest.isInMergeQueue`, tagged `json:",omitempty"`. See docs/third-party-facts.md § `PullRequest.isInMergeQueue` is a non-null `Boolean!`, and GitHub CLI selects it on every `gh pr merge` whatever the repository
  - Read by the enrichment query (`enrichedPullRequestSelection`), beside `mergeable`, so every enriched open PR gets it, approved or not
  - A PR left without reviewer info by a failed enrichment reads false, like its other merge flags
  - Merged PRs read false, so the merged PR cache stays byte for byte the same
- New next action, `prview.NextActionInMergeQueue`, checked first in `GetNextAction`: a queued PR is in the merge queue, whatever else is true of it
- A queued PR's row shows its reviewers and `MergeBlockers()` like any open row
- New section, "In merge queue", after "Ready to merge" in both views:
  - Message: `🚛 In merge queue`. The message's sections then read `👀 Waiting for review`, `💬 Waiting for author`, `✅ Ready to merge`, `🚛 In merge queue`, `🚀 Recently merged`. Empty renders nothing
  - Canvas: `## 🚛 In merge queue`, between `## ✅ Ready to merge` and `## 🚀 Merged`. Hides when empty, like the other open sections. A queued PR keeps the all-empty `## Open` heading and its `_No open PRs_` line from showing
  - Rows render with the open PR row renderer
- Summary text gains a last part, `in merge queue`: `"1 PR to review, 2 to merge, 1 in merge queue 👀"`, `"1 PR in merge queue 👀"`
- No change to action inputs or state keys
- The README's permission list stays the same. Unverified: whether a token with those permissions can read `isInMergeQueue`. See § Caveats

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

Marked stale by the next post: the staleness warning, then the blocks as last sent, with the `🚛 In merge queue` section if the last message had one.

## Breaking Change

- Non-breaking: a minor release. The new section and summary part show only where a queue holds a PR, and no input or state key changes
- First run after upgrading: a repository without a queue renders the same message and canvas, so the canvas hash matches and the canvas write is skipped

## Summary of Steps

- 1 (checkpoint): Read `isInMergeQueue` in the enrichment query
- 2: `NextActionInMergeQueue` in `prview`
- 3: "In merge queue" section in the message
- 4: "In merge queue" section in the canvas
- 5: README

## Steps

### 1: Read `isInMergeQueue` in the enrichment query (checkpoint)

- `internal/apiclients/githubclient/`: add `isInMergeQueue` to `enrichedPullRequestSelection` and an `IsInMergeQueue` field to `pullRequestNode`, and set `PR.InMergeQueue` from it in `prWithReviewers`
  - `enrichPRsWithReviewInfo` runs that selection for `FindOpenPRs`, `FindRecentlyMergedPRs` and `FindPRsMergedSince`
  - `fullPullRequestSelection` stays as it is, so `GetPRs` PRs read `InMergeQueue` false. They are only read for their merged state
- `testhelpers/mockgithubclient/`: the enrichment route answers `isInMergeQueue` from a new fixture option listing the queued PR numbers, all others false
- Tests pinning the enrichment query text change with the selection
- `githubclient.spec.md`: Behaviour, beside the merge flags, and that `GetPRs` PRs read it false, as they read `RequiredChecksUnknown`
- Checkpoint before step 2, passed in Build E2E run 44934692097:
  - The job log shows the enrichment of the private `hellej/pr-slack-reminder-test-repo-1` and `-2`, and the public `-3`, with no field error. The user confirmed the job's App token lacks the "Merge queues" permission
  - The enrichment request's `rateLimit.cost` equals the main-code run 44914969961's at the same PR counts: 1 PR 1, 19 PRs 20, 20 PRs 21, 25 PRs 27
  - A field error would have stopped the loop and sent the target shape back to the user
  - The result is recorded in the `isInMergeQueue` facts entry, replacing its unverified permission line for the App token

### 2: Next action in `prview`

- `internal/prview/prview.go`: `NextActionInMergeQueue` first in `GetNextAction`
- Table cases in the existing `GetNextAction` test: queued over conflicting, over failing checks, over a waiting thread and over no approval
- `prview.spec.md`: the next action, and an Oddity: a PR the queue removed, such as for a failing merge group check, goes back to the section its own state gives it, often "Ready to merge", since the merge group's checks never reach its head commit. It moves on once its own checks or reviews change

### 3: "In merge queue" section in the message

- `internal/messagecontent/`: `Content.InMergeQueue`, filled and grouped like `ReadyToMerge`
  - `HasPRs` counts it
  - The summary text's last part, as in § Target Shape
  - The "Putting …" log line names its PR count
- `internal/messagebuilder/`: the `🚛 In merge queue` section, between `ReadyToMerge` and `Merged`
- `cmd/pr-slack-reminder/`: new snapshot scenarios: a post with a queued PR beside one ready to merge, and a post and an update whose only open PR is queued. The last two show a queued PR alone still posts or edits the message, since `HasPRs` counts it. Every existing snapshot stays as recorded: that is the no-queue check
- `messagecontent.spec.md`: the section and the summary part
- `messagebuilder.spec.md`: the section order, and both block cap Oddities, the ungrouped and the grouped one. Adding a section adds blocks:
  - Ungrouped, a section is a heading and one list block. Five sections, the merge stats line and the footer make at most 12 blocks, not 10
  - Grouped, a section is 1 + 2 × repositories blocks. With all five sections, 4 repositories fit (5 × 9 = 45 content blocks), not 5 (5 × 11 = 55)

### 4: "In merge queue" section in the canvas

- `internal/canvascontent/`: `Content.InMergeQueue`, bucketed like the other open buckets, and its PR count in the log line with theirs
- `internal/canvasbuilder/`: `## 🚛 In merge queue` after `## ✅ Ready to merge`, hiding while empty, and part of the all-empty `## Open` check
- A new golden file with a queued PR. Existing golden files stay as recorded
- Specs: `canvascontent.spec.md`, `canvasbuilder.spec.md`

### 5: README

- The intro's status list gains "in merge queue"
- The update mode example's `pull_request` types gain `enqueued` and `dequeued`. See docs/third-party-facts.md § `pull_request` activity types `enqueued` and `dequeued` are not defaults, and GitHub publishes `dequeued` for every removal, a merge by the queue included
- This repository's `.github/workflows/pr-reminder.yml` stays as it is: no repository the workflow lists can have a queue
- Done when: `make check-style` passes

## Consequences

### Positive

- A queued PR stops asking the team to merge it

### Negative

- None

### Caveats

- A token that cannot read `isInMergeQueue` gets a field error on every enriched PR, open or merged. Each such PR loses its reviewer info, so:
  - open PRs also lose their snooze and their check status fetch
  - merged rows lose their reviewers
  - the merge stats fetch warns once per merged PR
- Build E2E run 44934692097 read the field with the App token, which lacks "Merge queues", on private repositories with no field error. `GITHUB_TOKEN` on a private repository is unverified
- No live run against a queued PR: unit and snapshot tests cover the queued row instead
- Unverified: whether a `dequeued` sent by the queue starts an Actions run. If it does not, a queue merge still shows through `closed`, and a removal shows only once another event starts a run
- Each queue merge sends `dequeued` beside `closed`, and each enqueue sends `enqueued`: up to two more update runs per merged PR. Within AGENTS.md § GitHub API Budget at the PR counts of AGENTS.md § Reference Deployment

### Neutral

- None
