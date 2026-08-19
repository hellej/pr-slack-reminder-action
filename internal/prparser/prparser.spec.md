# prparser

Enriches fetched PRs with display-ready metadata.

## Behaviour

- `ParsePRs(prs, contentInputs)` returns PRs enriched for display, sorted oldest first (by creation time, ties broken by update time)
- Each collaborator (author, approvers, commenters) gets a Slack user ID attached when one is mapped for their GitHub login; unmapped users get an empty Slack ID
- A PR is flagged `IsOldPR` when an old-PR age threshold is configured and the PR is older than it
- `GetPRAgeText` renders age as days, hours, or minutes depending on magnitude
- `IsMerged` and `IsClosedButNotMerged` expose PR state for display styling

## Doesn't Do

- Doesn't validate that mapped Slack user IDs are well-formed
- Doesn't handle a creation time in the future: the age text goes negative, e.g. "-30 minutes"

## Oddities

- Age text is always plural and rounded to whole units, so a one-day-old PR reads "1 days" and a 23.6-hour-old PR reads "24 hours"
- A PR with a missing/zero creation timestamp counts as old whenever a threshold is set, whatever the threshold value
- An old-PR threshold of 0 turns the check off instead of flagging every PR as old
