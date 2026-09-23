# Mark the previous message stale

date: 2026-09-23
status: draft

## Goals

- When a `post` run sends a new message, the message the previous state points at gets its
  footer swapped in place: `_Live, updated 9:58 AM_` becomes
  `_⚠️ Stale, updated Sep 2 at 9:58 AM_`
  - The timestamp is the one the old live footer showed
  - Every other block of the old message stays as last sent, not refreshed
- Only one message in the channel ever reads "Live"
- The work lands on branch `claude/inspiring-shannon-2x6up8`, plan and implementation in one PR

Serves § **Purpose**: a reader scrolling back to yesterday's reminder no longer acts on rows
that read as live but stopped updating at the next 09:00 post. Sized against § **Reference
Deployment**: one daily post, so one edit per day, and no notification, since an edit does not
notify.

### Non-goals

- No new action input: marking is always on
- No link to the new message. The edit would add it, and `chat.update` documents no
  `unfurl_links`, so its preview could not be turned off (`slackapi/slack-api-specs`,
  `web-api/slack_web_openapi_v2.json`, `/chat.update` parameters)
- No change to update mode's own edit, delete or canvas behaviour
- No fix for the existing race where an update run loaded the previous state before the post
  finished (see **Caveats**)
- No marking of messages older than the previous one. Each was marked by its own successor

## Target shape

- `state.State` gains `LastSentMessage`: the message's blocks as last sent, its summary text, and
  the `generatedAt` its footer shows
  - `post` writes it with the new message, `update` rewrites it after each successful edit
  - An artifact saved before this change decodes an empty `LastSentMessage`, so there is nothing
    to mark
- A `post` run, after sending the new message:
  1. Loads the previous state. A failed load is logged and marking is skipped. The run succeeds
  2. Skips with a log line when the previous state has no `LastSentMessage`
  3. Rebuilds the previous message from its stored blocks, the footer replaced, and edits it
     with `chat.update`
  4. Treats `message_not_found` as nothing to mark. Any other Slack error joins the run's error,
     like a canvas failure. The new message and its state still land
- The stored blocks go back out through `slack.BlockFromJSON`, byte for byte
  (`slack-go@v0.29.0/block_json.go`), so no block type has to survive an unmarshal round trip
- The stale footer's time is `<!date^unix^{date_short_pretty} at {time}|Jan 2 15:04 UTC>`,
  rendered in each reader's timezone
  ([formatting message text](https://docs.slack.dev/messaging/formatting-message-text)).
  `{date_short_pretty}` reads `yesterday` for the usual daily case
  (`slackapi/node-slack-sdk` `main`, `packages/types/src/block-kit/block-elements.ts`)

### Action inputs and permissions

- No input added or renamed
- `github-token-for-state` and `state-artifact-name` descriptions (`action.yml`, README): `post`
  now reads the state too, to mark the previous message. Neither becomes required
- No new token permission or Slack scope: the state read already needs `actions: read`, and
  `chat.update` needs `chat:write`, which `update` mode already uses
- README example and `pr-reminder.yml`: state artifact `retention-days` goes from 1 to 4, so the
  next daily post still finds the state when no update run happened in between

## Breaking change

Minor. New default behaviour on an existing message, no input or config change. A setup without
state keeps working and marks nothing.

The release notes carry a `## Migration Guide (optional)` section: workflows uploading the state
artifact with `retention-days: 1` should raise it to 4, or the previous message is sometimes not
marked stale. Link the README's workflow example.

## Summary

- R1: Fix the sent-blocks record, which is empty in real runs
- 1: Store the last sent message in state
- 2: Build the stale message from stored blocks
- 3: Mark the previous message stale in `post`, record the last sent message in `update`
- 4: Docs and example workflow

## Steps

### R1. Fix the sent-blocks record

- `slackclient.parseSentJSONBlocks` returns nil on every real run: slack-go v0.21.1 moved block
  marshalling to send time, so `UnsafeApplyMsgOptions` returns no `blocks` key
  (`slack-go@v0.29.0/CHANGELOG.md`, `## [0.21.1]`)
- Tests miss it: the mock's `getJSONBlocks` marshals the blocks itself
- Replace `SentMessageInfo.JSONBlocks []string` with `Blocks json.RawMessage`, the block array as
  `json.Marshal(message.Blocks.BlockSet)` gives it, which is what `formSender` sends
  (`slack-go@v0.29.0/chat.go`, `formSender.BuildRequestContext`)
  - The mock calls the same exported slackclient function, so the two cannot drift again
- `state.SaveSentSlackBlocksToFile` takes the `json.RawMessage` and writes it as is. The file
  changes from `[[…blocks…]]` to `[…blocks…]`
- Re-record the ~10 snapshots in `cmd/pr-slack-reminder/testdata/snapshots/` with
  `make update-test-snapshots`. The diff drops one nesting level only
- A `slackclient` test through the real client and a fake `SlackAPI` fails before the fix
- Touches `slackclient`, `state`, `testhelpers/mockslackclient`, `run.go`'s sent-message handler.
  Update `slackclient.spec.md`

### 1. Store the last sent message in state

- New `state.LastSentMessage` struct, field `LastSentMessage` on `State`:
  - `Blocks json.RawMessage`: the block array from `SentMessageInfo.Blocks`
  - `SummaryText string`
  - `GeneratedAt time.Time`: what the live footer shows
- `NewPostState` takes the summary text and `generatedAt` and fills it
- New pure `WithLastSentMessage(state, sentMessageInfo, summaryText, generatedAt) State` for
  update mode, returning a copy
- `CurrentSchemaVersion` stays 1, as it did for `CanvasContentHash`. Nothing checks the version
- Update `state.spec.md`: the new field, its empty value in older artifacts, and the oddity that
  it is the only state update mode rewrites besides the canvas hash

### 2. Build the stale message from stored blocks

- New `messagebuilder.BuildStaleMessage(sentBlocks json.RawMessage, generatedAt time.Time)
  (slack.Message, error)`. Plain arguments, so `messagebuilder` does not import `state`
  - Splits `Blocks` into per-block `json.RawMessage`, since `BlockFromJSON` keeps only the first
    block of an array
  - Drops the last block, the live footer, and wraps the rest with `slack.BlockFromJSON`
  - Appends a context block reading
    `_⚠️ Stale, updated <!date^…^{date_short_pretty} at {time}|Jan 2 15:04 UTC>_`
  - Errors on blocks that do not parse or are empty
- Tests: every non-footer block comes back byte-identical, including a header's `level` and a
  grouped message's spacing block; the footer text; malformed and empty input
- Update `messagebuilder.spec.md`: the stale footer, and that the stale message never re-renders
  content

### 3. Mark the previous message stale, record the last sent message

- `runPostMode`, after a successful send:
  - Loads the previous state with `state.Load`. A failure logs and skips
  - Skips with a log line when `LastSentMessage.Blocks` is empty
  - Builds the stale message and edits it with `slackClient.UpdateMessage`, on the previous
    state's channel ID and timestamp, with the stored summary text
  - Does not pass the stale edit to `sentMessageHandler`: the debug file and snapshots record the
    new message only
- `slackclient.UpdateMessage` returns an error the caller can tell apart as "message not found"
  (a sentinel wrapped with `%w`). `DeleteMessage`'s string match on `message_not_found` moves to
  the same check
- The marking error joins `messageErr`'s return. Post still returns the new state, so it is saved
- `runUpdateMode` returns `state.WithLastSentMessage(...)` after a successful edit. The delete and
  keep branches return the loaded state unchanged
- `mockgithubclient`'s `MockStateForUpdateMode` is now read by post mode too. Rename it to
  `MockPreviousState` across ~20 call sites, no behaviour change
- Integration tests in `main_test.go`:
  - The old message gets the stale footer and its other blocks unchanged
  - A failed state load, a state without `LastSentMessage`, and `message_not_found` each leave
    the run green with no edit or with the edit ignored
  - Another Slack error on the stale edit fails the run, and the new state still saves
  - Update mode saves the edited blocks, summary text and `generatedAt`
- Update `run.spec.md`
- Done also means a live check: `gh workflow run pr-reminder.yml --ref <branch> -f run-mode=post
  -f build-first=true`, run twice. The first run's message shows the stale footer, its other
  blocks unchanged, and the date reads as expected

### 4. Docs and example workflow

- `action.yml`: `github-token-for-state` and `state-artifact-name` descriptions say `post` reads
  the state too. `go run .github/scripts/check_inputs.go` still passes
- README:
  - The same two input descriptions
  - One line where run modes are described: `post` marks the previous message stale
  - The workflow example's `retention-days: 4`
- `.github/workflows/pr-reminder.yml`: `retention-days: 4`
- Verified by reading the rendered README

## Consequences

### Positive

- An old reminder can't be mistaken for the current one
- The sent-blocks debug file records real payloads again

### Negative

None

### Caveats

- A previous state older than the artifact's retention, or missing, leaves that message reading
  "Live"
- An update run that loaded the previous state before a post finished edits the old message
  afterwards, restoring its live footer, and its state upload then points later update runs at
  the old message. The post and update concurrency groups differ, so this can already happen
  today, and marking makes it visible
- Unverified: whether Slack applies an edit window to a bot editing its own message. If it does,
  `edit_window_closed` fails the run
- Unverified: that the mrkdwn `{date_short_pretty}` reads as the node SDK documents it for the
  rich_text `date` element. The live check in Step 3 confirms it
- State artifacts grow by the message's JSON, a few KB at most

### Neutral

- `SentMessageInfo.JSONBlocks` becomes `Blocks`, an internal type change
- The snapshot files lose one nesting level
