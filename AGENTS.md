# AGENTS.md

GitHub Action written in Go that fetches open PRs from GitHub repositories and sends or updates a Slack reminder listing them.

## Releasing

- Release procedure: [.agents/skills/release/SKILL.md](.agents/skills/release/SKILL.md)

## Code Style

- Use descriptive naming instead of explanatory comments (exception: complex algorithms or non-obvious business logic)
- Return errors instead of panicking
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

Data pipeline with 6 stages:

1. **Config** (`internal/config/`) — parses GitHub Action inputs via `INPUT_` prefix env vars
2. **GitHub Client** (`internal/apiclients/githubclient/`) — fetches PR data and reviews, applies filtering
3. **PR Parser** (`internal/prparser/`) — enriches PRs with Slack user mappings and metadata
4. **Message Content** (`internal/messagecontent/`) — structures data for messaging
5. **Message Builder** (`internal/messagebuilder/`) — constructs Slack Block Kit messages
6. **Slack Client** (`internal/apiclients/slackclient/`) — sends messages

## Key Patterns

### Input Configuration

- GitHub Action inputs are accessed via `inputhelpers.GetInput()` (`internal/config/inputhelpers/`) which converts `input-name` to `INPUT_INPUT_NAME` env vars
- Repository-specific mappings use semicolon/newline-separated format: `"repo1: value1; repo2: value2"`
- JSON inputs are parsed with `DisallowUnknownFields()` for strict validation

### Repository Processing

- Multiple repositories supported via `config.Repositories` slice of `Repository` structs (`config.InputGithubRepositories`)
- If `config.InputGithubRepositories` is set, `config.EnvGithubRepository` is ignored
- Repository filters are mapped by repository name (not full path)
- Each PR maintains its `Repository` field for context throughout the pipeline

### Error Handling

- Filters validate mutual exclusivity (e.g., can't use both `authors` and `ignored-authors`)
- Missing required inputs fail fast with descriptive error messages

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
