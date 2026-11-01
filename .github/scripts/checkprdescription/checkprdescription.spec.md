# checkprdescription

## Behaviour

- `make check-pr-description FILE=<path>` runs it on a PR description in a local file
  - Prints one line per problem and exits 1 if any. Exits 2 when it can't read the file
- Checks the sections under `### The problem` and `### The solution`, each up to the next heading:
  - The heading's text after a colon: at most 8 words, and not the template's `<a few words` placeholder
  - Top-level bullets: 2 to 3 for the problem, 2 to 5 for the solution. Sub bullets don't count
  - Each bullet, sub bullets included: 1 to 15 words
  - Both sections' bullets together: at most 80 words
- Every non-blank line is a list item:
  - Before the first heading: only a `Closes #<n>` or `Fixes #<n>` line
  - Under `## Open`: a numbered item, `1. `, with bullet or numbered sub items
  - Elsewhere: a bullet, with bullet or numbered sub items
- Under `## Related`, up to the next heading: a line naming an issue or PR, as `#12` or an `/issues/12` or `/pull/12` URL, starts with `- Open `, `- Closed ` or `- Merged `
- With `-mcp` (`MCP=1`), for a description written with the GitHub MCP tools: every Markdown link URL in the whole description is at most 150 characters

## Doesn't Do

- Reads the stored PR description. The agent checks its local file before creating or editing the PR
- Sections other than the summary's, such as `## Docs`
- Whether a stated issue or PR state is true
- Whether a link resolves: `make check-pr-description` runs `checkreferences` on the file for that
- Whether a `## Docs` link points to the first heading the PR adds or changes
- Whether `## Docs` lists every `.md` file the PR changes

## Oddities

- A heading is found by prefix, so `### The problems` counts as `### The problem`
- A template HTML comment left in is reported as prose
- Words are split on spaces, Markdown syntax included: `[a b](url)` is 2 words
- A link anchor of digits after `#`, such as `README.md#2-setup`, reads as an issue number under `## Related`
