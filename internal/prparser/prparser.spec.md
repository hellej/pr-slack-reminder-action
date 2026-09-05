# prparser

Enriches fetched PRs with display-ready metadata.

## Behaviour

- `ParsePRs(prs, contentInputs)` returns PRs enriched for display, sorted oldest first (by creation time, ties broken by update time)
- Each collaborator (author, approvers, commenters) gets a Slack user ID attached when one is mapped for their GitHub login; unmapped users get an empty Slack ID
- A PR is flagged `IsOldPR` when an old-PR age threshold is configured and the PR is older than it
- `GetPRAgeText` renders age as days, hours, or minutes depending on magnitude; `GetPRAgeDisplayText` adds the suffix, "N days old" for a PR flagged old and "N days ago" otherwise. The old-PR warning marker belongs to the renderer
- `GetActivityText` renders time since `UpdatedAt` in the same magnitudes: "updated N minutes/hours ago" under a day, "idle N days" from a day onwards
- `GetMergedText` renders time since `MergedAt` as "merged N minutes/hours/days ago", prefixed like the activity text so a merge time cannot be misread as an age. A PR that was never merged yields no text
- `IsRecentlyUpdated` is true when `UpdatedAt` is under 24 hours old. The threshold is hardcoded, and matches where `GetActivityText` flips from "updated" to "idle"
- Unknown activity (a zero `UpdatedAt`) yields empty activity text, and counts as not recently updated
- `SortPRsNewestFirst(prs, timestamp)` returns PRs ordered newest first by the given timestamp, nil timestamps last, given order kept among equals. It leaves the given slice untouched
- `GetReviewersTextSegments(approvers, commenters)` renders reviewer names as `(✅ a, b / 💬 c)`, returning one text run per segment so a renderer can style or escape names separately from the glue; no reviewers yields no segments. Both groups are parameters, so a caller passing no approvers gets the commenters-only rendering
- `IsMerged` and `IsClosedButNotMerged` expose PR state for display styling
- `GroupPRsByRepositories(prs)` buckets PRs into `[]RepositoryPRs`, ordered alphabetically by repository path; PRs keep their given order within a bucket. It carries no display text, so each renderer supplies its own headings and links
- `GroupPRsByRepositoriesInGivenOrder(prs)` buckets the same way, but orders the buckets by each repository's first PR in the given list. Feeding it an already-sorted list puts the repository holding the leading PR first, whatever the sort was

## Doesn't Do

- Doesn't validate that mapped Slack user IDs are well-formed
- Doesn't handle a creation time in the future: the age text goes negative, e.g. "-30 minutes"

## Oddities

- Age and activity text are always plural and rounded to whole units, so a one-day-old PR reads "1 days" (and "idle 1 days") and a 23.6-hour-old PR reads "24 hours"
- A PR with a missing/zero creation timestamp counts as old whenever a threshold is set, whatever the threshold value
- An old-PR threshold of 0 turns the check off instead of flagging every PR as old
