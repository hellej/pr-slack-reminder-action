# Mark the previous message stale

date: 2026-09-23
status: draft

## Goals

- When a `post` run sends a new message, the message the previous state points at gets its
  footer swapped in place: `_Live, updated 9:58 AM_` becomes
  `_⚠️ Stale, updated September 2nd at 9:58 AM_`
  - The timestamp is the one the old live footer showed
  - Every other block of the old message stays as last sent, not refreshed
- Only the newest reminder reads "Live"
- The work lands on branch `claude/inspiring-shannon-2x6up8`, plan and implementation in one PR

Serves § **Purpose**: a reader scrolling back to yesterday's reminder no longer acts on rows
that read as live but stopped updating at the next 09:00 post. Sized against § **Reference
Deployment**: one daily post, so one edit per day, and no notification, since an edit does not
notify.

### Non-goals

- No change to the PR tracker canvas. This is about the channel message only
- No new action input: marking is always on
- No link to the new message: `chat.update` has no documented way to suppress its preview
  ([chat.update](https://docs.slack.dev/reference/methods/chat.update) lists no `unfurl_links`)
- No change to update mode's own edit or delete behaviour
- No fix for the existing race where an update run loaded the previous state before the post
  finished (see **Caveats**)
- No marking of messages older than the previous one. Each was marked by its own successor

## Target shape

- `state.State` gains `LastSentMessage`: the message's blocks as last sent, its summary text, and
  the `generatedAt` its footer shows
  - `post` writes it with the new message, `update` rewrites it after each successful edit
  - An artifact saved before this change decodes an empty `LastSentMessage`, so there is nothing
    to mark
- `post` marks the previous message stale after sending the new one. See Step 3
- The stale footer's time is `<!date^unix^{date_pretty} at {time}|Jan 2 15:04 UTC>`, rendered
  in each reader's timezone
  ([formatting message text](https://docs.slack.dev/messaging/formatting-message-text))
  - `{date_pretty}` reads `yesterday` for the usual daily case, otherwise `{date}`'s
    `September 2nd`, with the year only past six months
  - A bot's edited message never shows the `(edited)` label
    ([chat.update](https://docs.slack.dev/reference/methods/chat.update))

### Action inputs and permissions

- No input added or renamed
- `github-token-for-state` and `state-artifact-name` descriptions (`action.yml`, README): `post`
  now reads the state too, to mark the previous message. Neither becomes required
- No new required token permission or Slack scope. `actions: read`, already needed by `update`,
  now also lets `post` mark the previous message. Without it `post` still posts. `chat.update`
  needs `chat:write`, which `update` mode already uses
- README example and `pr-reminder.yml`: state artifact `retention-days` goes from 1 to 4, so the
  next daily post still finds the state when no update run happened in between

## Breaking change

Minor. No input or config change, and a setup without state keeps working and marks nothing.

The release notes carry a `## Migration Guide (optional)` section: workflows uploading the state
artifact with `retention-days: 1` should raise it to 4, or the previous message is sometimes not
marked stale. Link the README's workflow example.

## Summary

- R1: Fix the sent-blocks record, which is empty in real runs
- R2: Rename the mock's `MockStateForUpdateMode`
- 1: Store the last sent message in state, written by both run modes
- 2: Build the stale message from stored blocks
- 3: Mark the previous message stale in `post`
- 4: Docs, example workflow and the release note

## Steps

### R1. Fix the sent-blocks record

- `slackclient.parseSentJSONBlocks` returns nil on every real run: slack-go v0.21.1 moved block
  marshalling to send time, so `UnsafeApplyMsgOptions` returns no `blocks` key
  (`slack-go@v0.29.0/CHANGELOG.md`, `## [0.21.1]`)
- Tests miss it: the mock's `getJSONBlocks` marshals the blocks itself
- Replace `SentMessageInfo.JSONBlocks []string` with `Blocks json.RawMessage`, the block array as
  sent
- New exported `slackclient.MarshalSentBlocks(message slack.Message) (json.RawMessage, error)`
  returns `json.Marshal(message.Blocks.BlockSet)`, which is what `formSender` sends for posts and
  edits alike (`slack-go@v0.29.0/chat.go`, `sendConfig.BuildRequestContext`,
  `formSender.BuildRequestContext`)
  - It replaces `parseSentJSONBlocks`, and the mock calls it in place of its own `getJSONBlocks`,
    which goes away
  - `SendMessage` and `UpdateMessage` call it before the Slack call and return its error, so
    blocks that fail to marshal never reach Slack
- `state.SaveSentSlackBlocksToFile` takes the `json.RawMessage` and writes it indented
  (`json.Indent`). The file changes from `[[…blocks…]]` to `[…blocks…]`
  - Empty blocks are an error and write no file: they mean the info did not come from a send,
    which an empty record would hide
- Re-record the 11 snapshots in `cmd/pr-slack-reminder/testdata/snapshots/` with
  `make update-test-snapshots`. The diff drops one nesting level only
- `canvas_test.go`'s `TestCanvasDoesNotChangeMessageBlocks` parses the file as one block array
- A `slackclient` test through the real client and a fake `SlackAPI` fails before the fix
- Touches `slackclient`, `state`, `testhelpers/mockslackclient`, `run.go`'s sent-message handler,
  `canvas_test.go`. Update `slackclient.spec.md` and `state.spec.md`

### R2. Rename the mock's `MockStateForUpdateMode`

- Post mode will read it too. Rename to `MockPreviousState` in `testhelpers/mockgithubclient`
  and its 18 call sites in `cmd/pr-slack-reminder`'s tests, no behaviour change

### 1. Store the last sent message in state

- New `state.LastSentMessage` struct, field `LastSentMessage` on `State`:
  - `Blocks json.RawMessage`: the block array from `SentMessageInfo.Blocks`
  - `SummaryText string`
  - `GeneratedAt time.Time`: what the live footer shows
  - `Blocks` is `omitempty`: a nil `json.RawMessage` saves as `null`, which loads back as the
    bytes `null`, so a state update mode saves back unedited would no longer read as empty
- `NewPostState` takes the summary text and `generatedAt` and fills it. `runPostMode` passes its
  own
- New pure `WithLastSentMessage(state, sentMessageInfo, summaryText, generatedAt) State` for
  update mode, returning a copy
- `runUpdateMode` returns `state.WithLastSentMessage(...)` after a successful edit. The delete and
  keep branches, and a failed edit, return the loaded state unchanged
  - Wired here rather than in Step 3, since an unused `WithLastSentMessage` fails
    `make check-dead-code`. Nothing reads the field until Step 3
- `CurrentSchemaVersion` stays 1, as it did for `CanvasContentHash`. Nothing checks the version
- Unit tests pin the field through save and load, an old artifact without it, and
  `WithLastSentMessage` leaving its input alone. Integration tests in `main_test.go` pin the
  message each mode saves, and the loaded one kept on update mode's early returns
- Update `state.spec.md`: the new field, its empty value in older artifacts, and the oddity that
  it is the only state update mode rewrites besides the canvas hash. Update `run.spec.md`: each
  mode saves the message it sent

### 2. Build the stale message from stored blocks

- New `messagebuilder.BuildStaleMessage(sentBlocks json.RawMessage, generatedAt time.Time)
  (slack.Message, error)`. Plain arguments, so `messagebuilder` does not import `state`
  - Splits `sentBlocks` into per-block `json.RawMessage`, since `slack.BlockFromJSON` keeps only
    the first block of an array (`slack-go@v0.29.0/block_json.go`)
  - Drops the last block, the live footer, and wraps the rest with `slack.BlockFromJSON`, which
    re-sends each byte for byte, so no block type has to survive an unmarshal round trip
  - Does not check that the dropped block is a `context` block: `BuildMessage` always puts the
    footer last, and nothing else writes the stored blocks
  - Appends a context block reading
    `_⚠️ Stale, updated <!date^…^{date_pretty} at {time}|Jan 2 15:04 UTC>_`
  - Errors on blocks that do not parse or are empty
- Unit tests pin byte-identical non-footer blocks, the same compact blocks from an indented
  store, the stale footer, and the error cases
- `make check-dead-code` flags `BuildStaleMessage` and its two helpers until Step 3 calls it
- Update `messagebuilder.spec.md`: the stale footer, and that the stale message never re-renders
  content

### 3. Mark the previous message stale

- `runPostMode`, after a successful send, calls `markPreviousMessageStale`:
  - Loads the previous state with `state.Load`, after the send, so a run that sends nothing
    downloads no artifact. This run's own state is uploaded after it ends, so either order reads
    the previous post's. A failure logs and skips
  - Skips with a log line when `LastSentMessage.Blocks` is empty
  - Builds the stale message and edits it with `slackClient.UpdateMessage`, on the previous
    state's channel ID and timestamp, with the stored summary text
  - Does not pass the stale edit to `sentMessageHandler`: the debug file and snapshots record the
    new message only
- `slackclient.UpdateMessage` wraps Slack's `message_not_found`, `cant_update_message` and
  `edit_window_closed` in one sentinel, `ErrMessageNotEditable`, with `%w`
  ([chat.update](https://docs.slack.dev/reference/methods/chat.update) errors)
  - Post mode logs it and skips: that message cannot be marked
  - `DeleteMessage`'s string match on `message_not_found` stays as it is
  - It matches the code on `slack.SlackErrorResponse`, which slack-go returns for an `ok: false`
    response, its `Err` the error code (`slack-go@v0.29.0/misc.go`, `SlackResponse.Err`)
  - The mapping is exported as `slackclient.WrapUpdateMessageError`, and
    `mockslackclient.UpdateMessage` calls it, so the mock cannot drift from the real client
- Any other error from the stale edit, a `BuildStaleMessage` error included, joins post mode's
  returned error. Post still returns the new state, so it is saved
- The mock's `UpdatedMessage` also records `SentBlocks`, the block array as sent, so a test pins
  the stale edit byte for byte
- A `slackclient` unit test through the fake `SlackAPI` pins the error mapping. Integration tests
  in `main_test.go` cover the mark, each skip path, the failing edit, and no mark when the send
  fails or there is nothing to send
- Update `run.spec.md` and `slackclient.spec.md`
- Done also means a live check: `gh workflow run pr-reminder.yml --ref <branch> -f run-mode=post
  -f build-first=true`, run twice:
  - The first run's message keeps its other blocks unchanged, and its footer reads
    `⚠️ Stale, updated today at <time>`, not the raw `<!date…>` text or the `UTC` fallback
  - Re-opened the next day, the same footer reads `yesterday at <time>`

### 4. Docs, example workflow and the release note

- `action.yml`: `github-token-for-state` and `state-artifact-name` descriptions say `post` reads
  the state too. `go run .github/scripts/check_inputs.go` still passes
- README:
  - The same two input descriptions
  - One line where run modes are described: `post` marks the previous message stale
  - The workflow example's `retention-days: 4`
  - The permissions snippet's `actions: read` comment: needed by `update`, and lets `post` mark
    the previous message
- `.github/workflows/pr-reminder.yml`: `retention-days: 4`
- The PR description carries the `## Migration Guide (optional)` text from **Breaking change**,
  since the release skill drops README, workflow and `docs/plans/` commits from the notes
- Done means the rendered README reads right and the PR description holds the migration text

## Consequences

### Positive

- An old reminder can't be mistaken for the current one
- The sent-blocks debug file records real payloads again

### Negative

None

### Caveats

- A previous state older than the artifact's retention, or missing, leaves that message reading
  "Live"
- `edit_window_closed` comes from the workspace's message edit settings. Unverified: whether they
  apply to a bot's own messages. If they do, a message past the window keeps reading "Live", and
  the run stays green
- An update run that loaded the previous state before a post finished edits the old message
  afterwards, restoring its live footer, and its state upload then points later update runs at
  the old message. The post and update concurrency groups differ, so this can already happen
  today, and marking makes it visible
- State artifacts grow by the message's JSON, a few KB at most

### Neutral

- `SentMessageInfo.JSONBlocks` becomes `Blocks`, an internal type change
- The snapshot files lose one nesting level
