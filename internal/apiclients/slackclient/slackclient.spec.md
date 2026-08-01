# slackclient

Sends PR reminder messages to Slack.

## Behaviour

- `Client.GetChannelIDByName` resolves a Slack channel ID by name, searching public then private conversations
- `Client.SendMessage` posts a new Block Kit message; `UpdateMessage` edits an existing one by timestamp; `DeleteMessage` removes one
- Send/update calls return `SentMessageInfo` (channel ID, timestamp, and the JSON actually sent) — used by [internal/state](../../state/state.spec.md) to record what was posted

## Doesn't Do

- `SendMessage` doesn't chunk oversized messages; it errors if given more than 50 blocks
- No retries or rate-limit backoff on API errors

## Oddities

- `GetChannelIDByName`'s error message differs depending on whether the public or private channel listing failed, and suggests using the channel-ID input instead
- `DeleteMessage` treats a Slack "message not found" error as success (already-deleted is not an error)
