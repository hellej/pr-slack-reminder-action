# messagebuilder

Turns `messagecontent.Content` into a Slack message, and marks a sent message stale.

## Behaviour

- `BuildMessage(content)` returns the Slack message plus its summary text (used as Slack's plain-text fallback)
- The message has no title block. Its first block is `NoOpenPRsText` as a plain line when that is set, otherwise the heading of its first non-empty section
- Each non-empty section opens with a `header` block at level 2 holding the heading, then the blocks of its rows: ungrouped, one `rich_text` block. An empty section renders no block at all
- The headings are this package's own display text: `✅ Ready to merge`, `💬 Waiting for author`, `👀 Waiting for review`, `🚀 Recently merged`
- Grouped-by-repository case: the section's rows come as one `rich_text` block per repository, in the order [internal/messagecontent](../messagecontent/messagecontent.spec.md) gives them. The block opens with the repository name in bold, linked to the repository's pulls page, then holds that repository's rows
- A spacing block, a `section` block of one blank space, sits between the repositories of a grouped section, never after its last one
- Nothing sits between rendered sections: a `header` block carries its own vertical padding
- An open PR row shows: title (linked), age (warning marker when [internal/prview](../prview/prview.spec.md) flagged the PR old, otherwise a plain "N ago"), author, approvers/commenters (marked distinctly, both shown together if both exist)
- A merged PR row shows: title (linked), when it merged in italics, author, approvers/commenters. No age, no old-PR marker: the section heading says it landed
- The age, merged and reviewer texts come from `prview`; this package supplies the surrounding spacing, the Block Kit styling and the old-PR marker
- The author renders as a Slack mention when a Slack user ID is mapped for them, otherwise by GitHub name; approvers and commenters always render by GitHub name
- The last block is always a `context` block reading `_Live, updated <!date^…|HH:MM UTC>_`, built from `Content.GeneratedAt`. Slack renders it in each reader's own timezone, 12-hour or 24-hour by their own client setting, and the fallback after the pipe carries UTC
- The message is capped at 50 blocks; content blocks past the cap are dropped and logged, and the footer keeps the last slot
- `BuildStaleMessage(sentBlocks, generatedAt)` rebuilds a sent message from its stored block array, every block but the last re-sent as stored, between two `context` blocks:
  - First, a stale line reading `_⚠️ Stale, updated <!date^…^{date_pretty} at {time}|Jan 2 15:04 UTC>_`
  - Last, in place of the live footer, a stale footer reading `_Updated <!date^…^{date_pretty} at {time}|Jan 2 15:04 UTC>_`
  - The stale message is one block longer than the stored one. Stored at the cap, it drops its last content block, logged, so it stays at 50
  - `{date_pretty}` reads `today` or `yesterday` when it applies, otherwise a date; the fallback after the pipe carries the date and time in UTC
  - Errors on blocks that do not parse, on an empty array or `null`, and on a block without a `type`

## Doesn't Do

- Doesn't check any Slack limit other than block count, such as per-block text length or total payload size
- `BuildStaleMessage` never re-renders content: rows, ages and headings stay as last sent
- `BuildStaleMessage` doesn't check that the block it drops is a footer. It relies on `BuildMessage` always putting the footer last

## Oddities

- Ungrouped, the block cap is out of reach: the layout spends at most 10 blocks whatever it lists, because a whole section's rows go in one block. Per-block text length is what a huge section would run into instead, and nothing checks it
- Grouped by repository, a section costs 2 × repositories blocks (a block per repository, a spacing block between each pair, and the section heading), so the cap is reachable: 6 repositories with PRs in all four sections is the most that fits, at 48 content blocks plus the footer, one slot short of the cap. 7 build 56 content blocks, of which 7 are dropped
- Truncation leaves no marker in the message: it is sent with its tail cut, and only a log line records it. The cut ignores where a section starts, so a section heading can be left with all of its repositories dropped, and the last block before the footer can be a spacing block
- A message with nothing to list at all is the footer alone, or the no-open-PRs line above it when that is set. Neither is worth sending, and it is the caller that decides
- Same-named repositories under different owners get identical sub-headings: only their link targets tell those groups apart
- `BuildStaleMessage` re-sends stored blocks compacted, whatever whitespace they were stored with, so an indented store sends the same JSON
