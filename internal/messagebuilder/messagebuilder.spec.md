# messagebuilder

Turns `messagecontent.Content` into a Slack message, and marks a sent message stale.

## Behaviour

- `BuildMessageToPost(content)` returns a message to post plus its summary text (used as Slack's plain-text fallback). It has no footer
- `BuildMessageWithUpdateTimeFooter(content)` returns the same message ending with an update-time footer, for an edit in place
- The message has no title block. Its first block is `NoOpenPRsText` as a plain line when that is set, otherwise the heading of its first non-empty section
- Each non-empty section opens with a `header` block at level 2 holding the heading, then the blocks of its rows: ungrouped, one `rich_text` block. An empty section renders no block at all
- Sections render top to bottom in this order, under this package's own heading text: `👀 Waiting for review`, `💬 Waiting for author`, `✅ Ready to merge`, `🚀 Recently merged`. The open sections match the canvas's order ([internal/canvasbuilder](../canvasbuilder/canvasbuilder.spec.md))
- Grouped-by-repository case: the section's rows come as two blocks per repository, in the order [internal/messagecontent](../messagecontent/messagecontent.spec.md) gives them, with nothing between repositories:
  - A sub-heading: a `context` block of one mrkdwn element `*<url|name>*`, the repository name without its owner, linked to `models.Repository.GetPullsURL()`
  - A `rich_text` block holding only that repository's bullet list
- Nothing sits between rendered sections: a `header` block carries its own vertical padding
- An open PR row shows: title (linked), age (warning marker when [internal/prview](../prview/prview.spec.md) flagged the PR old, otherwise a plain "N ago"), author, approvers/commenters (marked distinctly, both shown together if both exist)
- A merged PR row shows: title (linked), when it merged in italics, author, approvers/commenters. No age, no old-PR marker: the section heading says it landed
- A collapsed row (`prview.CollapsedRow`), in any section, is one more bullet in the section's list: a bold link `🤖 <label>:`, the label from `GetAuthorLabel()`, linking to `GetSearchURL()`; then each PR number as a bold `#<n>` link to its PR. A plain `" "` run separates each element. No age, author, reviewers, old-PR marker or merge time
- Every section renders its rows in the given order: sorting, grouping and collapsing are [internal/messagecontent](../messagecontent/messagecontent.spec.md)'s
- The age, merged and reviewer texts come from `prview`; this package supplies the surrounding spacing, the Block Kit styling and the old-PR marker
- The author renders as a Slack mention when a Slack user ID is mapped for them, otherwise by GitHub name; approvers and commenters always render by GitHub name
- When `Content.MergeStats` is set, both messages render the merge stats line after the sections, before the update-time footer: a `context` block with block ID `merge_stats_line`, one plain `mrkdwn` element
  - `Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge`
  - The window reads in whole days, `ComputedAt` minus `MergedSince` rounded to the nearest day, at least 1: `Last 1 day`, `Last 3 days`, `Last 7 days`
  - A nil median drops its part: `Last 7 days: 2 merged · median 1.0 days to merge`, and with both nil, `Last 7 days: 0 merged`
  - A duration reads in whole minutes under an hour, in hours with one decimal under 24 hours, else in days with one decimal: `24 minutes`, `1 minute`, `4.0 hours`, `1.0 days`
- The update-time footer is a `context` block with block ID `update_time_footer`, reading `_Live, updated <!date^…|HH:MM UTC>_`, built from `Content.GeneratedAt`. Slack renders it in each reader's own timezone, 12-hour or 24-hour by their own client setting, and the fallback after the pipe carries UTC
- Content is capped at 48 blocks, dropped past the cap and logged, in both messages, with or without the merge stats line. The 2 slots left hold the merge stats line when set, and the update-time footer of an edited message or the staleness warning of a posted message marked stale
- `BuildMessageMarkedStale(sentBlocks, generatedAt)` rebuilds a sent message from its stored block array:
  - First, a staleness warning, a `context` block reading `_⚠️ Stale, updated <!date^…^{date_pretty} at {time}|Jan 2 15:04 UTC>_`
  - Then every stored block except the one with block ID `update_time_footer`, re-sent as stored, the merge stats line included. A posted message never had that footer, an edited one did. No footer replaces it
  - At most 50 blocks: the stored content was capped at 48, plus the line
  - `{date_pretty}` reads `Today` or `Yesterday`, capitalised even mid-sentence, when it applies, otherwise a date; the fallback after the pipe carries the date and time in UTC
  - Errors on blocks that do not parse, on an empty array or `null`, and on a content block without a `type`

## Doesn't Do

- Doesn't check any Slack limit other than block count, such as per-block text length or total payload size
- `BuildMessageMarkedStale` never re-renders content: rows, ages and headings stay as last sent
- `BuildMessageMarkedStale` applies no block cap. It relies on `BuildMessageToPost` and `BuildMessageWithUpdateTimeFooter` having capped the stored content at 48 blocks

## Oddities

- Ungrouped, the block cap is out of reach: the layout spends at most 10 blocks whatever it lists, because a whole section's rows go in one block. Per-block text length is what a huge section would run into instead, and nothing checks it
- Grouped by repository, a section costs 1 + 2 × repositories blocks (the section heading, then a sub-heading and a list per repository), so the cap is reachable: 5 repositories with PRs in all four sections is the most that fits, at 44 content blocks. 6 build 52 content blocks, of which 4 are dropped
- Truncation leaves no marker in the message: it is sent with its tail cut, and only a log line records it. The cut ignores where a section starts, so a section heading can be left with all of its repositories dropped, and the last content block can be a repository sub-heading with its list dropped
- The cap keeps a slot for the merge stats line on every day, so a grouped message without the line drops a content block it had room for
- A window under 12 hours rounds to 0 days and reads `Last 1 day`, as does any window from 12 to 36 hours
- The line's rounding can read one unit past its switch: 59.6 minutes reads `60 minutes`, and 23.96 hours `24.0 hours`
- A message with nothing to list at all is empty, or holds only the no-open-PRs line and the merge stats line, whichever are set, with the update-time footer after them in an edit. None is worth sending, and it is the caller that decides
- Same-named repositories under different owners get identical sub-headings: only their link targets tell those groups apart
- Ungrouped, a collapsed row can span repositories, and its numbers carry no repository prefix: the same number can show twice in one row, and only the link targets tell those PRs apart
- `BuildMessageMarkedStale` re-sends stored blocks compacted, whatever whitespace they were stored with, so an indented store sends the same JSON
