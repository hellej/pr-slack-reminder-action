# checkprdescription

## Behaviour

- `make check-pr-description FILE=<path>` runs it on a PR description in a local file
  - Prints one line per problem and exits 1 if any. Exits 2 when it can't read the file
- Checks the sections under `### The problem` and `### The solution`, each up to the next heading:
  - The heading's text after a colon: at most 6 words, and not the template's `<a few words` placeholder
  - Every non-blank line is a bullet
  - Top-level bullets: 2 to 3 for the problem, 2 to 5 for the solution. Sub bullets don't count
  - Each bullet, sub bullets included: 1 to 15 words
  - Both sections' bullets together: at most 80 words
- Every Markdown link URL in the whole description: at most 150 characters

## Doesn't Do

- Reads the stored PR description. The agent checks its local file before creating or editing the PR
- Sections other than the summary's, such as `## Docs`
- Whether `## Docs` lists every `.md` file the PR changes

## Oddities

- A heading is found by prefix, so `### The problems` counts as `### The problem`
- An HTML comment line in a summary section is reported as not a bullet
- Words are split on spaces, Markdown syntax included: `[a b](url)` is 2 words
