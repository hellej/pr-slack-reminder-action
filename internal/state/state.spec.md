# state

Persists and reloads the "post" run's PR set, Slack message reference and last written message, so "update" mode can find and edit that message and the next "post" can mark it stale. See [AGENTS.md](../../AGENTS.md) for the two run-modes' overall flow.

## Behaviour

- State carries a schema version, `MessagePostedAt`, `MessageRef` (the sent Slack message's channel/timestamp), the list of PRs it covered, the hash of the markdown last written to the PR tracker canvas, and `LastWrittenMessage`: the message's blocks as last sent or edited, its summary text, and the `generatedAt` its content was built at
- `Load()` fetches the most recent saved state for a repository (via a GitHub Actions artifact)
- `NewPostState()` builds state from a "post" run's PR views, Slack send result, summary text and `generatedAt`. The only place stamping the schema version and `MessagePostedAt`, and it leaves the canvas hash empty for the caller to fill in
- `WithLastWrittenMessage()` returns a copy of a state with `LastWrittenMessage` replaced by an edit's blocks, summary text and `generatedAt`
- `Save()` writes a state value to a file, for later reloading by `Load()`, and logs what it wrote
- `SaveSentSlackBlocksToFile()` separately writes a sent message's block array to a file, indented, for inspection
  - Errors on empty blocks rather than writing an empty record

## Doesn't Do

- No migration path between schema versions, and nothing checks the version on load: a mismatch goes unnoticed
- Doesn't rotate or clean up old state/blocks files; each run overwrites in place

## Oddities

- `SaveSentSlackBlocksToFile`'s output is never loaded back by this codebase. It exists as a side-channel debug artifact only
- `LastWrittenCanvasMarkdownHash` (JSON key `canvasContentHash`) was added without bumping `CurrentSchemaVersion`. An artifact saved before it decodes an empty hash, which reads as "write the canvas"
- `LastWrittenMessage` was added the same way. An artifact saved before it decodes an empty `LastWrittenMessage`, and an empty one saves and loads back empty
- `LastWrittenMessage` is the only state an update run rewrites besides the canvas hash. The PR set, `MessageRef` and `MessagePostedAt` stay the post run's
- `Save()` indents the stored blocks with the rest of the file, so they load back with the same JSON but not the same bytes as sent
- `MessagePostedAt` and `MessageRef` save as `messagePostedAt` and `messageRef`. Loading still reads the keys older releases wrote, `createdAt` and `slackMessage`, when the new key is absent or empty. A later release drops that fallback
- A PR ref's repository saves as `"Owner"` and `"Name"`, capitalised: `models.Repository` has no JSON tags. The state snapshots in `cmd/pr-slack-reminder` pin these keys, and renaming those Go fields would break reading older state files
