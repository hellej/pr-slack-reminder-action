# Mark PRs whose approval went stale

date: 2026-11-01
status: draft

Lands on the feature branch `plan-020-stale-approvals`, through a PR.

## Requirements

- An open PR whose approval went stale after a push names that approver in a `♻️` group of its reviewer list, in the message and on the canvas: `(♻️ Dana Davis / 💬 Erin Evans)`
- Both rules that make an approval stale are covered:
  - **Dismissed by a push**: "Dismiss stale pull request approvals when new commits are pushed" in branch protection, or `dismiss_stale_reviews_on_push` in a ruleset. GitHub dismisses the approval
  - **Approval before the last push**: "Require approval of the most recent reviewable push", without the dismissal rule. The approval stays `APPROVED`, but no longer allows the merge
- A PR whose approvals are all stale is not "ready to merge". With nothing else outstanding it is "waiting for review"
- An approver who approves again is a current approver: `✅`, not `♻️`
- A manually dismissed approval is not stale: its reviewer stays under `💬`, as today
- The notification text is unchanged: a PR with stale approvals counts under "to review"
- No new action input, no new permission

Serves § **Purpose**: today a dismissed approval turns a PR into one that looks untouched (`💬` only), and under the last-push rule a PR sits in "ready to merge" with a blocked merge button. `♻️` says the PR needs a quick re-stamp and names who gave the last one. Sized for § **Reference Deployment**: human approval gates every merge, and a weekly Dependabot batch rebases ~5 approved PRs at once.

### Non-goals

- No new section, no change to the sort order
- No Slack mention of a stale approver
- No `♻️` on merged, draft or collapsed rows
- No flag on an approval older than the latest commit when no rule asks for re-approval: the merge is still allowed
- No fix for a PR short of a required approval count: with one current approval of two required, it stays "ready to merge"

## Target shape

- `githubclient.PR` gains `StaleApprovedByUsers []Collaborator`: the users whose approval went stale and who have no current approval
  - Deduped by login, no bots, never the PR author
  - Disjoint from `ApprovedByUsers`. `CommentedByUsers` leaves them out
- A user's approval went stale when either holds:
  - A `ReviewDismissedEvent` has `previousReviewState` `APPROVED`, a non-null `pullRequestCommit`, and names the user as its `review`'s author. See docs/third-party-facts.md § A push that dismisses a stale approval leaves the review `DISMISSED` and records a `ReviewDismissedEvent` with `pullRequestCommit` set
  - `reviewDecision` is `REVIEW_REQUIRED` and the user has an `APPROVED` review whose `commit.oid` is not `headRefOid`. A review with a null `commit` counts as not on `headRefOid`
- A current approval is an `APPROVED` review, except under `REVIEW_REQUIRED`, where it must be on `headRefOid`. `ApprovedByUsers` holds current approvals only
- Both selections, `enrichedPullRequestSelection` and `fullPullRequestSelection`, gain:
  ```
  headRefOid reviewDecision
  reviews(last: 100){ nodes { ... commit { oid } } }
  reviewDismissals: timelineItems(itemTypes: [REVIEW_DISMISSED_EVENT], last: 100){ nodes { ... on ReviewDismissedEvent {
    previousReviewState pullRequestCommit { id } review { author { login __typename ... on User { name } } } } } }
  ```
  - All populate under `pull-requests: read` on `GITHUB_TOKEN`. See docs/third-party-facts.md § A review's `commit { oid }`, `headRefOid`, `reviewDecision` and `ReviewDismissedEvent` populate under `pull-requests: read` on `GITHUB_TOKEN`, in a public repository
  - Cost: no change measured over 18 PRs. See docs/third-party-facts.md § Adding `headRefOid`, `reviewDecision`, a review's `commit { oid }` and a second filtered `timelineItems` kept the enrichment query's cost
- `prview.PR` gains `StaleApprovers []Collaborator`, with Slack IDs attached as for `Approvers`
- `prview.PR.GetNextAction` doesn't change. It reads `Approvers`, which now holds current approvals only
- `prview.GetReviewersTextSegments(approvers, staleApprovers, commenters)` renders the groups `✅`, `♻️`, `💬`, in that order, separated by ` / `
  - Open rows pass `StaleApprovers`
  - Merged and draft rows pass no stale approvers, and name them among the commenters, after them, as a dismissed approver reads today
  - Plan 019 adds a `hasFailingChecks` parameter to the same function. Whichever plan lands second keeps `❌ CI` as the last group
- `FindOpenPRs` logs one line per PR with stale approvers, naming them
- Action inputs: none new. Permissions: none new

Message states for a Dependabot PR that Dana approved, under the dismissal rule:

```
Posted at 09:00, approved
*✅ Ready to merge*
  • Bump zod   3 days ago   by dependabot (✅ Dana Davis / 💬 Erin Evans)

Edited by an `update` after a rebase dismissed the approval
*👀 Waiting for review*
  • Bump zod   3 days ago   by dependabot (♻️ Dana Davis / 💬 Erin Evans)

Edited by an `update` after Dana approved again
*✅ Ready to merge*
  • Bump zod   3 days ago   by dependabot (✅ Dana Davis / 💬 Erin Evans)

Marked stale by the next post, after the message was posted while the approval was stale
⚠️ Stale, updated Nov 2 at 09:00
*👀 Waiting for review*
  • Bump zod   3 days ago   by dependabot (♻️ Dana Davis / 💬 Erin Evans)
```

Under the last-push rule the states are the same.

The canvas row reads `**[Bump zod](url)** _3 days ago_ by dependabot (♻️ Dana Davis / 💬 Erin Evans)` under `## 👀 Waiting for review`, and follows the same states.

## Breaking or non-breaking

Non-breaking, **minor**. Row text changes, and under the last-push rule a PR changes bucket, with no input or permission change.

- The merged PR cache saves `StaleApprovedByUsers` beside `ApprovedByUsers`. A cache saved before it decodes none, so a cached merged row names its dismissed approvers among the commenters, as today
- The first run after upgrading re-renders every PR with a stale approval, and the canvas rewrites once where one exists

## Summary

- **1** Fetch stale approvals in `githubclient` (checkpoint)
- **2** Render `♻️` in the reviewer list, in the message and on the canvas
- **3** Check the message and canvas live
- **4** Docs

## Steps

### 1. Fetch stale approvals in `githubclient` (checkpoint)

Touches `internal/apiclients/githubclient`, `testhelpers/mockgithubclient`, `.github/actions/e2e-tests/action.yml`.

- The selections and `StaleApprovedByUsers` as in § Target shape. `prWithReviewers` derives it beside `deriveReviewers`, whose approvers become the current ones
- `pullRequestNode` gains `HeadRefOid`, `ReviewDecision` and `ReviewDismissals`, and `reviewNode` gains `Commit`. The existing `timelineItems` field keeps its name
- Unit tests in `githubclient` cover each rule of § Target shape, including:
  - A push-dismissed approval followed by a fresh approval from the same user: current, not stale. PR 52 of this repository has this shape
  - A manual dismissal, a null `pullRequestCommit`: not stale
  - `REVIEW_REQUIRED` with one approval on `headRefOid` and another user's on an older commit: one current, one stale
  - `APPROVED` or null `reviewDecision` with an approval on an older commit: current
- `mockgithubclient`:
  - `reviewNodeJSON` writes `commit.oid` from `github.PullRequestReview.CommitID`, and a PR's `headRefOid` from `Head.SHA`
  - `MockGitHubClientOptions` gains `ReviewDecisionByPRNumber map[int]string` and `ReviewDismissalsByPRNumber`, the dismissal events per PR
- Add `hellej/pr-slack-reminder-test-repo-3` to every E2E run that lists repositories. It is public, so it can carry the rules. The owner set it up:
  - `main` under a ruleset dismissing stale approvals, `last-push-base` under one requiring last-push approval
  - Fixture PRs, all by `hellej` and approved by a second account: A, push-dismissed on `main`; B, approved before the last push on `last-push-base`; C and D, current approvals on each base; E, manually dismissed on `main`; F, changes requested then a push, on `main`
  - The E2E GitHub App is installed on it
- Update `githubclient.spec.md`: `StaleApprovedByUsers`, the current approval rule, the new fields, and the "Doesn't read `reviewDecision`" line, which now reads it for the last-push rule only
- Verifying it done:
  - `make test`
  - The repo-3 probe results are in docs/third-party-facts.md, amending § With "Require approval of the most recent reviewable push", older approvals stay `APPROVED`, and what `reviewDecision` reports is undocumented
  - If B's `reviewDecision` is not `REVIEW_REQUIRED`, or E's dismissal event has a non-null `pullRequestCommit`, stop and raise it with the user: the stale rule needs another signal
  - After pushing the branch, the Build job log shows stale approver lines for A and B only, no enrichment error line, and the enrichment `rateLimit.cost` with its difference against the last `main` Build run (state it in the PR, per AGENTS.md § GitHub API Budget)

### 2. Render `♻️` in the reviewer list

Touches `internal/prview`, `internal/messagebuilder`, `internal/canvasbuilder`, `cmd/pr-slack-reminder` snapshot tests.

- `prview.PR.StaleApprovers` and `GetReviewersTextSegments` as in § Target shape. ~4 call sites: the message's `getReviewersElements`, the canvas's `renderReviewers` for open, WIP and merged rows
- `♻️` is part of the glue segments, so neither builder escapes or styles it as a name
- Update `prview.spec.md` (the new field, `GetReviewersTextSegments`, and the Oddities line on "a user with any `APPROVED` review counts as an approver", which now holds outside the last-push rule only), `messagebuilder.spec.md` and `canvasbuilder.spec.md`
- Add a PR with a stale approver and a commenter to three fixture sets, then `make update-test-snapshots`:
  - `snapshotScenarios()` in `cmd/pr-slack-reminder/snapshot_test.go`, which serves the post and marked-stale snapshots
  - The update-mode table of `TestSnapshotsUpdateMode`, for the edited message
  - The fixtures of `TestBuildMarkdownSnapshots` in `internal/canvasbuilder/canvasbuilder_test.go`, for the canvas row
- Expect every state snapshot with a cached merged PR to gain the `StaleApprovedByUsers` key
- `main_test.go`: a last-push PR with only a stale approval lands in "waiting for review", in post and in update mode

### 3. Check the message and canvas live

Touches no files. The repo-3 fixtures are the setup.

- Read the branch's Build E2E runs. The grouped canvas runs ("Run with filters, canvas link and merge stats: post" and its update) show, under repo-3:
  - A and B under "Waiting for review" with `♻️` and the second account's name
  - C and D under "Ready to merge" with `✅`
  - E under "Waiting for review" with the second account under `💬`
  - F under "Waiting for author"
- Done means both the message and the canvas show those rows

### 4. Docs

Touches `README.md`.

- The bucket paragraph under `README.md` § PR Tracker Canvas: "ready to merge" needs a current approval. An approval dismissed by a push, or made before the last push under the last-push rule, is listed under `♻️` instead of `✅` until the reviewer approves again
- The screenshots in `docs/examples/` stay: no sample shows a stale approval
- Done means `make check-style` passes

## Consequences

### Positive

- A PR whose approval went stale reads as a quick re-stamp, not as untouched
- Under the last-push rule, "ready to merge" stops listing PRs with a blocked merge button
- No extra request and no measured cost

### Negative

- None

### Caveats

- No run reacts to the push itself: the example triggers carry no `synchronize`. Whether an automatic dismissal fires `pull_request_review: dismissed` is undocumented. Until the next PR, review or comment event or post, the message shows the PR as before the push
- Collapsed rows have no reviewer list, so a collapsed Dependabot PR with a stale approval says nothing about it
- Under the last-push rule, a push by the approver themselves needs another person's approval, but the row still names them under `♻️`
- Under the last-push rule with more than one required approval, `REVIEW_REQUIRED` also reads while the count is short. An approval on an older commit then reads stale even if GitHub still counts it
- A user whose approval was dismissed and who then requested changes reads under `♻️`, while the PR sits in "waiting for author"
- A PR with over 100 dismissals or reviews is judged on the newest 100, as for reviews today

### Neutral

- The notification text counts these PRs under "to review"
