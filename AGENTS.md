# AGENTS.md

GitHub Action written in Go that fetches open PRs from GitHub repositories and sends or updates a Slack reminder listing them.

## Purpose

Improve a software team's development velocity and flow.

- Listing PRs so they get reviewed and merged is the current means, not the goal
- Anything else that removes friction from the team's flow is in scope
- Favour signal over noise: every notification interrupts the team, so it must earn the interruption
- When planning or implementing, weigh changes against this goal, and propose new ideas serving it

## Reference Deployment

One known setup as an example. It's not the only supported one. Use it to weigh a change against § Purpose, never to drop support for setups it doesn't cover.

- One team, one Slack channel, no one outside the team in it. Every member both authors and reviews PRs
- A monorepo the team owns, plus PRs the team has open in repos owned by others
- 0 to 8 open PRs at a time. A weekly Dependabot batch adds ~5 at once
- A draft PR with recent activity is a real WIP signal. A quiet one is just left open
- Human approval gates merges: every PR needs an approving review from a human other than its author, bot-authored PRs included
- Bot accounts post review comments on every commit, so bot activity never counts as human review and never moves a PR out of "waiting for review"
- A daily scheduled `post` at 09:00, plus event-driven `update` runs on PR, review, comment and push events
- The PR tracker canvas runs alongside the message as the persistent live view

## Output Style

Applies to all agent output: chat answers, docstrings, plans, and text written to project files (docs, plans, skills, AGENTS.md).

- Use plain, simple words. Keep answers short and direct
- Prefer bullet points over prose
- Prefer short bullet points over long ones
- Avoid filler words
- Avoid duplication and overlap with what's already said or written
- Avoid dashes as punctuation. Use a comma, a colon, or a new sentence

### Examples

Don't add trailing justification for an obvious rule:

- ✗ `Return an error instead of calling os.Exit, so callers can decide what to do.`
- ✓ `Return an error instead of calling os.Exit.`

Don't restate the rule as its own reason (circular justification):

- ✗ `The mock goes in testhelpers/ rather than the package under test, because testhelpers/ is where shared mocks live.`
- ✓ `The mock goes in testhelpers/.`

Don't let a sentence's second half restate its first:

- ✗ `Evidence from this run only, and nothing that would fit any run.`
- ✓ `Evidence from this run only.`

Don't frame before saying the thing (meta framing):

- ✗ `One thing worth calling out before the details: the 2-month cutoff is hardcoded.`
- ✓ `The 2-month cutoff is hardcoded.`

Don't narrate how you found the answer, when the route doesn't change how much to trust it:

- ✗ `I went through the config package and checked each call site, and can confirm the input is unused.`
- ✓ `The input is unused.`

Do name the source when it does change how much to trust it, third-party APIs above all:

- ✗ `EditCanvas takes a CanvasID and a list of changes.`
- ✓ `EditCanvas takes a CanvasID and a list of changes (slack-go v0.27.0 source).`

Don't stack hedges:

- ✗ `This should probably work in most cases, though it may be worth verifying.`
- ✓ `Unverified: whether Slack rejects payloads over the 50-block limit.`

Don't open a sentence with a modifier that describes something other than its subject:

- ✗ `Grouped by repository, each section carries a sub-heading.`
- ✓ `When grouped by repository, each section carries a sub-heading.`

Don't use a pronoun when an earlier noun in the same sentence could equally be its antecedent:

- ✗ `...every PR in the tracked set, plus the newest 3 entries of the fetch that are not already in it`
- ✓ `...every PR in the tracked set, plus the newest 3 entries of the fetch not already in that set`

## Releasing

- Release procedure: [.agents/skills/release/SKILL.md](.agents/skills/release/SKILL.md)

## Package Specs

- Each Go package under `internal/` has a `<package>.spec.md` describing its current behaviour, non-goals, and oddities. Read it before reading the package's source, plus any related package's spec needed to understand how a change fits
- `cmd/pr-slack-reminder` has one too, [run.spec.md](cmd/pr-slack-reminder/run.spec.md), covering the run orchestration in `run.go`, `canvas.go` and `annotations.go`
- Writing/updating procedure: [.agents/skills/spec-writer/SKILL.md](.agents/skills/spec-writer/SKILL.md)
- Update a package's spec file whenever its behaviour changes, in the same change
  - A rough edge you knowingly leave, because the fix would need significant complexity for a rare case, goes in the spec's **Oddities** section
- `make check-style` fails when a package directory has no spec file
- A `git commit` with staged `internal/**/*.go` or `cmd/pr-slack-reminder/**/*.go` changes but no staged spec update triggers a non-blocking reminder (`.claude/hooks/check-spec-sync.sh`): safe to proceed if the change was a pure refactor

## Third-party Facts

- [docs/third-party-facts.md](docs/third-party-facts.md) records what past work confirmed about external APIs and libraries, each entry with its source
- Grep its `##` headings before verifying such a claim yourself. Each heading carries the whole claim, so read a body only when it bears on your work
- Add to it whenever you confirm such a fact, or rule an approach out
  - File only what outlives the task: a method's shape, a scope, a limit. Dead ends above all: nothing else records them
- Plans and code may cite an entry by its heading, e.g. `See docs/third-party-facts.md § <heading>`, or the source it names

## Third-party Tools

- Pin every third-party tool and action to an immutable reference. Never `@latest` or a moving tag
  - GitHub Actions: the full commit SHA, with the version in a trailing comment
  - Go dev tools: `tool` directives in `tools/go.mod`, a module apart from the action binary's `go.mod`. `tools/go.sum` hashes lock the versions
  - Add one with `go get -modfile=tools/go.mod -tool <package>@<version>`, run it with `go tool -modfile=tools/go.mod <tool>` (`GO_TOOL` in the Makefile)
- Dependabot updates the pins weekly. A new Go tool also needs an `allow` entry in `.github/dependabot.yml`. See docs/third-party-facts.md § Dependabot skips Go tool directive modules unless allow-listed

## Git

- Never amend commits or force push
- Work on `main` by default. Branch only if the user asks for a branch or mentions a PR
- Stage only the files for the task you were given. Another agent may have unrelated work
  in the same working tree, so never use `git add -A` or `git commit -a`

## Code Style

- **Readability > Speed:** Data sets are tiny; never trade clarity for execution speed or micro-optimizations.
- **KISS, YAGNI, & Avoid Hasty Abstractions (AHA):** Implement only what is required right now. Prefer concrete types and minor duplication over speculative wrappers, single-use interfaces, or premature helpers.
- **Intent-driven naming over comments:** Names must reveal *why* a variable or function exists (e.g., `activeSubscribers` over `filteredUsers`). If code feels complex enough to need a comment, refactor and/or rename instead. A long descriptive name is better than a short enigmatic name. A long descriptive name is better than a long descriptive comment.
  - Name a UI element by what it shows: `updateTimeFooter`, not `liveFooter`. Don't put an adjective before a noun it doesn't describe: the edit marking a message stale is a `markAsStaleEdit`, not a `staleEdit`
  - Short-lived names may be short: receivers, loop variables, a value used within a few lines. Maps still follow `<value>By<key>`
- **A comment must state something the code cannot:** an external fact earns its place, such as an API's behaviour, a measured limit, or why a decision went one way. A comment that restates what the code says means the code needs a better name. A comment decoding an expression, a double negative above all, means the expression should be written the other way round.
  - When the fact is already in the package's `.spec.md`, point to it instead of repeating it: at most one short line of the fact, then the pointer, e.g. `// An artifact saved before this field decodes an empty hash. See state.spec.md § Oddities`
- **Declarative slice transformations:** Prefer `./internal/utilities` (`Map`, `Filter`, `Find` etc), `slices` and `maps` over manual `for` loops and index management when transforming data. Extend `utilities` when no helper fits. A plain loop is fine where a helper reads worse, such as one building several values at once.
- **Pure functions:** Prefer pure, side-effect-free functions. Return new slices or structs rather than mutating input pointers or package-level state.
- **Flat structure:** Use early returns and guard clauses. Do not nest `if` blocks deeper than 2 levels.
- **Keep exported type names exported:** Don't unexport a type just to shrink a package's API surface. Unexporting renames it, and lowercase type names read worse here. Funcs and consts are fine to unexport.
- **Name a map `<value>By<key>`:** e.g. `isTrackedByPRRef` for a `map[PullRequestRef]bool`.

## Testing

- **Always use TDD**: write failing tests first, implement minimal code to pass, then refactor. No exceptions for small changes
  - Run the new test and confirm it fails for the expected reason before implementing
  - When a snapshot or golden file already covers the change's visible effect, that file is the test. Change the code, read the failing diff to confirm it is what you meant, then re-record. Add a separate assertion only for a mutant that fails it but passes the goldens
- Use table-driven tests for functions with multiple input scenarios
- Coverage counts across the whole suite, not per package
  - Snapshot anything a user sees in Slack or on the canvas, whenever feasible
  - Use `main_test.go` integration tests for behaviour a snapshot can't show, such as what gets saved, skipped or failed
  - Add a package test only for what neither reaches cheaply, such as limits and error mapping
  - Don't repeat a case a broader test already pins, so internals stay free to refactor
- Pick fixture values a wrong implementation would get wrong: `len(prs) == MaxDraftPRsToFetch` passes whatever that constant becomes, and input already in the expected order can't tell "kept" from "sorted". Reusing test-owned input in an assertion is fine
  - An expectation derived from the value under test asserts nothing. Pin the value literally, and feed the boundary itself
- Check for existing helpers in `testhelpers/` before creating new ones
- `cmd/pr-slack-reminder/main_test.go`: integration tests using full pipeline with mocks
- `testhelpers/confighelpers.go`: `TestConfig` struct and `SetTestEnvironment()` for consistent test setup
- `testhelpers/mockgithubclient/` and `testhelpers/mockslackclient/`: injectable mock dependencies

## Development Commands

- `make test`: run all tests, including `.github/scripts/`
- `make test-with-coverage`: run tests with coverage report (clears cache first)
- `make update-test-snapshots`: re-record the Slack payload and saved state snapshots in `cmd/pr-slack-reminder/testdata/snapshots/` and the canvas markdown in `internal/canvasbuilder/testdata/`
- `make run`: run locally (requires env vars, see Makefile for the pattern)
- `make build`: build linux binaries
- `gh workflow run pr-reminder.yml --ref <branch> -f run-mode=post -f build-first=true`: try a branch's own code against the real Slack workspace, a dev channel, so WIP work is safe to run. Without `build-first` the job runs the committed `dist/` binary that `invoke-binary.js` pins by version, so it goes green without ever executing the change
- `make check-fmt`: fail if any file needs `gofmt`
- `make check-vet`: run `go vet` over `./...` and `.github/scripts/`
- `make check-dead-code`: fail if `deadcode` finds an unreachable function under `./cmd/...`
- `make check-vulnerabilities`: run `govulncheck ./...`
- `make check-style`: fail on a dash used as punctuation in `AGENTS.md`, agent skills and agents, spec files, `README.md`, `docs/third-party-facts.md` and Go comments, a relative link in those Markdown files to a missing file, a `<file>.md § <section>` pointer in them or in a Go comment whose section is no heading or bold text of that file (`.github/scripts/checkreferences/`), a map not named `<value>By<Key>`, or a package missing its spec
- `make install-hooks`: point git at `githooks/`, a pre-commit hook running `check-fmt`, `check-vet`, `check-style` and `check_inputs.go`. One-time opt-in per clone
- Claude Code web sessions run `make install-hooks` at start (`.claude/hooks/session-start.sh`)
- `go run .github/scripts/check_inputs.go`: validate action.yml and config.go constants are in sync
- Go LSP (gopls), when available, is reachable via the LSP tool. Leverage it for finding real references or definitions of a Go symbol, especially short or common names, since grep also matches comments and strings

## Architecture

Two run modes (`run-mode` input): **post** sends a new reminder, marks the previous one stale, and saves state; **update** lists the PRs open right now, re-fetches the state's PRs for the merged section, and edits or deletes the existing message.

Shared input, in order:

1. **Config** (`internal/config/`): parses GitHub Action inputs via `INPUT_` prefix env vars
2. **GitHub Client** (`internal/apiclients/githubclient/`): fetches PR data and reviews, applies filtering
3. **PR View** (`internal/prview/`): enriches PRs with Slack user mappings and display metadata

Then two tracks, the message first. A failure in one doesn't skip the other:

- Message track:
  1. **Message Content** (`internal/messagecontent/`): structures data for messaging
  2. **Message Builder** (`internal/messagebuilder/`): constructs Slack Block Kit messages
  3. **Slack Client** (`internal/apiclients/slackclient/`): sends, updates, or deletes messages
- Canvas track:
  1. **Canvas Content** (`internal/canvascontent/`): structures open, draft and merged PRs into the PR tracker canvas sections
  2. **Canvas Builder** (`internal/canvasbuilder/`): renders the canvas content as markdown
  3. **Canvas Refresh** (`cmd/pr-slack-reminder/canvas.go`): writes the markdown to the canvas when configured, skipping the write when it is unchanged since the last run

Around the run:

- **State** (`internal/state/`): loaded from a GitHub Actions artifact where a mode needs it (update: before building content; post: after sending, to mark the previous message stale). Saved last when the mode produced one, holding PR refs, the Slack message ref, the last written message and the canvas markdown hash
- **Annotations** (`cmd/pr-slack-reminder/annotations.go`): reports warnings as they happen and the run's errors at exit, as annotations on the run page

Shared by all:

- **Models** (`internal/models/`): value types `Repository` and `PullRequestRef`
- **Utilities** (`internal/utilities/`): generic slice helpers used in place of manual loops

## Key Patterns

### Input Configuration

- GitHub Action inputs are accessed via `inputhelpers.GetInput()` (`internal/config/inputhelpers/`) which converts `input-name` to `INPUT_INPUT_NAME` env vars
- Repository-specific mappings use semicolon/newline-separated format: `"repo1: value1; repo2: value2"`
- JSON inputs are parsed with `DisallowUnknownFields()` for strict validation

### Error Handling

- Wrap an underlying error with `%w`, never `%v`: `fmt.Errorf("sending message: %w", err)`
- Only `main.go` exits: it writes one `::error` annotation per part of the run's error, then exits 1. Packages return errors
- Problems that don't fail the run are `::warning` annotations instead of plain log lines. See [run.spec.md](cmd/pr-slack-reminder/run.spec.md) § Behaviour for both
- Independent side effects fail independently: a failed message send doesn't skip the canvas refresh or vice versa, and a failed mark-as-stale edit doesn't stop state being saved. Their errors are joined into the run's error. See [run.spec.md](cmd/pr-slack-reminder/run.spec.md)
- Filters validate mutual exclusivity (e.g., can't use both `authors` and `ignored-authors`)

## File Relationships

- `action.yml` inputs must match constants in `internal/config/config.go`
- `testhelpers/confighelpers.go` mirrors real config parsing
- `.github/scripts/check_inputs.go` validates action.yml and config constants stay in sync

## Adding New Inputs

1. Add input to `action.yml`
2. Add constant to `internal/config/config.go`
3. Write failing tests first for the new functionality
4. Update `Config` struct and `GetConfig()` to make tests pass
5. Implement feature logic in the appropriate pipeline stage
6. Ensure all tests pass and refactor if needed
