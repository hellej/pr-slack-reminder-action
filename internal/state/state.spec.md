# state

Persists and reloads the "post" run's PR set and Slack message reference, so "update" mode can find and edit that message. See [AGENTS.md](../../AGENTS.md) for the two run-modes' overall flow.

## Behaviour

- State carries a schema version, creation time, the sent Slack message's channel/timestamp, and the list of PRs it covered
- `Load()` fetches the most recent saved state for a repository (via a GitHub Actions artifact)
- `SavePostState()` builds state from a "post" run's parsed PRs and Slack send result, and writes it for later reloading by `Load()`
- `SaveSentSlackBlocksToFile()` separately records the raw JSON actually sent to Slack, for inspection — not read back by this action
- `Save()` writes any already-constructed state value directly, for callers that don't need `SavePostState`'s assembly step

## Doesn't Do

- No migration path between schema versions, and nothing checks the version on load — a mismatch goes unnoticed
- Doesn't rotate or clean up old state/blocks files; each run overwrites in place

## Oddities

- `SaveSentSlackBlocksToFile`'s output is never loaded back by this codebase — it exists as a side-channel debug artifact only
