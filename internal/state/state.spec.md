# state

Persists and reloads the "post" run's PR set and Slack message reference, so "update" mode can find and edit that message. See [AGENTS.md](../../AGENTS.md) for the two run-modes' overall flow.

## Behaviour

- State carries a schema version, creation time, the sent Slack message's channel/timestamp, and the list of PRs it covered
- `Load()` fetches the most recent saved state for a repository (via a GitHub Actions artifact)
- `Validate()` checks only that the state's schema version matches what this version of the action understands
- `SavePostState()` builds state from a "post" run's parsed PRs and Slack send result, and writes it for later reloading by `Load()`
- `SaveSentSlackBlocks()` separately records the raw JSON actually sent to Slack, for inspection — not read back by this action
- `Save()` writes any already-constructed state value directly, for callers that don't need `SavePostState`'s assembly step

## Doesn't Do

- `Validate()` doesn't check the state's contents (PR refs, Slack ref) are non-empty or well-formed — only the schema version
- No migration path between schema versions — a mismatch is a hard error
- Doesn't rotate or clean up old state/blocks files; each run overwrites in place

## Oddities

- `SaveSentSlackBlocks`'s output is never loaded back by this codebase — it exists as a side-channel debug artifact only
- `Load()` doesn't call `Validate()` itself — a caller that skips the explicit `Validate()` call can act on a state from an incompatible schema version
