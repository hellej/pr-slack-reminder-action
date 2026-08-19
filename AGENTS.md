# AGENTS.md

GitHub Action written in Go that fetches open PRs from GitHub repositories and sends or updates a Slack reminder listing them.

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

## Releasing

- Release procedure: [.agents/skills/release/SKILL.md](.agents/skills/release/SKILL.md)

## Package Specs

- Each Go package under `internal/` has a `<package>.spec.md` describing its current behaviour, non-goals, and oddities — read it before reading the package's source
- Writing/updating procedure: [.agents/skills/spec-writer/SKILL.md](.agents/skills/spec-writer/SKILL.md)
- Update a package's spec file whenever its behaviour changes, in the same change
- A `git commit` with staged `internal/**/*.go` changes but no staged spec update triggers a non-blocking reminder (`.claude/hooks/check-spec-sync.sh`) — safe to proceed if the change was a pure refactor

## Git

- Never amend commits or force push
- Work on `main` by default. Branch only if the user asks for a branch or mentions a PR
- Stage only the files for the task you were given. Another agent may have unrelated work
  in the same working tree, so never use `git add -A` or `git commit -a`

## Code Style

- **Readability > Speed:** Data sets are tiny; never trade clarity for execution speed or micro-optimizations.
- **KISS, YAGNI, & Avoid Hasty Abstractions (AHA):** Implement only what is required right now. Prefer concrete types and minor duplication over speculative wrappers, single-use interfaces, or premature helpers.
- **Intent-driven naming over comments:** Names must reveal *why* a variable or function exists (e.g., `activeSubscribers` over `filteredUsers`). If code feels complex enough to need a comment, refactor and/or rename instead.
- **Declarative slice transformations:** Avoid manual `for` loops and index management when transforming data. Always reuse or extend `./internal/utilities` (`Map`, `Filter`, `Find` etc).
- **Pure functions:** Prefer pure, side-effect-free functions. Return new slices or structs rather than mutating input pointers or package-level state.
- **Flat structure:** Use early returns and guard clauses. Do not nest `if` blocks deeper than 2 levels.
- **Keep exported type names exported:** Don't unexport a type just to shrink a package's API surface. Unexporting renames it, and lowercase type names read worse here. Funcs and consts are fine to unexport.

## Testing

- **Always use TDD**: write failing tests first, implement minimal code to pass, then refactor
- Use table-driven tests for functions with multiple input scenarios
- Check for existing helpers in `testhelpers/` before creating new ones
- `cmd/pr-slack-reminder/main_test.go` — integration tests using full pipeline with mocks
- `testhelpers/confighelpers.go` — `TestConfig` struct and `SetTestEnvironment()` for consistent test setup
- `testhelpers/mockgithubclient/` and `testhelpers/mockslackclient/` — injectable mock dependencies

## Development Commands

- `make test` — run all tests
- `make test-with-coverage` — run tests with coverage report (clears cache first)
- `make update-test-snapshots` — re-record the Slack payload snapshots in `cmd/pr-slack-reminder/testdata/snapshots/`
- `make run` — run locally (requires env vars, see Makefile for the pattern)
- `make build` — build linux binaries
- `make check-fmt` — fail if any file needs `gofmt`
- `make check-vet` — run `go vet ./...`
- `make check-dead-code` — fail if `deadcode` finds an unreachable function under `./cmd/...`. Expected to fail until plan 001 Step 1 lands
- `make check-vulnerabilities` — run `govulncheck ./...`
- `make install-hooks` — point git at `githooks/`, a pre-commit hook running `check-fmt` and `check-vet`. One-time opt-in per clone
- `go run .github/scripts/check_inputs.go` — validate action.yml and config.go constants are in sync

## Architecture

Two run modes (`run-mode` input): **post** sends a new reminder and saves state; **update** loads state, re-fetches those PRs, and edits or deletes the existing message.

1. **Config** (`internal/config/`) — parses GitHub Action inputs via `INPUT_` prefix env vars
2. **GitHub Client** (`internal/apiclients/githubclient/`) — fetches PR data and reviews, applies filtering
3. **PR Parser** (`internal/prparser/`) — enriches PRs with Slack user mappings and metadata
4. **Message Content** (`internal/messagecontent/`) — structures data for messaging
5. **Message Builder** (`internal/messagebuilder/`) — constructs Slack Block Kit messages
6. **Slack Client** (`internal/apiclients/slackclient/`) — sends, updates, or deletes messages
7. **State** (`internal/state/`) — persists PR refs and the Slack message ref after `post`; loaded from a GitHub Actions artifact in `update` mode

## Key Patterns

### Input Configuration

- GitHub Action inputs are accessed via `inputhelpers.GetInput()` (`internal/config/inputhelpers/`) which converts `input-name` to `INPUT_INPUT_NAME` env vars
- Repository-specific mappings use semicolon/newline-separated format: `"repo1: value1; repo2: value2"`
- JSON inputs are parsed with `DisallowUnknownFields()` for strict validation

### Error Handling

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
