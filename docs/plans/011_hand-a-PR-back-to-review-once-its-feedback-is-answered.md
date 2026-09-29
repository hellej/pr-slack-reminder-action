# Hand a PR back to review once its feedback is answered

date: 2026-09-29
status: implemented

## Requirements

Source: [issue #75](https://github.com/hellej/pr-slack-reminder-action/issues/75). A submitted `COMMENTED` or `CHANGES_REQUESTED` review never goes away in GitHub, so `HasNonApprovingReview` keeps a PR in "Waiting for author" after the author has dealt with the feedback. Reviewers never see it in "Waiting for review".

- A `COMMENTED` review asks the author for nothing beyond its threads. The thread rule (`HasThreadWaitingForAuthor`) already covers those, unchanged
  - A `COMMENTED` review with only a summary body no longer puts the PR under its author's turn
- A `CHANGES_REQUESTED` review by a human other than the PR author keeps the PR with the author until the author re-requests that reviewer's review
- Re-requesting a reviewer does not clear that reviewer's unresolved threads. A thread still hands back only when resolved or when the author replies last
- Any approval still wins, as today: reviewer A's approval with reviewer B's outstanding changes request, no blocking thread and no conflict, is ready to merge
- `update` runs on a re-request, so the message and canvas move the PR without waiting for the next event
- Non-goals:
  - Pushing new commits as a hand-back signal
  - Team review requests as a hand-back signal
  - Any change to how approvals, conflicts or bots are read
- No new token permission or OAuth scope, `read:org` included
- Work lands on branch `claude/dreamy-faraday-o4sr2z`, then a PR

Purpose: every PR stuck under its author's turn is one the team's reviewers never pick up. In the reference deployment (0 to 8 open PRs, bot comments on every commit, Dependabot PRs driven by a human reviewer) one stuck PR is a large share of the queue.

## Target Shape

- `githubclient.PR.HasNonApprovingReview` becomes `HasOutstandingChangesRequest`: a submitted `CHANGES_REQUESTED` review by a non-bot user other than the PR author, whose author is not currently a requested reviewer on the PR
  - An **outstanding changes request** is this concept. Code, spec and README use the name
  - Later reviews by the same user are not read. An approval clears the PR through `GetNextAction`'s check 1, not through this flag
  - `COMMENTED` no longer sets any review-level flag
- The enrichment selection gains `reviewRequests(first: 100){ nodes { requestedReviewer { __typename ... on User { login } } } }` in both fetch fragments
  - No `Team` field is selected. See [Why no Team fields](#why-no-team-fields)
  - A `requestedReviewer` that is null (a hidden code-owner team) or not a `User` matches no review
- A pending request after a review means the author asked again: GitHub removes the request when the reviewer submits. See docs/third-party-facts.md § A submitted review removes its author from a PR's requested reviewers, and [Why review requests, not push times](#why-review-requests-not-push-times)
  - "Re-request review" puts the reviewer back into `reviewRequests`. See docs/third-party-facts.md § "Re-request review" puts a reviewer back into `reviewRequests`, and their earlier review stays
- `GetNextAction` keeps its three ordered checks. Check 2 reads `HasOutstandingChangesRequest` in place of `HasNonApprovingReview`
- The `pull_request` trigger gains `review_requested`, in `.github/workflows/pr-reminder.yml` and the README's update-mode example ([events that trigger workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows))
- No change to `action.yml` inputs. No new permission: `pull-requests: read` covers `requested_reviewers` in REST, per docs/third-party-facts.md § A submitted review removes its author from a PR's requested reviewers. GraphQL permissions stay undocumented, per docs/third-party-facts.md § No GitHub page documents the token permission for any GraphQL field
- Slack rendering is unchanged: the same three sections, only which PR lands where

## Breaking Change

Non-breaking. Patch: fixes issue #75, no input, state or rendering change.

- State holds no bucketing, so the first run after upgrading re-buckets every PR. The canvas markdown hash changes wherever a PR moves, so that run rewrites the canvas once

## Summary

1. (checkpoint) Select review requests, confirm the query passes under `GITHUB_TOKEN` and that a re-request after a changes request shows
2. Derive `HasOutstandingChangesRequest` and bucket on it
3. Re-word the bucketing docs and add the `review_requested` trigger

## Steps

### Step 1: select review requests (checkpoint)

Package: `internal/apiclients/githubclient/`

- Add the `reviewRequests` selection to `enrichedPullRequestSelection` and `fullPullRequestSelection`
- Add `ReviewRequests connection[reviewRequestNode]` to `pullRequestNode`. `reviewRequestNode` holds `RequestedReviewer *authorNode`, whose `Login` and `Typename` tags already fit
- Live check, needed because no doc says whether a `Team` union member with no fields selected passes the scope check under `GITHUB_TOKEN`:
  - Pick a public PR with a pending team review request, visible under "Reviewers" on github.com
  - Commit a temporary step in `.github/workflows/pr-reminder.yml`, before "Send PR reminder": `gh api graphql` with `GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}`, running the exact new selection against that PR and printing the response
  - Dispatch `gh workflow run pr-reminder.yml --ref claude/dreamy-faraday-o4sr2z -f run-mode=post -f build-first=true`
  - Done when the response carries a `"__typename": "Team"` node and no errors. Then revert the step in a second commit
  - If the team comes back only as `null`, the check proved nothing: ask the user to run the same step in their own team's repository
  - If the query is rejected, stop and hand back: the target shape needs another source for review requests
- Live check with the user, needed because the re-request was confirmed only on an `APPROVED` review to oneself:
  - On a team PR where a colleague submitted `CHANGES_REQUESTED`, the user re-requests that colleague
  - Done when `gh api repos/<owner>/<repo>/pulls/<n>/requested_reviewers` lists the colleague under `users`. Record the result in docs/third-party-facts.md § "Re-request review" puts a reviewer back into `reviewRequests`, and their earlier review stays
  - If the colleague is missing, stop and hand back: the re-request signal does not hold

### Step 2: derive `HasOutstandingChangesRequest`

Packages: `internal/apiclients/githubclient/`, `internal/prview/`, `testhelpers/mockgithubclient/`, `cmd/pr-slack-reminder/` snapshots, plus test option structs in `internal/messagecontent/` and `internal/canvascontent/`

- Replace `hasNonApprovingNonOwnReview` with a derivation over the submitted user reviews:
  - `CHANGES_REQUESTED` only, not by the PR author
  - Its author's login is not among the requested `User` logins
  - Both sides compared through `collaboratorFromAuthorNode`, as the thread and author checks do. Logins only: the selection reads a name off a review's author but not off a requested reviewer, so whole collaborators never match
- Extend `TestPRWithReviewersDerivesFlagsFromDecodedJSON` with a `reviewRequests` key, so a mistagged `reviewRequests`, `requestedReviewer` or `login` fails
- Drop `isNonApprovingReviewState` and `commentedReviewState`. Nothing else reads them
- Rename the `PR` field across ~7 files: `prview.GetNextAction`, its tests, and the test option structs in `messagecontent` and `canvascontent`. The field comment names the re-request
- Tests: rework `TestPRWithReviewersDerivesNonApprovingReviews` into the new rule's table. Cases a wrong implementation gets wrong:
  - A `COMMENTED` review alone: false
  - Changes requested, reviewer re-requested: false
  - Changes requested, a different user requested: true
  - Changes requested, a team or null reviewer requested: true
  - Changes requested, then approved by the same reviewer: true (the approval is check 1's to read)
  - Two reviewers requested changes, only one re-requested: true
  - The PR author's own changes request, a bot's changes request: false
  - Requested users carry no name, as the selection returns them, so a whole-collaborator comparison fails
- Both query-text tests (`TestBuildGetPRsQuery`, `TestBuildEnrichPRsQuery`) require the `reviewRequests` selection and forbid `Team`. The mock hand-builds its JSON, so these are what pin the selection
- Snapshots: PRs 74 and 65, both "A reviewer left a comment", carry a lone `COMMENTED` review, so they move from "💬 Waiting for author" to "👀 Waiting for review" across 8 snapshot files (4 messages, 4 saved states). Retitle both to what they now show. PRs 73 and 62 keep "Waiting for author" covered with a changes request
- `mockgithubclient`: `RequestedReviewerLoginsByPRNumber` renders as `User` review requests in `enrichedPullRequestNodeJSON`. Update mode's "every section under load" snapshot gains PR 60, a changes request whose reviewer was re-requested, sitting in "Waiting for review"
- Update `githubclient.spec.md`:
  - Replace the `HasNonApprovingReview` bullet with the new rule, why `COMMENTED` is left to the threads, and that later reviews by the same user are not read
  - Per PR, the first 100 review requests are read
  - § Doesn't Do: add `reviewRequests` to the two error-scope lists. In `GetPRs` a failed `reviewRequests` is only logged, so every changes request on that PR reads as outstanding
  - Oddities: an author without write access cannot re-request ("Pull request authors can request reviews only if they are repository owners or collaborators with write access", [about pull request reviews](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/reviewing-changes-in-pull-requests/about-pull-request-reviews)), so their changes request stays outstanding until someone with write access, such as the reviewer, re-requests the review, or the review is dismissed; a re-requested reviewer who then only comments is no longer requested, so their changes request counts again
- Update `prview.spec.md`: `GetNextAction` bullets name an outstanding changes request in place of "a non-approving review". Oddities: one reviewer's approval files a PR as ready to merge over another's outstanding changes request, next to the same-person case

### Step 3: README and the `review_requested` trigger

Files: `README.md`, `.github/workflows/pr-reminder.yml`

- `README.md` § PR Tracker Canvas: "a review comment or a review thread its author hasn't answered" becomes "a changes request whose reviewer hasn't been re-requested, or a review thread its author hasn't answered"
- Add `review_requested` to the `pull_request` `types:` in `pr-reminder.yml` and in the README's update-mode example
  - Live check: Dependabot authors PRs here and requests `hellej`, so the user, not being the author, can re-request themselves on an open Dependabot PR after reviewing it, as in the maintainer's re-request check. The trigger edit must be on `main` first: `pull_request` runs the workflow file of the PR's merge ref, so run it after the plan's PR merges
  - Done when that re-request starts a `PR Reminder` run whose event is `pull_request` with activity `review_requested`

## Consequences

### Positive

- A PR whose threads are all resolved or answered returns to "Waiting for review", as in the issue's example
- The author clears a changes request with GitHub's own signal, the re-request button

### Negative

None

### Caveats

- A `COMMENTED` review with only a summary body no longer hands the PR to its author
- Pushing fixes without re-requesting leaves a changes request outstanding
- An author without write access, such as a fork contributor, cannot re-request, so a changes request on their PR holds until someone with write access re-requests the review, a reviewer approves, or the review is dismissed
- A team re-request never clears a member's changes request
- Resolving a thread starts no run: no workflow event fires on it (docs/third-party-facts.md § Resolving a review thread triggers no GitHub Actions workflow). The PR moves on the next event or the 09:00 post
- `review_requested` also fires on first-time requests, one extra `update` run each

### Neutral

- One more connection per PR in the enrichment request

## Justification

### Why review requests, not push times

- GraphQL has no push time. See docs/third-party-facts.md § GraphQL has no push time for a PR's commits: `Commit.pushedDate` is deprecated
- `PullRequestReview.commit` against `headRefOid` shows the head moved, not who moved it: a reviewer's push or "Update branch" moves it too
- A pending review request needs no clock: GitHub removes it when the reviewer submits, so its presence after a review means someone asked again

### Why no Team fields

- Selecting any `Team` field fails the whole query without `read:org`. See docs/third-party-facts.md § Every GraphQL `Team` field needs `read:org`, and selecting one fails the whole query without it
- A team never matches a changes request anyway: a review is recorded under a user, and a team request is removed once a member reviews
