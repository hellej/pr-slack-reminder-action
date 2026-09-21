# messagebuilder

Turns `messagecontent.Content` into a Slack message.

## Behaviour

- `BuildMessage(content)` returns the Slack message plus its summary text (used as Slack's plain-text fallback)
- The message has no title block. Its first block is `NoOpenPRsText` as a plain line when that is set, otherwise the heading of its first non-empty section
- Each non-empty section opens with a `header` block at level 2 holding the heading, then the blocks of its rows: ungrouped, one `rich_text` block. An empty section renders no block at all
- The headings are this package's own display text: `✅ Ready to merge`, `💬 Waiting for author`, `👀 Waiting for review`, `🚀 Recently merged`
- Grouped-by-repository case: the section's rows come as a pair of blocks per repository, in the order [internal/messagecontent](../messagecontent/messagecontent.spec.md) gives them: a `header` block at level 3 holding the repository path, then a `rich_text` block of that repository's rows. A `header` block's text is `plain_text`, so the repository path carries no link
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

- Ungrouped, the block cap is out of reach: the layout spends at most 10 blocks whatever it lists, because a whole section's rows go in one block. Per-block text length is what a huge section would run into instead, and nothing checks it
- Grouped by repository, a section costs 1 + 2 × repositories blocks, so the cap is reachable: 6 repositories with PRs in all four sections spend 52, and 3 blocks are dropped
- Truncation leaves no marker in the message: it is sent with its tail cut, and only a log line records it. The cut ignores the heading and rows pairs, so a repository heading can be left with its rows dropped
- A message with nothing to list at all is the footer alone, or the no-open-PRs line above it when that is set. Neither is worth sending, and it is the caller that decides
