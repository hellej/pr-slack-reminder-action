# "Update" mode plan

Plan for adding support for a second run mode _update_ for the action.

## Run Modes
1. post (default): sends a PR reminder
2. update: updates that same reminder as PRs get reviewed/merged

Updated message contains exactly the same PR set as the original post (no new PRs included), regardless of whether they are now open, merged (or later optionally closed).

## High-Level Flow
post run:
- Fetch & filter PRs (current pipeline)
- Build + send Slack message
- Persist state (Slack identifiers + PR list)

update run:
- Load persisted state
- Re-fetch only those PRs in state (skip discovering new ones)
- Rebuild message (same deterministic PR ordering rules used in both modes)
- Mark merged PRs 🔀, refresh ages, reviewers, commenters
- Update existing Slack message in-place (chat.update)
- Add/update footer with refreshed timestamp (UTC)

Only post writes state. update is read-only regarding state (idempotent; does not alter file).

## State Contents
- Slack message reference (channel_id + message_ts)
- List of PR references (owner, repo, number)
- Version + created_at for evolution and staleness checks

### State JSON example (MVP)
```json
{
  "schemaVersion": 1,
  "createdAt": "2025-10-15T12:34:56Z",
  "slack": {
    "channelId": "C123",
    "messageTs": "1728392.000200"
  },
  "pullRequests": [
    { "owner": "owner1", "repo": "repo1", "number": 1 },
    { "owner": "owner1", "repo": "repo1", "number": 2 }
  ]
}
```

### State as Go types (structs)

```go
// Decide on schema version form: keep int (simple comparisons) OR switch to semantic string.
// Current JSON example uses an integer (schemaVersion: 1). We'll keep int.
const CurrentSchemaVersion = 1

type SlackRef struct {
  ChannelID string `json:"channelId"`
  MessageTS string `json:"messageTs"`
}

type PullRequestRef struct {
  Owner  string `json:"owner"`
  Repo   string `json:"repo"`
  Number int    `json:"number"`
}

type State struct {
  SchemaVersion int              `json:"schemaVersion"`
  CreatedAt     time.Time        `json:"createdAt"`
  Slack         SlackRef         `json:"slack"`
  PullRequests  []PullRequestRef `json:"pullRequests"`
}

// Validation sketch:
// func (s *State) Validate() error { ... }
```

Notes:
- pull_requests: `id` is a convenience key; structured fields to avoid reparsing.
- No reviewer/commenter snapshots in MVP; always re-derived on update.

## Storage Strategy
MVP: GitHub Cache action
- Pros: no extra permissions, quick to prototype
- Cons: eviction risk; not durable across long periods

Future: GitHub Actions Artifact
- Pros: controlled retention, explicit download, durable
- Needs: `actions: read` permission

Abstraction: read/write plain JSON file at configured path; the workflow decides how it is persisted (cache vs artifact). Avoid coupling core logic to the backend.

## Inputs (Proposed)
Minimal for MVP:
- `mode` (post|update) (default: post)
- `state-file-path` (default: `.pr-slack-reminder/state.json`)

Potential future inputs (defer unless needed):
- `state-cache-key` (override automatic key)
- `state-cache-ttl` (duration; warn/fail if state too old)
- `fail-if-missing-state` (bool, default true in update)
- `slack-update-retry-attempts` (default 3)
- `persistence` (cache|artifact)

## Message Update Behavior
- Recompute ages relative to current time
- Preserve original formatting (prefixes, sections)
- Add 🔀 indicator for merged PRs (suffix after reviewer info)
- Reviewer/commenter lists refreshed
- Footer replaced/added: `review info updated at HH:MM UTC`

## Edge Cases & Error Handling
| Scenario | Behavior (MVP) |
|----------|----------------|
| State file missing in update | Fail (clear error) |
| `fail-if-missing-state=false` | Log warning, exit success no-op |
| Slack message deleted | chat.update fails → error (future: optionally re-post) |
| PR merged | Show normally with 🔀 marker |
| PR closed (not merged) | Treat as still listed (no special marker MVP) |
| PR not found / permission lost | Skip with warning (future: placeholder) |
| State version != supported | Fail fast |
| State too old (if ttl configured) | Fail or warn depending on design (deferred) |
| Duplicate post with same path | Overwrite state silently (document) |
| Update retried | Safe; idempotent (same chat.update) |

## API Call Minimization
- Update mode fetches only PRs present in state (group PRs by repo).
- Single Slack update call per run.

## Idempotency
- Re-running post overwrites state and posts a brand-new message.
- Re-running update with unchanged PR states yields identical message payload.

## Testing Strategy (TDD)

The implementation will strictly follow a Test-Driven Development approach. For each step, failing tests will be written first to define the required functionality, followed by the minimal implementation to make them pass.

### 1. Config & Input Handling (`config_test.go`)
- Test that `mode` defaults to `post` when the input is not provided.
- Test that `mode` is correctly parsed as `post` or `update`.
- Test that an invalid `mode` value results in a validation error.
- Test that `state-file-path` is parsed correctly.
- Test that `state-file-path` falls back to the documented default (`.pr-slack-reminder/state.json`).

### 2. State Persistence (`internal/state/state_test.go`)
- Test successful `Save` and `Load` round-trip for a `State` object.
- Test that `Load` returns a `FileNotFound` error (or similar) when the state file does not exist.
- Test that `Load` returns an `InvalidJSON` error (or similar) for a malformed file.
- Test that `Validate()` method on `State` returns an error if `SchemaVersion` does not match `CurrentSchemaVersion`.

### 3. End-to-End Integration (`cmd/pr-slack-reminder/main_test.go`)
- **Post Mode**:
    1. Simulate a `post` run.
    2. Assert that the Slack client's `PostMessage` method was called.
    3. Assert that a state file is created at the expected path.
    4. Assert that the contents of the saved state file match the PRs and Slack message details from the run.
- **Update Mode (Happy Path)**:
    1. Seed a valid state file from a previous "post" run.
    2. Mutate the mock GitHub client to mark one PR as merged.
    3. Simulate an `update` run.
    4. Assert that the GitHub client was called only for the PRs listed in the state file.
    5. Assert that the Slack client's `UpdateMessage` method was called with the correct channel ID and timestamp.
    6. Assert that the generated message blocks contain the `🔀` marker for the merged PR and the "updated at" footer.
- **Update Mode (Edge Cases)**:
    1. Test that an `update` run fails clearly when the state file is missing.
    2. Test that an `update` run fails if the state file has a schema version mismatch.
    3. Test that the action correctly bubbles up an error if the mock Slack client's `UpdateMessage` call fails.

## Implementation Steps (Checklist)

Follow these steps in order, ensuring tests are written before implementation at each stage.

### 1. Configure Inputs
- **`internal/config/config.go`**:
    - Add `InputRunMode` and `InputStateFilePath` constants.
    - Add `RunMode` and `StateFilePath` fields to the `Config` struct.
    - In `GetConfig()`, parse these new inputs, setting a default for `RunMode` to `post`.
- **`internal/config/config_test.go`**:
    - Implement the tests outlined in "Testing Strategy" for config handling.

### 2. Create State Package
- **`internal/state/state.go`**:
    - Create the new package and file.
    - Define the `State`, `SlackRef`, `PullRequestRef` structs and the `CurrentSchemaVersion` constant.
    - Implement `Save(path string, state State) error` and `Load(path string) (*State, error)`.
    - Implement a `Validate() error` method on the `State` struct.
- **`internal/state/state_test.go`**:
    - Implement the tests for state persistence and validation.

### 3. Extend Slack Client Mock
- **`testhelpers/mockslackclient/mockslackclient.go`**:
    - Add an `UpdateMessage` method to the mock client.
    - Record the `channelID`, `timestamp`, and message `blocks` it was called with.
    - Add a corresponding `GetLastUpdateMessage()` helper to retrieve the recorded data for assertions.

### 4. Implement "Post" Mode Logic
- **`cmd/pr-slack-reminder/run.go`**:
    - In the `run` function, add a condition: `if cfg.RunMode == "post"`.
    - Inside this block, after a successful `slackClient.PostMessage` call:
        1. Construct the `state.State` object using the PRs from the pipeline and the response from `PostMessage`.
        2. Call `state.Save()` to write the state file to `cfg.StateFilePath`.
- **`cmd/pr-slack-reminder/main_test.go`**:
    - Implement the integration test for "Post Mode" to verify the state file is created correctly.

### 5. Implement "Update" Mode Logic
- **`cmd/pr-slack-reminder/run.go`**:
    - Add the main `else if cfg.RunMode == "update"` block.
    - Inside this block:
        1. Call `state.Load()` and `state.Validate()`. Handle errors according to the plan (fail fast).
        2. Instead of discovering PRs, use the `PullRequestRef` list from the loaded state to fetch PR data directly. This will require a new or modified function in `githubclient`.
        3. Pass the fetched PRs through the existing `prparser` and `messagecontent` pipeline.
        4. In `messagebuilder`, add logic to append the `🔀` marker to merged PRs and include the "updated at" footer.
        5. Call the new `slackClient.UpdateMessage` method with the message details from the state and the newly generated blocks.
- **`cmd/pr-slack-reminder/main_test.go`**:
    - Implement the integration tests for "Update Mode" (happy path and edge cases).

### 6. Refactor and Finalize
- Review the changes for any code duplication or areas that could be simplified.
- Ensure all tests are passing and logging provides clear insights into `post` and `update` operations.

## Logging Guidelines
- post: `saved state path=... prs=N`
- update: `loaded state path=... prs=N age=...`
- warnings for skipped/missing PRs

## Risks & Mitigations
- Cache eviction → update failure: Document; later switch to artifact backend.
- Schema evolution: Version now; add migration path later if needed.
- Slack rate limit: Single update unlikely to hit; add simple retry/backoff.
- Permissions drift: Validate early, fail fast with actionable message.

## Future Enhancements (Not MVP)
- Artifact persistence backend
- Closed (not merged) indicator (🚫) vs merged (🔀)
- Multiple tracked messages (namespaced state keys)

## Auth
- Slack: chat.postMessage, chat.update only.
- GitHub: read PRs; later maybe actions:read for artifact strategy.
- No repository write permissions required.

---
This document defines the MVP scope and guardrails for implementing update mode with a clear TDD path and extensibility for future enhancements.
