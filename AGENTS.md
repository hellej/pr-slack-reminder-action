# AGENTS.md

GitHub Action written in Go that fetches open PRs from GitHub repositories and sends or updates a Slack reminder listing them.

## Output Style

Applies to all agent output: chat answers, docstrings, plans, and text written to project files (docs, plans, skills, AGENTS.md).

- Use plain, simple words. Keep answers short and direct
- Prefer bullet points over prose
- Prefer short bullet points over long ones
- Avoid filler words
- Avoid duplication and overlap with what's already said or written

### Examples

Don't add trailing justification for an obvious rule:

- ✗ `Return an error instead of calling os.Exit, so callers can decide what to do.`
- ✓ `Return an error instead of calling os.Exit.`

Don't restate the rule as its own reason (circular justification):

- ✗ `The mock goes in testhelpers/ rather than the package under test, because testhelpers/ is where shared mocks live.`
- ✓ `The mock goes in testhelpers/.`

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

## Code Style

- Use descriptive naming instead of explanatory comments (exception: complex algorithms or non-obvious business logic)
- Use `errors.Join()` for combining multiple errors

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
- `make run` — run locally (requires env vars, see Makefile for the pattern)
- `make build` — build linux binaries
- `go run .github/scripts/check_inputs.go` — validate action.yml and config.go constants are in sync

## Architecture

Two run modes (`run-mode` input) drive the pipeline: **post** sends a new reminder and saves state; **update** loads state, re-fetches those PRs, and edits or deletes the existing message.

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

### Repository Processing

- Multiple repositories supported via `config.Repositories` slice of `Repository` structs (`config.InputGithubRepositories`)
- If `config.InputGithubRepositories` is set, `config.EnvGithubRepository` is ignored
- Repository filters are matched by full path (`owner/repo`) first, falling back to bare repository name
- Each PR maintains its `Repository` field for context throughout the pipeline

### Error Handling

- Filters validate mutual exclusivity (e.g., can't use both `authors` and `ignored-authors`)

### Slack Message Construction

- Uses Slack Block Kit with `RichTextBlock` and `RichTextSection`
- `IsOldPR` field controls age indicator styling (emoji + bold)

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
