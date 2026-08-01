# messagebuilder

Turns `messagecontent.Content` into a Slack message.

## Behaviour

- `BuildMessage(content)` returns the Slack message plus its summary text (used as Slack's plain-text fallback)
- No-PRs case: renders `content.SummaryText` only
- Ungrouped case: one heading followed by one bulleted list of all PRs
- Grouped-by-repository case: per repository, a heading linking to that repo's GitHub pulls page, followed by its bulleted PR list, with visual spacing between repositories
- Each PR entry shows: title (linked, struck through if closed-but-not-merged), age (flagged with a warning marker if older than the configured threshold, otherwise a plain "N ago"), author (Slack mention if a Slack user ID is mapped for them, else their GitHub name), approvers/commenters (marked distinctly, both shown together if both exist), and a rocket marker if merged
- The message is capped at `maximumBlocksInSlackMessage` (50) content blocks; anything beyond that is silently dropped

## Doesn't Do

- Doesn't split content across multiple Slack messages when it exceeds the block cap — excess content is dropped, not deferred
- Doesn't validate that mapped Slack user IDs are real Slack user IDs

## Oddities

- The block cap effectively bounds repository count, not PR count: in grouped mode each repository costs multiple blocks, but in ungrouped mode all PRs share a single list block — a large PR count alone will not hit the cap, only a large repository count in grouped mode will
- When content is dropped for exceeding the cap, only a log line is emitted — the message is still sent, truncated, with no indication in the message itself that anything was cut
