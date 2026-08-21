# messagebuilder

Turns `messagecontent.Content` into a Slack message.

## Behaviour

- `BuildMessage(content)` returns the Slack message plus its summary text (used as Slack's plain-text fallback)
- No-PRs case: renders `content.SummaryText` only
- Ungrouped case: one heading followed by one bulleted list of all PRs
- Grouped-by-repository case: per repository, a heading carrying the repository link from [internal/messagecontent](../messagecontent/messagecontent.spec.md), then its bulleted PR list, with a spacing block between repositories
- Each PR entry shows: title (linked, struck through if closed-but-not-merged), age (warning marker when [internal/prparser](../prparser/prparser.spec.md) flagged the PR old, otherwise a plain "N ago"), author, approvers/commenters (marked distinctly, both shown together if both exist), and a rocket marker if merged
- The age and reviewer texts come from `prparser`; this package supplies the surrounding spacing, the Block Kit styling, and the old-PR, merged and closed-but-not-merged markers
- The author renders as a Slack mention when a Slack user ID is mapped for them, otherwise by GitHub name; approvers and commenters always render by GitHub name
- The message is capped at 50 content blocks; blocks past the cap are dropped and logged
- When the content carries a canvas URL, a context block linking to the PR tracker canvas is appended as the message's last block, after the cap is applied, and the cap drops to 48 to reserve room for it (a grouped repository costs 3 blocks, so cutting at 49 would leave a heading without its list)
- The no-PRs message gets the same footer, without any capping (two blocks)

## Doesn't Do

- Doesn't check any Slack limit other than block count, such as per-block text length or total payload size

## Oddities

- The block cap bounds repository count, not PR count: grouped mode spends 3 blocks per repository, ungrouped mode puts every PR in one list block
- Truncation leaves no marker in the message: it is sent with its tail cut, and only a log line records it
