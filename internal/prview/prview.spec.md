# prview

Enriches fetched PRs with display-ready metadata.

## Behaviour

- `BuildPRViews(prs, contentInputs)` returns PRs enriched for display, in the given order
- Each collaborator (author, approvers, commenters) gets a Slack user ID attached when one is mapped for their GitHub login; unmapped users get an empty Slack ID
- A PR is flagged `IsOldPR` when an old-PR age threshold is configured and the PR is older than it
- `GetPRAgeText` renders age as days, hours, or minutes depending on magnitude; `GetPRAgeDisplayText` adds the suffix, "N days old" for a PR flagged old and "N days ago" otherwise. The old-PR warning marker belongs to the renderer
- `GetActivityText` renders time since `UpdatedAt` in the same magnitudes: "updated N minutes/hours ago" under a day, "idle N days" from a day onwards
- `GetMergedText` renders time since `MergedAt` as "merged N minutes/hours/days ago", prefixed like the activity text so a merge time cannot be misread as an age. A PR that was never merged yields no text
- `IsActiveAsOf(asOf, threshold)` is true when `UpdatedAt` is within `threshold` of `asOf`, inclusive at the boundary. Callers give both, so a caller that can't read the clock passes `time.Now()` itself, and a canvas-generation timestamp stays reusable
- `IsRecentlyUpdated` is `IsActiveAsOf(time.Now(), RecentActivityThreshold)`, for a caller that can read the wall clock directly. `RecentActivityThreshold` (24 hours) is exported so other packages bucket by the same boundary; it matches where `GetActivityText` flips from "updated" to "idle"
- Unknown activity (a zero `UpdatedAt`) yields empty activity text but counts as active for both `IsActiveAsOf` and `IsRecentlyUpdated`
- `SortPRsNewestFirst(prs, timestamp)` returns PRs ordered newest first by the given timestamp, nil timestamps last, given order kept among equals. It leaves the given slice untouched
- `GetReviewersTextSegments(approvers, commenters)` renders reviewer names as `(✅ a, b / 💬 c)`, returning one text run per segment so a renderer can style or escape names separately from the glue; no reviewers yields no segments. Both groups are parameters, so a caller passing no approvers gets the commenters-only rendering
- `IsMerged` reports whether a PR was merged
- `GetPullRequestRef()` returns the `models.PullRequestRef` (repository + number) that identifies a PR across fetches, independent of its current state
- `IsOpen` and `IsDraft` are inverse views of `GetDraft()`
- `LastActivityAt` returns `UpdatedAt` as a pointer, nil when it's zero, the nil convention `SortPRsNewestFirst` expects for unknown activity
- `GetNextAction()` says what a PR needs next, as the first of three ordered checks that matches: approved with nothing outstanding is `NextActionReadyToMerge`; an approval, a non-approving review or a thread waiting for the author is `NextActionWaitingForAuthor`; anything else is `NextActionWaitingForReview`. A `PRNextAction` is an identifier, not a heading: each renderer supplies its own wording
- The checks being ordered is what settles the overlaps: a reviewer who commented and then approved leaves the PR ready to merge, since the approval is read before the comment
- A conflict only demotes. It keeps an approved PR out of `NextActionReadyToMerge`, while an unreviewed conflicting PR stays in `NextActionWaitingForReview`, where reviewing around a coming rebase is not wasted work
- `GetNextAction` reads `Conflicting`, `HasThreadWaitingForAuthor` and `HasNonApprovingReview` off the fetched PR, and the approvals off `Approvers`, the same list a row's reviewer segment names, so a next action can never disagree with the row beside it
- `GroupPRsByRepositoriesInGivenOrder(prs)` buckets PRs into `[]RepositoryPRs`, ordered by each repository's first PR in the given list; PRs keep their given order within a bucket. Feeding it an already-sorted list puts the repository holding the leading PR first, whatever the sort was. It carries no display text, so each renderer supplies its own headings and links

## Doesn't Do

- Doesn't validate that mapped Slack user IDs are well-formed
- Doesn't handle a creation time in the future: the age text goes negative, e.g. "-30 minutes"

## Oddities

- `GetNextAction` inherits `githubclient`'s reading of an approval: a user with any `APPROVED` review counts as an approver, so a PR approved and then changes-requested by the same person, with every thread answered and no conflict, files as ready to merge
- `GetNextAction` on a PR without its fetched half, a nil embedded `githubclient.PR`, reports `NextActionWaitingForReview`. It carries no signal to read, and keeping it in the review queue beats panicking a canvas render
- Age and activity text are rounded to whole units, singular at a count of 1 and plural otherwise (0 included), so a one-day-old PR reads "1 day" (and "idle 1 day") and a 23.6-hour-old PR reads "24 hours"
- A PR with a missing/zero creation timestamp counts as old whenever a threshold is set, whatever the threshold value
- An old-PR threshold of 0 turns the check off instead of flagging every PR as old
