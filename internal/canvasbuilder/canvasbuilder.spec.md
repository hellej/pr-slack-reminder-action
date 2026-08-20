# canvasbuilder

Renders `canvascontent.Content` as the markdown of a Slack canvas, as one string.

## Behaviour

- `BuildMarkdown(content)` renders a fixed `## Open PRs` heading and its list, then a fixed `## Work in Progress` heading and its list. Both headings always render
- Grouped open PRs get one `###` sub-heading per repository, linking to `models.Repository.GetPullsURL()`, with no "Open PRs in " prefix: the parent heading already says it
- Open PR row: linked title, age text (`🚨` plus a code span past the old-PR threshold, italic otherwise), author, reviewers
- WIP PR row: linked title, author, commenters, the activity text as a code span, then `💤` when idle. Unknown activity renders neither the code span nor `💤`
- Both sections render through one `renderSection`, differing only in row renderer and empty text, so a third section costs one call
- An empty section keeps its heading and shows one italic line, `_No open PRs_` or `_No work in progress_`. Grouped mode with no open PRs shows that same line and no sub-headings
- Footer: a blank line, a `---` divider, then `_Updated <YYYY-MM-DD HH:MM UTC>_` from `Content.GeneratedAt`. `GeneratedAt` is converted to UTC
- A capped fetch adds an italic line above the `Updated` line naming what was cut, with the counts read from `githubclient.MaxPRsToFetch` and `MaxDraftPRsToFetch`. Both caps are named in one line
- Everything coming from GitHub (PR titles, author and reviewer names, repository paths) is backslash-escaped for `\`, `` ` ``, `*`, `_`, `[`, `]`, `~`, `<`, `>` and `&`, the backslash first so later replacements aren't double-escaped
- The rendered markdown is covered by golden files in `testdata/`, re-recorded with `make update-test-snapshots`

## Doesn't Do

- No top-level heading: the canvas title lives on the canvas tab in Slack and survives a full content replace
- No Slack mentions: authors and reviewers render through `GetGitHubName()`, never `SlackUserID`, so refreshing the canvas notifies nobody
- Never approvers on a WIP row, and never the old-PR `🚨` marker there: nobody has been asked to review a draft yet
- No strike-through and no `🚀`: the canvas lists open PRs only, so a closed or merged PR can't reach a row
- Doesn't escape link targets: they come from GitHub and can't contain a space or a closing parenthesis
- Doesn't sort, group or filter, that is `canvascontent`'s job
- Doesn't limit the canvas size: an oversized canvas fails the write in `slackclient`

## Oddities

- Escaping is context-free, so characters that would be inert anyway (a spaced `*`, a mid-word `_`) still get a backslash. The escape survives as the literal character, so this is invisible in the rendered canvas
