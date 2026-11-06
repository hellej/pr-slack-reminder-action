# Mark PRs whose approval went stale

date: 2026-11-01
status: draft

Lands on the feature branch `plan-020-stale-approvals`, through a PR. Builds on plans [019](019_demote-approved-PRs-with-failing-checks.md) and [020](020_show-merge-blockers-on-PR-rows.md), both implemented, whose merge blockers, `CI failing` and `conflicts`, stay last in the reviewer list.

## Requirements

- A row in "waiting for review", or a merged row, names each approver whose approval a push dismissed in a `🔁` group of its reviewer list, in the message and on the canvas: `(🔁 Dana Davis / 💬 Erin Evans)`
- A row in "waiting for author" or "ready to merge" lists them under `💬` instead: the next move is not their re-stamp. Examples: a PR with a changes request, or with a thread waiting for its author
- A push dismisses an approval under the "Dismiss stale pull request approvals when new commits are pushed" rule: in branch protection, or `dismiss_stale_reviews_on_push` in a ruleset
- An approver who approves again is a current approver: `✅`, not `🔁`
- A manually dismissed approval is not stale: its reviewer stays under `💬`, as today
- No bucket change: with no other approval, such a PR is in "waiting for review" today and stays there
- The notification text doesn't change
- No new action input, permission or GitHub request

Serves AGENTS.md § Purpose: a PR whose approval a push dismissed looks untouched today, and `🔁` turns it into a quick re-stamp from a named reviewer. Fits AGENTS.md § Reference Deployment, where a weekly Dependabot batch rebases ~5 approved PRs at once.

### Non-goals

- The "Require approval of the most recent reviewable push" rule. Its approvals stay `APPROVED`, and telling which ones went stale needs the review's commit, which GraphQL forbids on a private repository under `pull-requests: read`. See docs/third-party-facts.md § On a private repository, `pull-requests: read` alone makes a review's `commit`, `baseRef` and a `PullRequestCommit` timeline node `FORBIDDEN`, while REST `pulls/{n}/reviews` returns `commit_id`
- No bucket change for a PR with a merge conflict (`mergeable` is `CONFLICTING`) whose only approvals were push-dismissed: it stays in "waiting for review", its row reading `(🔁 Dana Davis / conflicts)`
- No `🔁` on draft or collapsed rows
- No new section, sort order or Slack mention

## Target shape

- `githubclient.PR` gains `StaleApprovedByUsers []Collaborator`: the users with a push-dismissed approval and no `APPROVED` review
  - Deduped by login, no bots, never the PR author
  - `CommentedByUsers` leaves them out. `ApprovedByUsers` is unchanged
- A push-dismissed approval is a `ReviewDismissedEvent` with `previousReviewState` `APPROVED` and a null `dismissalMessage`. Its `review`'s author is the stale approver. A manual dismissal carries a message, since the API requires one. See docs/third-party-facts.md § A manual review dismissal requires a message, and a push dismissal reads a null `dismissalMessage`
- Both selections, `enrichedPullRequestSelection` and `fullPullRequestSelection`, gain one aliased connection beside the existing `timelineItems`:
  ```
  reviewDismissals: timelineItems(itemTypes: [REVIEW_DISMISSED_EVENT], last: 100){ nodes { ... on ReviewDismissedEvent {
    previousReviewState dismissalMessage review { author { login __typename ... on User { name } } } } } }
  ```
  - Cost: 27 points instead of 26 over 25 PRs. See docs/third-party-facts.md § Adding `headRefOid`, `reviewDecision`, a review's `commit { oid }` and a second filtered `timelineItems` added at most 1 point to the enrichment query
  - Populates under `pull-requests: read` on `GITHUB_TOKEN`, in a public and a private repository
- `prview.PR` gains `StaleApprovers []Collaborator`, with Slack IDs attached as for `Approvers`
- `prview.GetReviewersTextSegments(approvers, staleApprovers, commenters, mergeBlockers)` renders the groups `✅`, `🔁`, `💬`, then the merge blockers, in that order, separated by ` / `: `(✅ Dana Davis / 🔁 Erin Evans / 💬 Frank Fisher / CI failing)`
  - `prview.PR.ReviewerGroupsForOpenRow() (staleApprovers, commenters []Collaborator)` splits them for an open row: `StaleApprovers` and `Commenters` when `GetNextAction()` is `NextActionWaitingForReview`, else no stale approvers and `Commenters` followed by `StaleApprovers`. Both builders' open rows call it
  - Merged rows pass `StaleApprovers` and `Commenters` as they are. Draft rows pass no stale approvers, as they pass no approvers
- `GetNextAction` doesn't change
- Action inputs: none new. Permissions: none new

Message states for a Dependabot PR that Dana approved:

```
Posted at 09:00, approved
*✅ Ready to merge*
  • Bump zod   3 days ago   by dependabot (✅ Dana Davis / 💬 Erin Evans)

Edited by an `update` after a rebase dismissed the approval
*👀 Waiting for review*
  • Bump zod   3 days ago   by dependabot (🔁 Dana Davis / 💬 Erin Evans)

Edited by an `update` after Dana approved again
*✅ Ready to merge*
  • Bump zod   3 days ago   by dependabot (✅ Dana Davis / 💬 Erin Evans)

Marked stale by the next post, after the message was posted while the approval was dismissed
⚠️ Stale, updated Nov 2 at 09:00
*👀 Waiting for review*
  • Bump zod   3 days ago   by dependabot (🔁 Dana Davis / 💬 Erin Evans)
```

- Today the "after a rebase" and "marked stale by the next post" states read `(💬 Dana Davis, Erin Evans)`
- The canvas row reads `**[Bump zod](url)** _3 days ago_ by dependabot (🔁 Dana Davis / 💬 Erin Evans)` under `## 👀 Waiting for review`, and follows the same states
- A merged row shows the reviewer list in the same format, after its merge time

## Breaking or non-breaking

Non-breaking, **minor**. Row text changes, with no input or permission change.

- The merged PR cache saves `StaleApprovedByUsers`. A cache saved before this change decodes it empty, so a cached merged row lists its push-dismissed approvers under `💬`, as today
- The first run after upgrading re-renders every row with a push-dismissed approval. The canvas is rewritten in that run only if it has such a row

## Summary

- **1** Fetch push-dismissed approvals in `githubclient` (checkpoint)
- **2** Render `🔁` in the reviewer list, in the message and on the canvas
- **3** Check the rows live
- **4** Docs

## Steps

### 1. Fetch push-dismissed approvals in `githubclient` (checkpoint)

Touches `internal/apiclients/githubclient`, `testhelpers/mockgithubclient`.

- The connection populates on a private repository under `pull-requests: read` alone. See docs/third-party-facts.md § On a private repository, `pull-requests: read` alone makes a review's `commit`, `baseRef` and a `PullRequestCommit` timeline node `FORBIDDEN`, while REST `pulls/{n}/reviews` returns `commit_id`
  - Its fixture stays: PR 7 of private `hellej/pr-slack-reminder-test-repo-1` carries a manually dismissed approval by `hellej`, so every E2E run listing that repository reads a private-repo `ReviewDismissedEvent`
- `pullRequestNode` gains `ReviewDismissals`. The existing `timelineItems` field keeps its name
- `deriveReviewers` derives `StaleApprovedByUsers` too, and leaves those users out of `CommentedByUsers`. `prWithReviewers` passes it the push-dismissed approvals' authors, skipping a null `review` or `author`. See docs/third-party-facts.md § `ReviewDismissedEvent.review` and `dismissalMessage` are nullable, `previousReviewState` is not
- `StaleApprovedByUsers` is tagged `json:",omitempty"`, so no state snapshot changes in this step: an empty list is left out of the saved merged PR cache, and decodes the same as before
- Unit tests cover: dedupe by login, bots and the PR author left out, a null versus a set `dismissalMessage`, a null `review` or `author`, a later `APPROVED` review cancelling the stale entry, and a stale approver left out of `CommentedByUsers`. A test pins the query text of both selections, and another runs the mock through `FindOpenPRs` and `GetPRs`
- `mockgithubclient`: `MockGitHubClientOptions` gains `ReviewDismissalsByPRNumber`, the dismissal events per PR, as `[]ReviewDismissal`. `NewPushDismissedApproval` builds one with a null message
- The grouped canvas E2E runs already list public `hellej/pr-slack-reminder-test-repo-3`, added by plan 019. A ruleset on its `main` dismisses approvals on push. [hellej/pr-slack-reminder-test-repo-3#7](https://github.com/hellej/pr-slack-reminder-test-repo-3/issues/7) set up four open PRs to `main`, authored by `github-actions[bot]` and approved by `hellej`:
  - PR 10, **push-dismissed**: approved, then a push
  - PR 11, **manually dismissed**: approved, then dismissed with a message
  - PR 12, **current approval**: approved
  - PR 13, **re-approved**: approved, a push, then approved again
  - Not Dependabot: 2 or more Dependabot PRs in a section collapse into one row without a reviewer list
  - The E2E GitHub App is installed on it
- Update `githubclient.spec.md`: `StaleApprovedByUsers`, the new connection, the `CommentedByUsers` line, the `FirstReviewAt` line, which still counts a dismissed review, and the PR-scoped field lists of the `FindOpenPRs` and `GetPRs` lines under § Doesn't Do, which gain `reviewDismissals`
- Verifying it done:
  - `make test`
  - The implementation PR's Build run: the E2E job log shows no enrichment error line
  - Put the enrichment `rateLimit.cost` in the implementation PR's description, next to the same figure from the last `main` Build run, per AGENTS.md § GitHub API Budget

### 2. Render `🔁` in the reviewer list

Touches `internal/prview`, `internal/messagebuilder`, `internal/canvasbuilder`, `cmd/pr-slack-reminder` snapshot tests.

- `prview.PR.StaleApprovers` and `GetReviewersTextSegments` as in § Target shape
- `ReviewerGroupsForOpenRow` as in § Target shape
- Call sites: the message's `getReviewersElements`, shared by open and merged rows, takes every reviewer group from its callers, as it takes `mergeBlockers`, the same signature as the canvas's `renderReviewers`. Never call `ReviewerGroupsForOpenRow` inside it: a merged PR with an approver reads `NextActionReadyToMerge`, so its merged row would lose `🔁`
- The canvas's `renderReviewers` already takes its groups from its callers: open, merged and draft rows
- Open rows read `ReviewerGroupsForOpenRow`, merged rows `StaleApprovers` and `Commenters`, draft rows no stale approvers
- Update `prview.spec.md`: the `GetReviewersTextSegments` line, and a § Behaviour line for `ReviewerGroupsForOpenRow` and its rule. Update `messagebuilder.spec.md` and `canvasbuilder.spec.md`
- In the post, update-mode and canvas snapshot fixtures, give one PR in "waiting for review" that has a commenter a push-dismissed approver: post #74, update-mode #65, canvas #43 (which also gains a commenter, since no canvas PR in that section had one). In the post and canvas fixtures, also give one merged PR that has a current approver a push-dismissed approver (post #83, canvas #9), so a merged row split by next action would move that push-dismissed approver under `💬`. And give one PR in "waiting for author" a push-dismissed approver whose name sorts before its commenter's, so the `💬` order shows:
  - Canvas #34, with a thread waiting for its author
  - Post #73, through its changes request: the mock GitHub client renders no review threads
- The canvas WIP PR #3 gets a push-dismissed approver too, and its row stays unchanged
- Then `make update-test-snapshots`. Read each diff before re-recording: only those PRs' rows change
- `prview` unit tests cover the `GetReviewersTextSegments` group order, `ReviewerGroupsForOpenRow` in each next action, and the stale approvers' Slack IDs, which no row text shows
- Expect only the state snapshots whose cached merged PR has a push-dismissed approver to gain the `StaleApprovedByUsers` key: step 1 tags it `omitempty`

### 3. Check the rows live

Touches no files. Step 1's repo-3 PRs are the setup.

- The implementation PR's Build E2E grouped canvas runs ("Run with filters, canvas link and merge stats: post" and its update) show, under repo-3:
  - **push-dismissed** under "Waiting for review" with `Joose` under `🔁`
  - **manually dismissed** under "Waiting for review" with `Joose` under `💬`
  - **current approval** and **re-approved** under "Ready to merge" with `Joose` under `✅`
  - Rows name a reviewer by GitHub display name: `hellej` reads `Joose`
- Done means the message and the canvas both show those rows

### 4. Docs

Touches no files.

- No `README.md` change: a README cleanup on `main` dropped the row format prose from § Example Output, collapsed rows included, so `🔁` gets no line there either
- Done means `make check-style` passes

## Consequences

### Positive

- A PR whose approval a push dismissed reads as a quick re-stamp, with the reviewer named
- One point more per 25 enriched PRs, and no extra request

### Negative

- None

### Caveats

- The README's update mode example triggers on no `synchronize`. A push starts an `update` once the CI workflows its `workflow_run` trigger lists complete, or earlier if the dismissal fires `pull_request_review: dismissed`, and no GitHub docs page says whether it does. This repository's `pr-reminder.yml` has no `workflow_run` and skips runs by `dependabot[bot]`, so a Dependabot rebase starts no `update` here. Without a run, the row changes at the next PR, review or comment event, CI completion or post
- Under "Require approval of the most recent reviewable push" without the dismissal rule, an approval made before the latest push keeps reading `✅` in "ready to merge"
- Collapsed rows have no reviewer list, so a collapsed Dependabot PR shows no `🔁`
- A draft row drops a push-dismissed approver, who read under `💬` on that row before
- A manual dismissal sent with an empty message may read as push-dismissed
- In "waiting for author", every push-dismissed approver reads under `💬`, also one whose only open item is the re-stamp, while another reviewer's thread holds the PR
- A PR with over 100 dismissals is judged on the newest 100

### Neutral

- None
