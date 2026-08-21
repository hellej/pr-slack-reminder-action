# slackclient

Sends PR reminder messages to Slack and replaces PR tracker canvas content.

## Behaviour

- `Client.GetChannelIDByName` resolves a Slack channel ID by name, searching public then private conversations
- `Client.SendMessage` posts a new Block Kit message; `UpdateMessage` edits an existing one by timestamp; `DeleteMessage` removes one
- `Client.ReplaceCanvasContent` replaces a canvas's whole content with a markdown string, via one `canvases.edit` call with a `replace` change carrying no section ID. Requires the `canvases:write` scope and canvas access, which the bot gets implicitly when the canvas is a tab in a channel it is in
- Send and update both disable link and media unfurling, so the canvas link in the message footer doesn't expand into a preview card
- Send/update calls return `SentMessageInfo` (channel ID, timestamp, and the JSON actually sent) — used by [internal/state](../../state/state.spec.md) to record what was posted

## Doesn't Do

- `SendMessage` doesn't chunk oversized messages; it errors if given more than 50 blocks
- No retries or rate-limit backoff on API errors
- No canvas creation, deletion, lookup or access granting; the canvas is created by the user and addressed by ID
- `ReplaceCanvasContent` doesn't split oversized content; Slack's canvas size limit fails the call

## Oddities

- `GetChannelIDByName`'s error message differs depending on whether the public or private channel listing failed, and suggests using the channel-ID input instead
- `DeleteMessage` treats a Slack "message not found" error as success (already-deleted is not an error)
- `chat.update` documents neither unfurl parameter, so the two options are presumed inert there; they are passed anyway rather than branching per send mode
- A failed `ReplaceCanvasContent` wraps the Slack error with a fixed hint about the `canvases:write` scope and channel membership, since canvas access is the usual cause and the run log is the only place it surfaces
