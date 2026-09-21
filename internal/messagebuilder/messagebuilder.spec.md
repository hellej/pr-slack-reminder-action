# messagebuilder

Turns `messagecontent.Content` into a Slack message.

## Behaviour

- `BuildMessage(content)` returns the Slack message plus its summary text (used as Slack's plain-text fallback)
- The message has no title block. Its first block is `NoOpenPRsText` as a plain line when that is set, otherwise the heading of its first non-empty section
- Each non-empty section is two blocks: a `header` block at level 2 holding the heading, then one `rich_text` block holding its rows. An empty section renders neither block
- The headings are this package's own display text: `✅ Ready to merge`, `💬 Waiting for author`, `👀 Waiting for review`, `🚀 Recently merged`
- Grouped-by-repository case: inside that same rich_text block, an unbolded repository sub-heading carrying the repository link from [internal/messagecontent](../messagecontent/messagecontent.spec.md) and a bulleted list, per repository
- Nothing sits between rendered sections: a `header` block carries its own vertical padding
- An open PR row shows: title (linked), age (warning marker when [internal/prview](../prview/prview.spec.md) flagged the PR old, otherwise a plain "N ago"), author, approvers/commenters (marked distinctly, both shown together if both exist)
- A merged PR row shows: title (linked), when it merged in italics, author, approvers/commenters. No age, no old-PR marker: the section heading says it landed
- The age, merged and reviewer texts come from `prview`; this package supplies the surrounding spacing, the Block Kit styling and the old-PR marker
- The author renders as a Slack mention when a Slack user ID is mapped for them, otherwise by GitHub name; approvers and commenters always render by GitHub name
- The last block is always a `context` block reading `_Live, updated <!date^…|HH:MM UTC>_`, built from `Content.GeneratedAt`. Slack renders it in each reader's own timezone, 12-hour or 24-hour by their own client setting, and the fallback after the pipe carries UTC
- The message is capped at 50 blocks; content blocks past the cap are dropped and logged, and the footer keeps the last slot

## Doesn't Do

- Doesn't check any Slack limit other than block count, such as per-block text length or total payload size

## Oddities

- The block cap is unreachable in practice: the layout spends at most 10 blocks, whatever it lists, because a whole section's rows go in one block. Per-block text length is what a huge section would run into instead, and nothing checks it
- Truncation leaves no marker in the message: it is sent with its tail cut, and only a log line records it
- A message with nothing to list at all is the footer alone, or the no-open-PRs line above it when that is set. Neither is worth sending, and it is the caller that decides
