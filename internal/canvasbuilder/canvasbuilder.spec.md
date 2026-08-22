# canvasbuilder

Renders `canvascontent.Content` as the markdown of a Slack canvas, as one string.

## Behaviour

- `BuildMarkdown(content)` renders a fixed `## Open` heading and its list, then `## WIP`, then `## Merged`, each with its own list. All three headings always render
- Any of the three sections can render grouped: its PRs then get one `###` sub-heading per repository, the bare repository path linking to `models.Repository.GetPullsURL()`. The section heading above them already scopes the rows, so it is not repeated. Each section reads its own grouped slice, so all three group together, off the one `Content.GroupedByRepository` flag
- Open PR row: linked title, age text (`🚨` plus a code span past the old-PR threshold, italic otherwise), author, reviewers
- WIP PR row: linked title, author, commenters, the activity text as a code span, then `💤` when idle. Unknown activity renders neither the code span nor `💤`
- Merged PR row: linked title, the merge text in italics, author, then a trailing `🚀`. Never reviewers: the section answers what landed, not who reviewed it. An unknown merge time drops that segment only
- All three sections render through one path, differing only in heading, PR lists, row renderer and empty text, so a fourth section costs one call
- An empty section keeps its heading and shows one italic line: `_No open PRs_`, `_No work in progress_`, or `_No merged PRs_`. A merged section whose fetch failed shows `_Merged PRs could not be fetched_` instead, so a failure never reads as an empty week. Grouped mode with an empty section shows that section's line and no sub-headings
- Footer: a blank line, a line holding a lone non-breaking space, another blank line, a `---` divider, then `_Updated <YYYY-MM-DD HH:MM UTC>_` from `Content.GeneratedAt`. `GeneratedAt` is converted to UTC
- No cap note for the merged section: it is the newest 15 merges by definition, so a 16th is no surprise omission. A capped open or WIP fetch adds an italic line above the `Updated` line naming the fetch limit, with the counts read from `githubclient.MaxPRsToFetch` and `MaxDraftPRsToFetch`. Both caps are named in one line. It never claims how many rows the canvas shows: `canvascontent` prunes inactive drafts after the fetch
- Everything coming from GitHub (PR titles, author and reviewer names, repository paths) is backslash-escaped for `\`, `` ` ``, `*`, `_`, `[`, `]`, `~`, `<`, `>` and `&`, the backslash first so later replacements aren't double-escaped
- The rendered markdown is covered by golden files in `testdata/`, re-recorded with `make update-test-snapshots`

## Doesn't Do

- No top-level heading: the canvas title is its own field, set by a `rename` change on `canvases.edit`, so it survives a full content replace. Slack renders it as an H1 at the top of the document and doesn't dedupe a matching one from the body, so a body H1 would show the title twice
- No Slack mentions: authors and reviewers render through `GetGitHubName()`, never `SlackUserID`, so refreshing the canvas notifies nobody
- Never approvers on a WIP row, and never the old-PR `🚨` marker there: nobody has been asked to review a draft yet
- No strike-through, and no `🚀` outside the merged section: an open or WIP row can never carry a closed or merged PR
- Doesn't escape link targets: they come from GitHub and can't contain a space or a closing parenthesis
- Doesn't sort, group or filter, that is `canvascontent`'s job
- Doesn't limit the canvas size: an oversized canvas fails the write in `slackclient`

## Oddities

- Escaping is context-free, so characters that would be inert anyway (a spaced `*`, a mid-word `_`) still get a backslash. The escape survives as the literal character, so this is invisible in the rendered canvas
- Slack's canvas renderer collapses a truly empty markdown line, so the extra footer spacing above `---` uses a line holding one non-breaking space (`U+00A0`) instead of an empty string
