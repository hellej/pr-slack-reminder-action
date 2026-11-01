# Show merge blockers on PR rows

date: 2026-11-01
status: draft

Builds on [plan 019](019_demote-approved-PRs-with-failing-checks.md), which must be implemented first. Plan 019 adds `prview.MergeBlocker`, `MergeBlockers()` and the `❌ CI` group. This plan adds conflicts to them.

## Requirements

- An open row's `❌` group also names a merge conflict: `(✅ Dana Davis / 💬 Erin Evans / ❌ CI, conflicts)`
- `conflicts` shows on an open row (not draft, merged or collapsed) whose PR's `mergeable` is `CONFLICTING`, whatever its review state, after `CI` when both apply
- The rule holds in the message and on the canvas, in every open section
- No bucket changes: an unapproved conflicting PR stays in "waiting for review" and now says it conflicts
- No new action input, permission or GitHub request

Serves § **Purpose**: the row says what the author has to fix, and a reviewer sees a rebase is coming before reviewing. Fits § **Reference Deployment** (0 to 8 open PRs) and adds no API cost.

### Non-goals

- No marker for an outstanding changes request or a thread waiting for the author
- No re-review marker in "waiting for review"
- No marker on draft, merged or collapsed rows
- No change to the notification text

## Target shape

- `prview.MergeBlockerConflicts` (`"conflicts"`), a second `MergeBlocker`
- `prview.PR.MergeBlockers()` adds `MergeBlockerConflicts` when the PR's `Conflicting` field is true (`mergeable` is `CONFLICTING`), after `MergeBlockerFailingChecks`
- `GetReviewersTextSegments` needs no change. With no approvers or commenters it renders ` (❌ conflicts)`, a case an unreviewed conflicting PR now reaches
- `mergeable` is already fetched for every open PR, so the GraphQL cost does not change. A PR GitHub reports `UNKNOWN` reads as not conflicting. See docs/third-party-facts.md § `PullRequest.mergeable` returns `UNKNOWN` while GitHub computes it, and only REST documents the retry
- Action inputs: none new. Permissions: none new

Message rows, one per case, oldest first in each section:

```
*👀 Waiting for review*
  • Split the parser   1 day ago   by bob (💬 carol / ❌ conflicts)
  • Add retry budget   3 hours ago   by alice (❌ conflicts)

*💬 Waiting for author*
  • Drop old config   4 days ago   by dana (✅ erin / 💬 frank / ❌ CI)
  • Fix retry loop   2 days ago   by alice (✅ bob / 💬 carol / ❌ CI, conflicts)
  • Rename inputs   1 day ago   by erin (✅ dana / ❌ conflicts)
```

- An `update` edit and a stale mark show the same rows
- The canvas rows read the same, e.g. `**[Fix retry loop](url)** _2 days ago_ by alice (✅ bob / 💬 carol / ❌ CI, conflicts)`

## Breaking or non-breaking

Non-breaking, **minor**. Row text changes, with no input or permission change, as plan 019 did.

- No state change. The first run after upgrading re-renders the rows, and the canvas rewrites once wherever a row gains `conflicts`

## Summary

- **1** Add conflicts to the merge blockers, in the message and on the canvas
- **2** Check the rows live
- **3** Docs

## Steps

### 1. Add conflicts to the merge blockers, in the message and on the canvas

Touches `internal/prview`, `testhelpers/mockgithubclient`, `cmd/pr-slack-reminder` and `internal/canvasbuilder` snapshot tests.

- `MergeBlockerConflicts` and the `MergeBlockers()` change as in § Target shape. Size: one constant, one condition
- `mockgithubclient`: `MockGitHubClientOptions.ConflictingPRNumbers []int` answers those PRs' enrichment with `mergeable: CONFLICTING`. Other PRs keep today's answer
- Fixtures, then `make update-test-snapshots`. Mark existing PRs, so only their rows change:
  - `snapshotScenarios()` in `cmd/pr-slack-reminder/snapshot_test.go`: mark plan 019's failing-check PR and one existing unreviewed PR conflicting. That serves the post and marked-stale snapshots
  - The update-mode table of `TestSnapshotsUpdateMode`: mark plan 019's failing-check PR conflicting, for the edited message
  - The fixtures of `TestBuildMarkdownSnapshots` in `internal/canvasbuilder/canvasbuilder_test.go`: plan 019's failing-check PR and an unreviewed PR, both marked conflicting, for the canvas rows
  - Read each snapshot diff before re-recording: only those PRs' rows change
- Update `prview.spec.md`'s `MergeBlockers` line

### 2. Check the rows live

Touches no files.

- Make each PR conflict with a commit on its base branch, so its head commit keeps its statuses. A push to the head would get no check run: GitHub runs no `pull_request` workflow on a conflicting PR. See docs/third-party-facts.md § GitHub Actions runs no `pull_request` workflow while the PR has a merge conflict
- Setup, by the repository owner:
  - An unreviewed PR in `hellej/pr-slack-reminder-test-repo-2`, made conflicting. `-2` avoids the `alice` author filter of `-1`'s grouped runs. It carries no `ignore-reminder` label
  - Plan 019's fixture PR in this repository, made conflicting by a commit on `fixture/required-check-base`
- The branch's Build E2E grouped canvas runs ("Run with filters, canvas link and merge stats: post" and its update) show the unreviewed PR under "Waiting for review" with `(❌ conflicts)`, in the message and on the canvas
- `gh workflow run pr-reminder.yml --ref <implementation branch> -f run-mode=post -f build-first=true`, then the same with `-f run-mode=update`, show plan 019's fixture under "Waiting for author" with `❌ CI, conflicts`, in the dev channel's message and on its canvas
- Done means both PRs show as described, in the message and on the canvas

### 3. Docs

Touches `README.md`.

- The bucket paragraph under `README.md` § PR Tracker Canvas: an open row's `❌` group also says `conflicts` when the PR conflicts with its base branch, in any open section
- Done means `make check-style` passes

## Consequences

### Positive

- The author sees a conflict on the row, beside a failing check
- A reviewer sees a conflict before reviewing an unapproved PR
- No extra GitHub request and no state change

### Negative

- None

### Caveats

- Collapsed rows have no reviewer list, so a conflicting Dependabot PR in a collapsed row shows no `❌ conflicts`
- While GitHub is still computing `mergeable` after a push, the PR reads as not conflicting, and its `❌ conflicts` can vanish until the next run

### Neutral

- An approved PR held back only by a thread waiting for the PR's author shows no `❌` group
