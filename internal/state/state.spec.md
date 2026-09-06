# state

Persists and reloads the "post" run's PR set and Slack message reference, so "update" mode can find and edit that message. See [AGENTS.md](../../AGENTS.md) for the two run-modes' overall flow.

## Behaviour

- State carries a schema version, creation time, the sent Slack message's channel/timestamp, the list of PRs it covered, and the hash of the markdown last written to the PR tracker canvas
- `Load()` fetches the most recent saved state for a repository (via a GitHub Actions artifact)
- `NewPostState()` builds state from a "post" run's parsed PRs and Slack send result. The only place stamping the schema version and creation time, and it leaves the canvas hash empty for the caller to fill in
- `Save()` writes a state value to a file, for later reloading by `Load()`, and logs what it wrote
- `SaveSentSlackBlocksToFile()` separately records the raw JSON actually sent to Slack, for inspection — not read back by this action

## Doesn't Do

- No migration path between schema versions, and nothing checks the version on load — a mismatch goes unnoticed
- Doesn't rotate or clean up old state/blocks files; each run overwrites in place

## Oddities

- `SaveSentSlackBlocksToFile`'s output is never loaded back by this codebase — it exists as a side-channel debug artifact only
- `CanvasContentHash` was added without bumping `CurrentSchemaVersion`. An artifact saved before it decodes an empty hash, which reads as "write the canvas"
