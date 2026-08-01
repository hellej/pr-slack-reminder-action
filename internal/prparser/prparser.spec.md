# prparser

Enriches fetched PRs with display-ready metadata.

## Behaviour

- `ParsePRs(prs, contentInputs)` returns PRs enriched for display, sorted newest first (by creation time, ties broken by update time)
- Each collaborator (author, approvers, commenters) gets a Slack user ID attached when one is mapped for their GitHub login; unmapped users get an empty Slack ID
- A PR is flagged `IsOldPR` when an old-PR age threshold is configured and the PR is older than it
- PR age is rendered as days, hours, or minutes depending on magnitude
- Merged/closed-but-not-merged state is exposed for display styling

## Doesn't Do

- Doesn't validate that mapped Slack user IDs are well-formed
- Doesn't handle a PR with a creation time in the future — age rendering assumes it's in the past

## Oddities

- A PR with a missing/zero creation timestamp is always treated as "old", regardless of the configured threshold
- An old-PR threshold of 0 disables the old-PR check entirely, rather than flagging everything as old — 0 is not a usable threshold value
