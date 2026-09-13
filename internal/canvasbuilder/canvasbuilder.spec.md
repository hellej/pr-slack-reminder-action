# canvasbuilder

Renders `canvascontent.Content` as the markdown of a Slack canvas, as one string.

## Behaviour

- `BuildMarkdown(content)` renders the open PRs as three sections, `## Ready to merge`, then `## Waiting for author`, then `## Waiting for review`, then `## WIP`, then `## Merged`, each with its own list
- An open section with no PRs renders nothing at all, heading included, so a reader scanning the headings sees only the buckets asking something of them. `## WIP` and `## Merged` always render
- All three open sections empty renders one `## Open` heading with `_No open PRs_`, the zero state the canvas had before the open split: a canvas opening at `## WIP` would read as a broken render
- Any section can render grouped: its PRs then get one `###` sub-heading per repository, the bare repository path linking to `models.Repository.GetPullsURL()`. The section heading above them already scopes the rows, so it is not repeated. Each section reads its own grouped slice, so all of them group together, off the one `Content.GroupedByRepository` flag. Nothing dedupes a repository across sections, so one repository can carry a `###` heading under each
- The three open sections share one row renderer, one empty-state line and one heading level: only the heading text and the PR list differ. A PR's bucket is `canvascontent`'s to decide
- Open PR row: linked title, age text (`🚨` plus a code span past the old-PR threshold, italic otherwise), author, reviewers
- WIP PR row: linked title, author, commenters, then the activity text, styled as a code span when `prparser.PR.IsRecentlyUpdated` and in italics otherwise. Unknown activity renders no activity segment at all
- Merged PR row: linked title, the merge text in italics, author, then a trailing `🚀`. Never reviewers: the section answers what landed, not who reviewed it. An unknown merge time drops that segment only
- Every section renders through one path, differing only in heading, PR lists, row renderer, empty text and whether it hides while empty, so a further section costs one call
- An empty section that does not hide keeps its heading and shows one italic line: `_No work in progress_` or `_No merged PRs_`, and `_No open PRs_` in the all-empty open case. A merged section whose fetch failed shows `_Merged PRs could not be fetched_` instead, so a failure never reads as an empty week. Grouped mode with such a section shows that section's line and no sub-headings
- Footer: a blank line, a line holding a lone non-breaking space, another blank line, a `---` divider, then `_Updated <YYYY-MM-DD HH:MM UTC>_` from `Content.GeneratedAt`. `GeneratedAt` is converted to UTC
- No cap note for the merged section, though open and WIP get one: it is always the newest 6 by design. A capped open or WIP fetch adds an italic line above the `Updated` line naming the fetch limit, with the counts read from `githubclient.MaxPRsToFetch` and `MaxDraftPRsToFetch`. Both caps are named in one line. It never claims how many rows the canvas shows: `canvascontent` prunes inactive drafts after the fetch
- Everything coming from GitHub (PR titles, author and reviewer names, repository paths) is backslash-escaped for `\`, `` ` ``, `*`, `_`, `[`, `]`, `~`, `<`, `>` and `&`, the backslash first so later replacements aren't double-escaped
- The rendered markdown is covered by golden files in `testdata/`, re-recorded with `make update-test-snapshots`

## Doesn't Do

- No top-level heading: the canvas title is its own field, set by a `rename` change on `canvases.edit`, so it survives a full content replace. Slack renders it as an H1 at the top of the document and doesn't dedupe a matching one from the body, so a body H1 would show the title twice
- No Slack mentions: authors and reviewers render through `GetGitHubName()`, never `SlackUserID`, so refreshing the canvas notifies nobody
- Never approvers on a WIP row, and never the old-PR `🚨` marker there: nobody has been asked to review a draft yet
- No strike-through, and no `🚀` outside the merged section: an open or WIP row can never carry a closed or merged PR
- Doesn't escape link targets: they come from GitHub and can't contain a space or a closing parenthesis
- Doesn't decide a PR's bucket, nor derive a heading from `prparser.PRNextAction`: the heading strings are this package's, and the bucketing is `canvascontent`'s
- Doesn't sort, group or filter, that is `canvascontent`'s job
- Doesn't limit the canvas size: an oversized canvas fails the write in `slackclient`

## Oddities

- Escaping is context-free, so characters that would be inert anyway (a spaced `*`, a mid-word `_`) still get a backslash. The escape survives as the literal character, so this is invisible in the rendered canvas
- Slack's canvas renderer collapses a truly empty markdown line, so the extra footer spacing above `---` uses a line holding one non-breaking space (`U+00A0`) instead of an empty string
