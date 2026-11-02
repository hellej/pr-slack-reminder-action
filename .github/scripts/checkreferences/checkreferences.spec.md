# checkreferences

## Behaviour

- `.github/scripts/check-style.sh` runs it from the repository root, passing the files to scan
  - Prints one line per broken reference: `<path>:<line>: <reference>: <problem>`
  - Exits 0 either way: `check-style.sh` fails on any output. Exits 2 when it can't run, such as on a file it can't read
- `make check-pr-description` runs it with `-pr-description` on a PR description file:
  - Skips the repository path check: a description may name a path the PR deletes
  - Reports a blob URL into this repository on a branch other than `main` or the current branch, likely a typo. A blob URL on a commit SHA or a tag is not reported
  - Prints a note on stderr, without failing, when `origin` is missing or not a GitHub URL it recognises: blob URLs then go unchecked
- Section pointers, preferred in agent files, specs and code comments:
  - `AGENTS.md § Testing`: `AGENTS.md` has a heading or bold label that the text after `§` starts with, here "Testing", and the label ends at a separator or where the pointer's text ends
  - A separator after the name: `,` `;` `:` `)` `?` `!`, a `.` before a space or the end, `**`, `'s`, ` (`, or ` §`, ` and §`, ` or §` for chained pointers. `§ Git hooks run` fails against `## Git`. Bold the pointer to end it mid-sentence: `**AGENTS.md § Git** runs first`
  - A pointer's text runs to the blank line, so a name ending its line ends there too, however the next line goes on
  - `AGENTS.md § Git and § Testing`: the second pointer reuses the file
  - `` the `plan` skill § Structure ``: looks in `.agents/skills/plan/SKILL.md`. `` the `reviewer` agent § … `` looks in `.agents/agents/reviewer.md`
  - `§ Oddities` alone: a heading of the file holding it. In a code comment this fails, since a comment must name its file
  - `See docs/third-party-facts.md § <full heading>`: the heading's ` [YYYY-MM-DD]` tail is left out
  - `[AGENTS.md](AGENTS.md) § Testing`: fails when the name matches a heading, which a link can reach: `[AGENTS.md § Testing](AGENTS.md#testing)`. Passes when it matches only bold text, which has no anchor
  - A named file resolves next to the file holding the pointer, then from the repository root
- Links, preferred in docs read on GitHub, such as `README.md`:
  - `[spec](../state/state.spec.md)`: the file exists
  - `[AGENTS.md § Testing](AGENTS.md#testing)`: a heading with that GitHub anchor exists, and the text is `<file name> § <heading>`, with the heading's ` [YYYY-MM-DD]` tail left out. A heading of the file holding the link reads `[§ Oddities](#oddities)`
  - Free text such as `[tips](#-tips)` is allowed in `README.md`, but not text with a `§` that isn't in that form. Images and blob URLs are free too
  - A blob URL into this repository on `main` or the current branch, such as `https://github.com/<owner>/<repo>/blob/main/README.md#-tips`: checked the same way, its path read from the repository root in the working tree. `<owner>/<repo>` comes from the `origin` remote: `git@github.com:`, `ssh://git@github.com/` or `https://github.com/`, with or without credentials before the host
- Repository paths: `` `internal/state/` `` exists in the files git lists. An untracked new file counts, a gitignored one doesn't
- Skill and agent names:
  - `` the `coding` skill ``, `` the `plan` and `writing` skills ``, `` the `reviewer` agent ``: each exists under `.agents/`
  - `skills: [coding, writing]` frontmatter: each skill exists
  - Claude Code's own `Explore`, `general-purpose` and `Plan` agent types count as existing
  - A skill's or agent's frontmatter `name:` matches its folder or file name
- Make targets: `` `make check-style` `` is a rule in the root `Makefile`. Flags and `VAR=value` arguments are ignored
- Where it looks:
  - Markdown: everything outside code blocks
  - Go: full-line `//` comments
  - Any other file: full-line `#` comments
  - Links and skill and agent names: Markdown only

## Doesn't Do

- A link or `§` pointer in a code span or code block: an example, such as `` `See docs/third-party-facts.md § <heading>` ``
- External links other than the blob URLs above
- A pointer right after an external link: `[Limits](https://…) § Timeouts`
- References without `§`: `AGENTS.md **Code Style**`, "the plan skill's Structure"
- Bare file names: `run.go`, `chat.go`. Most name third-party files
- Placeholders, globs, commands and module paths: `internal/<pkg>/`, `internal/**/*.go`, `./...`, `slack-go@v0.29.0/chat.go`
- Paths not starting at a top-level entry of the repository: `testdata/`, `owner/repo`
- Go symbols named in prose, such as `WithLastWrittenMessage()`
- Repository paths in `docs/third-party-facts.md`, which quotes third-party paths
- Trailing `//` comments and `/* */` blocks
- Whether the target says what the pointer claims. The `reviewer` agent does that

## Oddities

- A label only has to start the text, up to a separator:
  - `§ Purpose` passes against a bold "Purpose" anywhere in the file
  - `§ Git, hooks` passes against a `## Git` heading
  - A name wrapped onto a second line passes when its first line ends in a heading: `§ Git` then `hooks` on the next line
- A pointer's file comes from right before its `§`, with only spaces, backticks or `**` between:
  - `checks docs/facts.md files (see § References)` points into its own file
  - A file named at the end of the line before doesn't count
- A `§` with no file right before it reuses the file of the previous pointer on its line, however far back:
  - `AGENTS.md § Git. Then this file's § Local` looks for "Local" in `AGENTS.md`
  - After a skipped pointer into an external page, it is skipped too
- Reads `Makefile` rule lines by pattern: a target made through a variable or an `include` is missed
- Outside `-pr-description`, a blob URL on any branch other than `main` and the current one is skipped as external: a branch name may hold slashes, so the path can't be found without knowing the branch. In CI, on a detached HEAD, only `main` counts
- A blob URL is checked against the working tree, not the branch it names
- In `-pr-description` mode, a blob URL's segment after `blob/` of 7 to 40 lowercase hex characters counts as a commit SHA, and one starting with `v` and a digit as a tag. Neither is reported, so a branch named like either never is. A tag not starting with `v` is reported as on another branch
- Anchors approximate GitHub's rule as "keep letters, digits, `-` and `_`"
- A code span wrapping onto the next line isn't recognised as one
