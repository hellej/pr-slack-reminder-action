# Hand a PR back to review once its feedback is answered

date: 2026-09-29
status: draft

## Requirements

Source: [issue #75](https://github.com/hellej/pr-slack-reminder-action/issues/75). A submitted `COMMENTED` or `CHANGES_REQUESTED` review never goes away in GitHub, so `HasNonApprovingReview` keeps a PR in "Waiting for author" after the author has dealt with the feedback. Reviewers never see it in "Waiting for review".

- A `COMMENTED` review asks the author for nothing beyond its threads. The thread rule (`HasThreadWaitingForAuthor`) already covers those, unchanged
  - A `COMMENTED` review with only a summary body no longer puts the PR under its author's turn
- A `CHANGES_REQUESTED` review by a human other than the PR author keeps the PR with the author until the author re-requests that reviewer's review
  - Approval by the same reviewer, or dismissal, clears it as today
- Re-requesting a reviewer does not clear that reviewer's unresolved threads. A thread still hands back only when resolved or when the author replies last
- `update` runs on a re-request, so the message and canvas move the PR without waiting for the next event
- Non-goals:
  - Pushing new commits as a hand-back signal
  - Team review requests as a hand-back signal: a review is always recorded under a user, never a team
  - Any change to how approvals, conflicts or bots are read
- Work lands on branch `claude/dreamy-faraday-o4sr2z`, then a PR

Purpose: every PR stuck under its author's turn is one the team's reviewers never pick up. In the reference deployment (0 to 8 open PRs, bot comments on every commit, Dependabot PRs driven by a human reviewer) one stuck PR is a large share of the queue.

## Target Shape

- `githubclient.PR.HasNonApprovingReview` becomes `HasOutstandingChangesRequest`: a submitted `CHANGES_REQUESTED` review by a non-bot user other than the PR author, whose author is not currently a requested reviewer on the PR
  - An **outstanding changes request** is this concept. Code, spec and README use the name
  - `COMMENTED` no longer sets any review-level flag
- The enrichment selection gains `reviewRequests(first: 100){ nodes { requestedReviewer { __typename ... on User { login } } } }` in both fetch fragments
  - No `Team` field is selected: every GraphQL `Team` field needs the `read:org` scope, and one team request then rejects the whole query. See [Why no Team fields](#why-no-team-fields)
  - A `requestedReviewer` that is null (a hidden code-owner team) or not a `User` matches no review
- Why re-request works as the signal: "Once a requested reviewer submits a review, they are no longer considered a requested reviewer" ([REST review requests](https://docs.github.com/en/rest/pulls/review-requests)). So a reviewer holding both a changes request and a pending request was re-requested after reviewing. No timestamp comparison needed. See [Why review requests, not push times](#why-review-requests-not-push-times)
  - That the "Re-request review" button puts the reviewer back into `reviewRequests` is not documented. Pending: the user's live check on a team PR
- `GetNextAction` keeps its three ordered checks. Check 2 reads `HasOutstandingChangesRequest` in place of `HasNonApprovingReview`
- The `pull_request` trigger gains `review_requested`, in `.github/workflows/pr-reminder.yml` and the README's update-mode example
- No change to `action.yml` inputs. No new permission: `pull-requests: read` covers `requested_reviewers` ([fine-grained PAT permissions](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens)). GraphQL permissions stay undocumented, per docs/third-party-facts.md § No GitHub page documents the token permission for any GraphQL field
- Slack rendering is unchanged: the same three sections, only which PR lands where

## Breaking Change

Non-breaking. Patch: a bucketing fix, no input, state or rendering change.

- State holds no bucketing, so the first run after upgrading re-buckets every PR. The canvas markdown hash changes wherever a PR moves, so that run rewrites the canvas once

## Summary

1. (checkpoint) Select review requests and derive `HasOutstandingChangesRequest` in `githubclient`
2. Re-word the bucketing docs and add the `review_requested` trigger

## Steps

### Step 1: derive `HasOutstandingChangesRequest` (checkpoint)

Packages: `internal/apiclients/githubclient/`, `internal/prview/`, plus test option structs in `internal/messagecontent/` and `internal/canvascontent/`

- Add the `reviewRequests` selection to `enrichedPullRequestSelection` and `fullPullRequestSelection`
- Add `ReviewRequests connection[reviewRequestNode]` to `pullRequestNode`. `reviewRequestNode` holds `RequestedReviewer *authorNode`, whose `Login` and `Typename` tags already fit
- Replace `hasNonApprovingNonOwnReview` with a derivation over the submitted user reviews:
  - `CHANGES_REQUESTED` only, not by the PR author
  - Its author's login is not among the requested `User` logins
  - Both sides compared through `collaboratorFromAuthorNode`, as the thread and author checks do
- Drop `isNonApprovingReviewState` and `commentedReviewState` if nothing else reads them
- Rename the `PR` field across ~6 files: `prview.GetNextAction`, its tests, and the test option structs in `messagecontent` and `canvascontent`. Comment on the field names the re-request
- Tests: rework `TestPRWithReviewersDerivesNonApprovingReviews` into the new rule's table. Cases a wrong implementation gets wrong:
  - A `COMMENTED` review alone: false
  - Changes requested, reviewer re-requested: false
  - Changes requested, a different user requested: true
  - Changes requested, a team or null reviewer requested: true
- Extend `TestPRWithReviewersDerivesFlagsFromDecodedJSON` with a `reviewRequests` key, so a mistagged `reviewRequests`, `requestedReviewer` or `login` fails
- `prview` tests: the existing `GetNextAction` cases change by field name only
- Update `githubclient.spec.md`:
  - Replace the `HasNonApprovingReview` bullet with the new rule and why `COMMENTED` is left to the threads
  - Per PR, the first 100 review requests are read
  - Oddities: an author without write access cannot re-request, so their changes request holds until approval or dismissal; a changes-requesting reviewer who is re-requested and then only comments has their request removed again, so the changes request counts again, as GitHub also shows it
- Live check, needed because no doc says whether a `Team` union member with no fields selected passes the scope check under `GITHUB_TOKEN`:
  - On the branch, temporarily set `github-repositories` in `.github/workflows/pr-reminder.yml` to this repository plus a public repository with an open PR carrying a pending team review request (visible under "Reviewers" on github.com)
  - Dispatch `gh workflow run pr-reminder.yml --ref claude/dreamy-faraday-o4sr2z -f run-mode=post -f build-first=true`
  - Done when the run is green and its log has a "Found N reviews" line for that PR, not "Unable to fetch reviews/comments". Then revert the workflow edit
  - If the query is rejected, stop and hand back: the target shape needs another source for review requests

### Step 2: docs and the `review_requested` trigger

Files: `README.md`, `.github/workflows/pr-reminder.yml`, `internal/prview/prview.spec.md`

- `prview.spec.md`: `GetNextAction` bullets name an outstanding changes request in place of "a non-approving review", and the field list names `HasOutstandingChangesRequest`
- `README.md` § PR Tracker Canvas: "a review comment or a review thread its author hasn't answered" becomes a changes request not yet re-requested, or a review thread its author hasn't answered
- Add `review_requested` to the `pull_request` `types:` in `pr-reminder.yml` and in the README's update-mode example
  - Done when a re-request on a PR in this repository starts a `PR Reminder` run

## Consequences

### Positive

- A PR whose threads are all resolved or answered returns to "Waiting for review", as in the issue's example
- The author clears a changes request with GitHub's own signal, the re-request button

### Negative

None

### Caveats

- A `COMMENTED` review with only a summary body no longer hands the PR to its author
- Pushing fixes without re-requesting leaves a changes request outstanding
- An author without write access, such as a fork contributor, cannot re-request, so a changes request on their PR holds until approval or dismissal
- A team re-request never clears a member's changes request
- `review_requested` also fires on first-time requests, one extra `update` run each

### Neutral

- The first run after upgrading rewrites the canvas where a PR moves bucket
- One more connection per PR in the enrichment request

## Justification

### Why review requests, not push times

- GraphQL has no push time: `Commit.pushedDate` is deprecated, "no longer supported" since 2023-07-01 ([schema](https://raw.githubusercontent.com/github/docs/main/src/graphql/data/fpt/schema.docs.graphql)). `committedDate` is set by the committer and predates the push after a rebase
- `PullRequestReview.commit` against `headRefOid` shows the head moved, not who moved it: a reviewer's push or "Update branch" moves it too
- A pending review request needs no clock: GitHub removes it when the reviewer submits, so its presence after a review means the author asked again

### Why no Team fields

- Selecting a `Team` field such as `name` fails the whole query without `read:org`: "The 'name' field requires one of the following scopes: ['read:org', 'read:discussion']" ([cli/cli#812](https://github.com/cli/cli/issues/812)); superset replaced the same selection with REST for this reason ([superset#7788](https://github.com/superset-sh/superset/pull/7788/files))
- A review is always recorded under a user, and a team request is removed once a member reviews ([community discussion 16853](https://github.com/orgs/community/discussions/16853)), so a team never matches a changes request anyway
