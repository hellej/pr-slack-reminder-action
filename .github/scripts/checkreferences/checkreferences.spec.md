# checkreferences

## Behaviour

- A command run from the repository root by `.github/scripts/check-style.sh`. It takes file paths as arguments and prints one finding per broken reference: `<path>:<line>: <reference>: <problem>`
- Exits 0 whether or not it finds anything: `check-style.sh` fails on any output. Exits 2 when it can't run, such as on a file it can't read, a missing `Makefile` or a failing `git ls-files`
- Checks the reference forms listed in [AGENTS.md](../../../AGENTS.md) § References
- Reads a file by its extension: `.md` as Markdown outside code blocks, `.go` as full-line `//` comments, anything else as full-line `#` comments
- Checks links, skill and agent names, and frontmatter `name:` in Markdown only. Checks `§` pointers, repository paths and make targets in every file
- Knows the repository's files from `git ls-files --cached --others --exclude-standard`: an untracked new file counts, a gitignored one doesn't
- Reads make targets from the root `Makefile`
- A `§` pointer passes when its text, with whitespace collapsed, starts with a heading or bold label of the target and ends there on a word boundary. A facts file heading's ` [YYYY-MM-DD]` tail is ignored
- A pointer's named file resolves next to the file holding the pointer first, then from the repository root
- A code comment's pointer naming no file is reported: a code file has no headings
- Matches a link's anchor against the target's headings as GitHub builds them. See docs/third-party-facts.md § GitHub builds a heading's anchor by lowercasing it, dropping punctuation and turning spaces into hyphens
- Accepts Claude Code's own agent types `Explore`, `general-purpose` and `Plan` as agent names

## Doesn't Do

- Doesn't check that a target says what its pointer claims. The `reviewer` agent does
- Doesn't check bare file names such as `run.go`, Go symbols named in prose, or external links
- Doesn't check repository paths in `docs/third-party-facts.md`, which quotes third-party paths
- Doesn't read trailing `//` comments or `/* */` blocks
- Doesn't check an anchor into a file that isn't Markdown
- Doesn't choose which files to scan: `check-style.sh` passes the list

## Oddities

- Any bold text in the target counts as a label, not only headings and rule names: `§ Purpose` passes against a bold "Purpose" anywhere in the file
- A section name has no end marker, so a pointer to a longer name passes when a label is its prefix: `§ Git Hooks` passes against a `Git` heading
- A pointer takes the file named right before the `§`, with only spaces, backticks or `**` between. Failing that, it takes the previous pointer's file on the same line, or else its own file. So a file named earlier in the sentence doesn't count, and a pointer with its file on the line before points into its own file
- A backticked token counts as a repository path only when it holds a `/` and its first part is a top-level entry git lists. A token holding whitespace, `...` or any of `<>*{}$"'[]()@:|=` is skipped as a placeholder, glob, command or URL
- Reads `Makefile` rule lines by pattern, so a target made through a variable or an `include` is missed
- Anchors approximate GitHub's punctuation list as "keep letters, digits, `-` and `_`"
- Parses code spans one line at a time: a span wrapping onto the next line isn't one
