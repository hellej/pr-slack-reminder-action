# slackclient

Sends PR reminder messages to Slack and replaces PR tracker canvas content.

## Behaviour

- `Client.GetChannelIDByName` resolves a Slack channel ID by name, searching public then private conversations
- `Client.SendMessage` posts a new Block Kit message; `UpdateMessage` edits an existing one by timestamp; `DeleteMessage` removes one
- `Client.ReplaceCanvasContent` replaces a canvas's whole content with a markdown string, via one `canvases.edit` call with a `replace` change carrying no section ID. Requires the `canvases:write` scope and canvas access, which the bot gets implicitly when the canvas is a tab in a channel it is in
- Send/update calls return `SentMessageInfo`: channel ID, timestamp, and the block array as sent
- `UpdateMessage` wraps Slack's `message_not_found`, `cant_update_message` and `edit_window_closed` in `ErrMessageNotEditable`: the message is gone or can no longer be edited. `WrapUpdateMessageError` holds that mapping, exported so the mock shares it
- `MarshalSentBlocks` returns a message's block array as sent. Send/update call it before the Slack call, so blocks that fail to marshal fail the call without reaching Slack

## Doesn't Do

- `SendMessage` doesn't chunk oversized messages; it errors if given more than 50 blocks
- No retries or rate-limit backoff on API errors
- No canvas creation, deletion, lookup or access granting; the canvas is created by the user and addressed by ID
- `ReplaceCanvasContent` doesn't split oversized content; Slack's canvas size limit fails the call

## Oddities

- `GetChannelIDByName`'s error message differs depending on whether the public or private channel listing failed, and suggests using the channel-ID input instead
- `DeleteMessage` treats a Slack "message not found" error as success (already-deleted is not an error)
- `UpdateMessage` matches the error code on slack-go's `slack.SlackErrorResponse` type, while `DeleteMessage` matches `message_not_found` anywhere in the error text
- A failed `ReplaceCanvasContent` wraps the Slack error with a fixed hint about the `canvases:write` scope and channel membership, since canvas access is the usual cause and the run log is the only place it surfaces
- `SentMessageInfo.Blocks` is marshalled by this package, not read back from slack-go: slack-go v0.21.1 and later marshal blocks only when building the request, so `UnsafeApplyMsgOptions` returns no `blocks` key
