# run

Orchestrates one action run: fetches PRs once, builds and sends the message, refreshes the PR
tracker canvas, persists state, and reports problems as annotations on the run page. The two
run modes and the pipeline order are in
[AGENTS.md](../../AGENTS.md).

## Behaviour

- `main` sends all log output to stdout, the stream for workflow commands, so log lines and annotations share one ordered stream. It sets no log line prefix: the runner reads a command only at a line's start
- A failing run writes one `::error` annotation per part of its joined error, then exits 1. An escaped newline shows as a space on the run page, running the parts together. See [docs/third-party-facts.md](../../docs/third-party-facts.md) § An escaped newline is a line break in the job log and the API, but a space on the run's summary page
  - Parts are split recursively through nested joins. An error wrapping several errors in one `fmt.Errorf` message stays one annotation, since splitting it would drop its own text
- Each annotation line reads `::<warning|error> title=PR Slack Reminder::<message>`, with no `file`, `line` or `col`. The message escapes `%`, CR and LF as `%25`, `%0D` and `%0A`
- A `::warning` annotation replaces the log line for each of these, none failing the run:
  - The previous state failing to load in post mode, for any reason but a missing artifact (`githubclient.ErrNoArtifactFound`)
  - The previous message not editable in post mode
  - The recently merged PR fetch failing, in either mode
  - The tracked PR fetch failing in update mode
  - The delete failing in update mode
- `Run(getGitHubClient, getSlackClient)` is the action's entrypoint. It loads config, resolves the Slack channel ID by name when only that is configured, fetches open and recently-merged PRs once, dispatches to post or update mode, refreshes the canvas when configured, and persists state
- The open-PR fetch and the merged-PR fetch (bounded by `githubclient.RecentlyMergedWindow` before `generatedAt`) share one `generatedAt`, used for both the merged-PR window and the canvas footer
- A failed merged-PR fetch does not fail the run: the message and canvas publish without merged rows, and the error is carried through so it can still reach the run's exit code and, in update mode, affect whether the message may be deleted
- Post mode builds the message from the run's own open-PR fetch (drafts dropped) and the merged fetch, without a footer, sends it, and returns the state to persist, nil when there is nothing to send, so nothing is written and no message goes out
- After a successful send, post mode marks the previous post's message stale: it loads the previous state and edits that message into [internal/messagebuilder](../../internal/messagebuilder/messagebuilder.spec.md)'s message marked stale, which opens with a staleness warning and drops the update-time footer an update run gave it, with the stored summary text
  - Skips when there is no previous state, when it does not load, when it records no sent message, or when Slack says the message cannot be edited (`slackclient.ErrMessageNotEditable`)
  - Any other failure joins the run's error. The new state is still saved
  - Marks only a previous message in the channel the new message went to, both channel IDs as Slack returned them from the send. Another channel's message belongs to another setup sharing the state artifact name, so it skips with a log line naming both channels
  - The mark-as-stale edit never reaches the sent-blocks record, which holds the new message only
- Update mode loads the previous state, re-resolves each tracked PR ref the run's own fetches didn't already resolve, builds the message from the live open fetch plus the resolved tracked and merged PRs, and edits the existing message. It deletes the message only when there is nothing left to show and both the merged and residue fetches succeeded; a fetch failure keeps the message standing instead
- Post mode passes a zero `messagePostedAt` to `messagecontent.GetContent` (no message posted yet); update mode passes the loaded state's `MessagePostedAt`
- The message send and the canvas refresh are independent: one failing does not skip the other, and both errors are joined into the run's return value
- Each part of the canvas refresh error, the write failure and the merged fetch failure, carries its own `PR tracker canvas refresh failed: ` prefix and stays a separate part of the join, so each is its own annotation
- The canvas refresh hashes the markdown it would write, footer timestamp excluded, and skips the write when the hash matches the previous run's, since Slack's canvas API can mis-merge a replace that lands while someone has the canvas open. It still carries the merged-fetch error alongside a skipped or successful write
- State is saved carrying whichever canvas content hash is now current:
  - Post mode saves it only when the message was sent, recording the new message
  - Update mode saves it whenever the previous state loaded. After a successful edit it records the edited message; when the message is deleted or kept, or the edit fails, the loaded state is saved back unchanged

## Doesn't Do

- Doesn't retry a GitHub or Slack call: a fetch or send failure past its timeout is final for that run
- Doesn't fail the whole run because the canvas refresh failed, or vice versa: each surfaces its own error independently
- Doesn't mark the previous message stale when post mode sends nothing: it keeps reading as current
- Doesn't tell apart two setups posting to the same channel under one state artifact name: each marks the other's message stale
- Doesn't annotate an expected skip: no state artifact yet, no stored message in the previous state, the previous message in another channel, update mode keeping its message after a failed fetch (the fetch's own warning covers it)
- Doesn't distinguish, for its own caller, "empty because there is truly nothing" from "empty because a fetch failed": that distinction only changes whether update mode deletes the message

## Oddities

- A tracked PR ref no fetch resolved is fetched again individually, but a ref already resolved by the merged fetch counts as tracked, so it is exempt from the merged section's cap on untracked PRs
- Drafts are only included in the open-PR fetch when the canvas is enabled, then filtered back out before the message is built, so a draft-carrying fetch never reaches the message even though it reached the canvas
- Deleting the update-mode message needs proof nothing is left to show: a failed merged or residue fetch reads as unknown, not empty, so the message survives until a run that can rebuild it says otherwise
- With the canvas on, a failed merged fetch shows on the run page twice: as a warning, and as an error part of the failed canvas refresh
- A failed canvas write still reports the previous content hash, so the next run retries the same write rather than treating it as already applied
