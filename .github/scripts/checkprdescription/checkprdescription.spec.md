# checkprdescription

## Behaviour

- `make check-pr-description FILE=<path>` runs it on a PR description in a local file
  - Prints one line per problem and exits 1 if any, or 2 when it can't read the file. `make` itself exits 2 on any failure
- Reads CRLF line endings the same as LF: `gh pr view` returns a description edited on GitHub with CRLF
- A footer, from the first line that is exactly `---` to the end, is left out of every check except the MCP link length
- Checks the sections under `### The problem` and `### The solution`, each up to the next heading:
  - The heading's summary, its text after a colon: at most 8 words, and not the template's `<a few words` placeholder
  - A problem heading with no summary is reported. A solution heading may have none
  - Text after the heading that doesn't start with a colon is reported
  - Top-level bullets: 2 to 3 for the problem, 2 to 5 for the solution. Sub bullets don't count
  - Each bullet, sub bullets included: 1 to 15 words
  - Both sections' bullets together: at most 80 words
- Before the first heading, only a `Closes #<n>` or `Fixes #<n>` line
- Every other non-blank line is a list item:
  - Under `## Open`: a numbered item, `1. `, with bullet or numbered sub items
  - Elsewhere: a bullet, with bullet or numbered sub items
  - A sub item is indented with spaces or a tab
  - An empty `-` bullet, as the template leaves, is reported in any section
- Under `## Related`, up to the next heading: a line naming an issue or PR, as `#12` or an `/issues/12` or `/pull/12` URL, starts with `- Open `, `- Closed ` or `- Merged `
- With `-mcp` (`MCP=1`), for a description written with the GitHub MCP tools: the URL of every Markdown link in the whole description is at most 150 characters

## Doesn't Do

- Doesn't read the stored PR description. It checks the local file the agent writes before creating or editing the PR
- Word and bullet count limits outside the summary, such as in `## Docs`
- Whether a stated issue or PR state is true
- Whether a link resolves: `make check-pr-description` runs `checkreferences` on the file for that
- Whether a `## Docs` link points to the first heading the PR adds or changes
- Whether `## Docs` lists every `.md` file the PR changes

## Oddities

- A heading is found by prefix, so `### The problems: x` is taken as `### The problem`. The text after that prefix starts with `s`, not a colon, so it fails with `summary not after a colon`
- A template HTML comment left in is reported as prose
- A `---` line used as a rule mid-description takes everything below it as footer, unchecked
- Words are split on spaces, Markdown syntax included: `[a b](url)` is 2 words
- A link anchor of digits after `#`, such as `README.md#2-setup`, reads as an issue number under `## Related`
