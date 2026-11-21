# Show PRs in merge queue

date: 2026-11-21
status: draft

## Requirements

- Problem: in a repository with GitHub's merge queue, a queued PR shows under "Ready to merge", so the team reads it as waiting for someone to press merge
- Goals:
  - A new open section, "In merge queue", in the message and the PR tracker canvas, holding the PRs in a merge queue right now
  - The README's update mode example also runs on a PR entering or leaving a queue
- Non-goals:
  - Telling that the queue removed a PR. A removed PR shows in the section its own state gives it, with its merge blockers, such as conflicts or a failing required check on its head commit. A failed merge group check never reaches that commit, so it shows no blocker. See docs/third-party-facts.md § Merge queue checks run on a temporary `gh-readonly-queue/<base branch>` branch with its own SHA, apart from the PR head that `statusCheckRollup` reads
  - A queue position or entry state on the row
  - `merge_group` as a trigger
  - A live check of a queued PR. No E2E repository can have a merge queue: see docs/third-party-facts.md § Merge queue is available only in organization-owned repositories: public ones, or private ones on GitHub Enterprise Cloud
- Hard constraint: a repository without a merge queue sees no change: the same message, canvas, state and run page as before
- Work lands on the `plan/merge-queue-section` feature branch, through a PR
- Purpose: a queued PR stops asking the team for a merge already under way. In the § Reference Deployment, with 0 to 8 open PRs, that is one fewer false ask per queued PR

## Target Shape

- New field `githubclient.PR.InMergeQueue`, from `PullRequest.isInMergeQueue`, tagged `json:",omitempty"`. See docs/third-party-facts.md § `PullRequest.isInMergeQueue` is a non-null `Boolean!`, and GitHub CLI selects it on every `gh pr merge` whatever the repository
  - Read by the enrichment query (`enrichedPullRequestSelection`), beside `mergeable`, so every enriched open PR gets it, approved or not
  - A PR left without reviewer info by a failed enrichment reads false, like its other merge flags
  - Merged PRs read false, so the merged PR cache stays byte for byte the same
- New next action, `prview.NextActionInMergeQueue`, checked first in `GetNextAction`: a queued PR is in the merge queue whatever else holds
- A queued PR's row shows its reviewers and `MergeBlockers()` like any open row
- New section, "In merge queue", after "Ready to merge" in both views:
  - Message: `🚛 In merge queue`, between `✅ Ready to merge` and `🚀 Recently merged`. Empty renders nothing
  - Canvas: `## 🚛 In merge queue`, between `## ✅ Ready to merge` and `## 🚀 Merged`. Hides when empty, like the other open sections, and counts toward the all-empty `## Open` case
  - Rows render with the open PR row renderer
- Summary text gains a last part, `in merge queue`: `"1 PR to review, 2 to merge, 1 in merge queue 👀"`, `"1 PR in merge queue 👀"`
- No change to action inputs, required token permissions or state keys

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

- 1 (checkpoint): Read `isInMergeQueue` in the enrichment query
- 2: `NextActionInMergeQueue` in `prview`
- 3: "In merge queue" section in the message
- 4: "In merge queue" section in the canvas
- 5: README

## Steps

### 1: Read `isInMergeQueue` in the enrichment query (checkpoint)

- `internal/apiclients/githubclient/`: `isInMergeQueue` in `enrichedPullRequestSelection`, the node field, and `PR.InMergeQueue` set in `prWithReviewers`
  - `enrichPRsWithReviewInfo` runs that selection for `FindOpenPRs`, `FindRecentlyMergedPRs` and `FindPRsMergedSince`
  - `fullPullRequestSelection` stays as it is, so `GetPRs` PRs read `InMergeQueue` false. They are only read for their merged state
- `testhelpers/mockgithubclient/`: the enrichment route answers `isInMergeQueue` from a new fixture option listing the queued PR numbers, all others false
- Tests pinning the enrichment query text change with it
- `githubclient.spec.md`: Behaviour, beside the merge flags, and that `GetPRs` PRs read it false, as they read `RequiredChecksUnknown`
- Verify done, before step 2:
  - The Build E2E job log of this branch's PR shows the enrichment of the private `hellej/pr-slack-reminder-test-repo-1` and `-2`, and the public `-3`, with no field error. The job's App token lacks the "Merge queues" permission
  - `rateLimit.cost` of the enrichment request against the last run on `main`, stated in the PR
  - A field error stops the loop: the target shape goes back to the user
  - Record the result in the `isInMergeQueue` facts entry, replacing its unverified permission line for the App token

### 2: Next action in `prview`

- `internal/prview/prview.go`: `NextActionInMergeQueue` first in `GetNextAction`
- Table cases in the existing `GetNextAction` test: queued over conflicting, over failing checks, over a waiting thread and over no approval
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

- A token that cannot read `isInMergeQueue` gets a field error on every enriched PR, open or merged. Each such PR loses its reviewer info, so:
  - open PRs also lose their snooze and their check status fetch
  - merged rows lose their reviewers
  - the merge stats fetch warns once per merged PR
- The Build E2E covers that for the App token on private repositories. `GITHUB_TOKEN` on a private repository is unverified
- No live check of a queued PR: unit and snapshot tests carry that path
- Unverified: that a queue-initiated `dequeued` starts an Actions run. Without it, a queue merge still shows through `closed`, and a removal at the next run another event starts
- `dequeued` also fires on each queue merge, beside `closed`: one more update run per merged PR, and one per enqueue. Within § GitHub API Budget at the § Reference Deployment's PR counts

### Neutral

- None
