# state

Persists and reloads the "post" run's PR set, Slack message reference and last sent message, so "update" mode can find and edit that message and the next "post" can mark it stale. See [AGENTS.md](../../AGENTS.md) for the two run-modes' overall flow.

## Behaviour

- State carries a schema version, creation time, the sent Slack message's channel/timestamp, the list of PRs it covered, the hash of the markdown last written to the PR tracker canvas, and `LastSentMessage`: the message's blocks as last sent or edited, its summary text, and the `generatedAt` its content was built at
- `Load()` fetches the most recent saved state for a repository (via a GitHub Actions artifact)
- `NewPostState()` builds state from a "post" run's PR views, Slack send result, summary text and `generatedAt`. The only place stamping the schema version and creation time, and it leaves the canvas hash empty for the caller to fill in
- `WithLastSentMessage()` returns a copy of a state with `LastSentMessage` replaced by an edit's blocks, summary text and `generatedAt`
- `CreatedAt` is stamped after the Slack send, so it marks when the message was posted
- `Save()` writes a state value to a file, for later reloading by `Load()`, and logs what it wrote
- `SaveSentSlackBlocksToFile()` separately writes a sent message's block array to a file, indented, for inspection
  - Errors on empty blocks rather than writing an empty record

## Doesn't Do

- No migration path between schema versions, and nothing checks the version on load — a mismatch goes unnoticed
- Doesn't rotate or clean up old state/blocks files; each run overwrites in place

## Oddities

- `SaveSentSlackBlocksToFile`'s output is never loaded back by this codebase — it exists as a side-channel debug artifact only
- `LastWrittenCanvasMarkdownHash` (JSON key `canvasContentHash`) was added without bumping `CurrentSchemaVersion`. An artifact saved before it decodes an empty hash, which reads as "write the canvas"
- `LastSentMessage` was added the same way. An artifact saved before it decodes an empty `LastSentMessage`, and an empty one saves and loads back empty
- `LastSentMessage` is the only state an update run rewrites besides the canvas hash. The PR set, message ref and `CreatedAt` stay the post run's
- `Save()` indents the stored blocks with the rest of the file, so they load back with the same JSON but not the same bytes as sent
- A PR ref's repository saves as `"Owner"` and `"Name"`, capitalised: `models.Repository` has no JSON tags. The state snapshots in `cmd/pr-slack-reminder` pin these keys, and renaming those Go fields would break reading older state files
