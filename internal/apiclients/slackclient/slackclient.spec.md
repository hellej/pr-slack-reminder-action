# slackclient

Sends PR reminder messages to Slack and replaces PR tracker canvas content.

## Behaviour

- `Client.GetChannelIDByName` resolves a Slack channel ID by name, searching public then private conversations
- `Client.SendMessage` posts a new Block Kit message; `UpdateMessage` edits an existing one by timestamp; `DeleteMessage` removes one
- `Client.ReplaceCanvasContent` replaces a canvas's whole content with a markdown string, via one `canvases.edit` call with a `replace` change carrying no section ID. Requires the `canvases:write` scope and canvas access, which the bot gets implicitly when the canvas is a tab in a channel it is in
- Send/update calls return `SentMessageInfo`: channel ID, timestamp, and the block array as sent
- `UpdateMessage` wraps Slack's `message_not_found`, `cant_update_message` and `edit_window_closed` in `ErrMessageNotEditable`: the message is gone or can no longer be edited ([chat.update](https://docs.slack.dev/reference/methods/chat.update)). `WrapUpdateMessageError` holds that mapping, exported so the mock shares it
- `SendMessage`, `UpdateMessage`, `DeleteMessage` and `ReplaceCanvasContent` retry a transient failure under [internal/apiclients/retry](../retry/retry.spec.md)'s `DefaultPolicy`, each attempt under its own deadline. The log line names the call: `Slack post`, `Slack update`, `Slack delete`, `Slack canvas edit`
- Transient for update, delete and canvas edit, idempotent calls Slack can safely receive twice:
  - No response, the attempt deadline included
  - A 429 or a 5xx, and the error codes `ratelimited`, `internal_error`, `fatal_error`, `service_unavailable`, `request_timeout`
- Transient for a post, only where Slack surely did not post: a 429, the error code `ratelimited`, and a failed connection (DNS or connection refused). `chat.postMessage` has no idempotency key. See docs/third-party-facts.md § `chat.postMessage` has no idempotency key, and `internal_error` or `fatal_error` may follow a partial success
- `MarshalBlocksAsSent` returns a message's block array as sent: slack-go's form sender marshals the same `BlockSet` at request time, for posts and edits alike (slack-go v0.29.0 `chat.go`)

## Doesn't Do

- `SendMessage` doesn't chunk oversized messages; it errors if given more than 50 blocks
- Never retries a post after an ambiguous failure: a 5xx, an attempt deadline, `internal_error`, `fatal_error`, or a connection cut after sending
- Doesn't read `Retry-After`: a rate limit gets the policy's fixed waits
- `GetChannelIDByName` is not retried
- No canvas creation, deletion, lookup or access granting; the canvas is created by the user and addressed by ID
- `ReplaceCanvasContent` doesn't split oversized content; Slack's canvas size limit fails the call

## Oddities

- `GetChannelIDByName`'s error message differs depending on whether the public or private channel listing failed, and suggests using the channel-ID input instead
- `DeleteMessage` treats a Slack "message not found" error as success (already-deleted is not an error), also on a retry after an attempt that deleted the message but failed
- `UpdateMessage` matches the error code on slack-go's `slack.SlackErrorResponse` type, its `Err` the code (slack-go v0.29.0 `misc.go` `SlackResponse.Err`), while `DeleteMessage` matches `message_not_found` anywhere in the error text
- A failed `ReplaceCanvasContent` wraps the Slack error with a fixed hint about the `canvases:write` scope and channel membership, since canvas access is the usual cause and the run log is the only place it surfaces
- `SentMessageInfo.BlocksAsSent` is marshalled by this package, not read back from slack-go: slack-go v0.21.1 and later marshal blocks only when building the request, so `UnsafeApplyMsgOptions` returns no `blocks` key
- A 429 with an unparseable `Retry-After` is not retried: slack-go returns the parse error instead of a rate-limit error (slack-go v0.29.0 `misc.go` `checkStatusCode`)
