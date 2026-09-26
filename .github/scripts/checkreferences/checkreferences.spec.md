# checkreferences

## Behaviour

- `.github/scripts/check-style.sh` runs it from the repository root, passing the files to scan
  - Prints one line per broken reference: `<path>:<line>: <reference>: <problem>`
  - Exits 0 either way: `check-style.sh` fails on any output. Exits 2 when it can't run, such as on a file it can't read
- Section pointers, preferred in agent files, specs and code comments:
  - `AGENTS.md § Testing`: `AGENTS.md` has a heading or bold label that the text after `§` starts with, here "Testing"
  - `AGENTS.md § Git and § Testing`: the second pointer reuses the file
  - `` the `plan` skill § Structure ``: looks in `.agents/skills/plan/SKILL.md`. `` the `reviewer` agent § … `` looks in `.agents/agents/reviewer.md`
  - `§ Oddities` alone: a heading of the file holding it. In a code comment this fails, since a comment must name its file
  - `See docs/third-party-facts.md § <full heading>`: the heading's ` [YYYY-MM-DD]` tail is left out
  - A named file resolves next to the file holding the pointer, then from the repository root
- Links, preferred in docs read on GitHub, such as `README.md`:
  - `[spec](../state/state.spec.md)`: the file exists
  - `[tips](#-tips)` or `[tips](README.md#-tips)`: a heading with that GitHub anchor exists
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
- External links, and a pointer right after one: `[Limits](https://…) § Timeouts`
- References without `§`: `AGENTS.md **Code Style**`, "the plan skill's Structure"
- Bare file names: `run.go`, `chat.go`. Most name third-party files
- Placeholders, globs, commands and module paths: `internal/<pkg>/`, `internal/**/*.go`, `./...`, `slack-go@v0.29.0/chat.go`
- Paths not starting at a top-level entry of the repository: `testdata/`, `owner/repo`
- Go symbols named in prose, such as `WithLastWrittenMessage()`
- Repository paths in `docs/third-party-facts.md`, which quotes third-party paths
- Trailing `//` comments and `/* */` blocks
- Whether the target says what the pointer claims. The `reviewer` agent does that

## Oddities

- A section name has no end marker, so a label that starts the text is enough:
  - `§ Git Hooks` passes against a `## Git` heading
  - `§ Purpose` passes against a bold "Purpose" anywhere in the file
- A pointer's file comes from right before its `§`, with only spaces, backticks or `**` between:
  - `checks docs/facts.md files (see § References)` points into its own file
  - A file named at the end of the line before doesn't count
- A `§` with no file right before it reuses the file of the previous pointer on its line, however far back:
  - `AGENTS.md § Git. Then this file's § Local` looks for "Local" in `AGENTS.md`
  - After a skipped pointer into an external page, it is skipped too
- Reads `Makefile` rule lines by pattern: a target made through a variable or an `include` is missed
- Anchors approximate GitHub's rule as "keep letters, digits, `-` and `_`"
- A code span wrapping onto the next line isn't recognised as one
