# run

Orchestrates one action run: fetches PRs once, builds and sends the message, refreshes the PR
tracker canvas, and persists state. The two run modes and the pipeline order are in
[AGENTS.md](../../AGENTS.md).

## Behaviour

- `Run(getGitHubClient, getSlackClient)` is the action's entrypoint. It loads config, resolves the Slack channel ID by name when only that is configured, fetches open and recently-merged PRs once, dispatches to post or update mode, refreshes the canvas when configured, and persists state
- The open-PR fetch and the merged-PR fetch (bounded by `githubclient.RecentlyMergedWindow` before `generatedAt`) share one `generatedAt`, used for both the merged-PR window and the canvas footer
- A failed merged-PR fetch does not fail the run: the message and canvas publish without merged rows, and the error is carried through so it can still reach the run's exit code and, in update mode, affect whether the message may be deleted
- Post mode builds the message from the run's own open-PR fetch (drafts dropped) and the merged fetch, sends it, and returns the state to persist, nil when there is nothing to send, so nothing is written and no message goes out
- Update mode loads the previous state, re-resolves each tracked PR ref the run's own fetches didn't already resolve, builds the message from the live open fetch plus the resolved tracked and merged PRs, and edits the existing message. It deletes the message only when there is nothing left to show and both the merged and residue fetches succeeded; a fetch failure keeps the message standing instead
- Post mode passes a zero `messagePostedAt` to `messagecontent.GetContent` (no message posted yet); update mode passes the loaded state's `CreatedAt`, the previous message's post time
- The message send and the canvas refresh are independent: one failing does not skip the other, and both errors are joined into the run's return value
- The canvas refresh hashes the markdown it would write, footer timestamp excluded, and skips the write when the hash matches the previous run's, since Slack's canvas API can mis-merge a replace that lands while someone has the canvas open. It still carries the merged-fetch error alongside a skipped or successful write
- State is only saved when a message was actually posted or updated, carrying whichever canvas content hash is now current

## Doesn't Do

- Doesn't retry a GitHub or Slack call: a fetch or send failure past its timeout is final for that run
- Doesn't fail the whole run because the canvas refresh failed, or vice versa: each surfaces its own error independently
- Doesn't distinguish, for its own caller, "empty because there is truly nothing" from "empty because a fetch failed": that distinction only changes whether update mode deletes the message

## Oddities

- A tracked PR ref no fetch resolved is fetched again individually, but a ref already resolved by the merged fetch counts as tracked, so it is exempt from the merged section's cap on untracked PRs
- Drafts are only included in the open-PR fetch when the canvas is enabled, then filtered back out before the message is built, so a draft-carrying fetch never reaches the message even though it reached the canvas
- Deleting the update-mode message needs proof nothing is left to show: a failed merged or residue fetch reads as unknown, not empty, so the message survives until a run that can rebuild it says otherwise
- A failed canvas write still reports the previous content hash, so the next run retries the same write rather than treating it as already applied
