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
  - The merge stats left out: their merged PR fetch failing
  - A merged PR in the merge stats fetch without complete reviewer info, one warning per PR naming it. It stays in the line's merged count
- `Run(getGitHubClient, getSlackClient, isMergeStatsDay)` is the action's entrypoint. It loads config, resolves the Slack channel ID by name when only that is configured, fetches open and recently-merged PRs once, dispatches to post or update mode, refreshes the canvas when configured, and persists state
- Both modes load the previous state once, after the open-PR fetch and before the merged-PR fetch. In update mode, a load error fails the run, but only after the merged fetch and the canvas refresh have run
- Post mode loads it whether or not it then sends. A missing artifact logs, and any other load error warns, before the send: post mode goes on without the previous state
- Update mode passes the loaded merged PR cache to the merged fetch, which returns a cached PR instead of enriching it. Post mode passes none, so it enriches every merged PR
- With `show-merged-prs-in-message` false, the run skips the work only the message's merged section needs:
  - the merged-PR fetch, unless the canvas is enabled, which still lists merged PRs
  - in update mode, resolving the tracked PRs, `GetPRs` included
- With `show-merged-prs-in-message` false, update mode's delete decision ignores a failed merged fetch: it doesn't keep a message with nothing left to show
- The open-PR fetch and the merged-PR fetch (bounded by `githubclient.RecentlyMergedWindow` before `generatedAt`) share one `generatedAt`, used for both the merged-PR window and the canvas footer
- A failed merged-PR fetch does not fail the run: the message and canvas publish without merged rows, and the error is carried through so it can still reach the run's exit code and, in update mode, affect whether the message may be deleted
- `main` passes `isMergeStatsDay` true when the run starts on a Monday in UTC
- Post mode computes the merge stats when `show-merge-stats-in-message` is `true`, or when it is unset on a merge stats day. `false` never does. Update mode never reads the input
- Post mode uses that one load for the merge stats window, the week before and marking the previous message stale
- A post computing the stats fetches every PR merged in the merge stats window through `FindPRsMergedSince`, computes the [internal/mergestats](../../internal/mergestats/mergestats.spec.md) stats with the window start and `generatedAt`, and logs them. The message carries them as its merge stats line and the new state saves them
  - The window starts at the loaded state's `MergeStatsLastComputedAt`, at most `githubclient.RecentlyMergedWindow` (7 days) before `generatedAt`, and at 7 days when the state did not load or has no such time. The recently merged fetch keeps its 7 days either way
  - A failed stats fetch leaves the merge stats out with a warning. The run goes on
  - A merged PR without complete reviewer info stays in the merged count and out of both medians, per [internal/mergestats](../../internal/mergestats/mergestats.spec.md), with a warning naming it
  - The new state's `MergeStatsLastComputedAt` is the new stats' `ComputedAt`, else the loaded state's value. A failed stats fetch keeps the loaded value too
- Update mode renders the loaded state's merge stats, when it has them, and saves them back with the state, so the line stays on the message until the next post replaces it
- The line compares against the week before: the entry of the loaded state's `MergeStatsByDate` dated 7 days before the shown stats' `ComputedAt`, through `state.MergeStatsWeekBefore`. See [internal/state](../../internal/state/state.spec.md)
  - Post mode looks up the week before for the stats it computed. A second post the same day compares against the entry 7 days back, not against the first post's entry from today. A post without a loaded previous state has no week before
  - Update mode looks up the week before off the `ComputedAt` of the loaded `MergeStats`, not off the run's clock, so an update on a later day still finds it
- Every post state saved goes through `state.WithRecentMergeStats`, with the loaded map, the post's own stats (nil when it computes none, or leaves them out) and `generatedAt`. So a post on any day carries the map forward and prunes it. Update mode saves the map back unchanged
- The merge stats line never counts as something to show: a post with nothing else sends nothing, and an update with nothing else deletes the message. It shows with `show-merged-prs-in-message` false too
- Post mode builds the message from the run's own open-PR fetch (drafts dropped) and the merged fetch, without a footer, sends it, and returns the state to persist, nil when there is nothing to send, so nothing is written and no message goes out
- After a successful send, post mode marks the previous post's message stale: it edits the message recorded in the loaded previous state into the message marked stale (see [internal/messagebuilder](../../internal/messagebuilder/messagebuilder.spec.md)), and sends the stored summary text with the edit. The marked-stale message opens with a staleness warning and drops any update-time footer an update run gave it
  - Skips when there is no previous state, when it does not load, when it records no sent message, or when Slack says the message cannot be edited (`slackclient.ErrMessageNotEditable`)
  - Any other failure joins the run's error. The new state is still saved
  - Marks only a previous message in the channel the new message went to, both channel IDs as Slack returned them from the send. Another channel's message belongs to another setup sharing the state artifact name, so it skips with a log line naming both channels
  - The mark-as-stale edit never reaches the sent-blocks record, which holds the new message only
- Update mode re-resolves each tracked PR ref the run's own fetches didn't already resolve, from the merged PR cache first, then through `GetPRs`, when the merged section is shown. It builds the message from the live open fetch plus the resolved tracked and merged PRs, and edits the existing message. It deletes the message only when there is nothing left to show and, with the merged section shown, both the merged and residue fetches succeeded; such a fetch failure keeps the message standing instead
- Post mode passes a zero `messagePostedAt` to `messagecontent.GetContent` (no message posted yet); update mode passes the loaded state's `MessagePostedAt`
- The message send and the canvas refresh are independent: one failing does not skip the other, and both errors are joined into the run's return value
- Each part of the canvas refresh error, the write failure and the merged fetch failure, carries its own `PR tracker canvas refresh failed: ` prefix and stays a separate part of the join, so each is its own annotation
- The canvas refresh hashes the markdown it would write, footer timestamp excluded, and skips the write when the hash matches the previous run's, since Slack's canvas API can mis-merge a replace that lands while someone has the canvas open. It still carries the merged-fetch error alongside a skipped or successful write
- State is saved carrying whichever canvas content hash is now current:
  - Post mode saves it only when the message was sent, recording the new message, caching its merged PRs, and saving the recent merge stats through `state.WithRecentMergeStats`
  - Update mode saves it whenever the previous state loaded
    - After a successful edit: it records the edited message, adds the PRs the edit listed to the tracked PRs, and replaces the merged PR cache with the merged and tracked PRs this run used. See [internal/state](../../internal/state/state.spec.md) for which of them are kept
      - Also when saving the sent blocks then fails
    - A deleted or kept message, or a failed edit: it saves the loaded state back unchanged

## Doesn't Do

- Doesn't fail the whole run because the canvas refresh failed, or vice versa: each surfaces its own error independently
- Doesn't mark the previous message stale when post mode sends nothing: it keeps reading as current
- Doesn't tell a scheduled post from a manual one: every post on a Monday in UTC computes the merge stats, unless the input says otherwise
- Doesn't tell apart two setups posting to the same channel under one state artifact name: each marks the other's message stale
- Doesn't annotate an expected skip: no state artifact yet, no stored message in the previous state, the previous message in another channel, update mode keeping its message after a failed fetch (the fetch's own warning covers it)
- Doesn't distinguish, for its own caller, "empty because there is truly nothing" from "empty because a fetch failed": that distinction only changes whether update mode deletes the message

## Oddities

- A tracked PR ref no fetch resolved is fetched again individually, but a ref already resolved by the merged fetch counts as tracked, so it is exempt from the merged section's cap on untracked PRs
- Drafts are only included in the open-PR fetch when the canvas is enabled, then filtered back out before the message is built, so a draft-carrying fetch never reaches the message even though it reached the canvas
- Deleting the update-mode message needs proof nothing is left to show: a failed merged or residue fetch reads as unknown, not empty, so the message survives until a run that can rebuild it says otherwise
- With the canvas on, a failed merged fetch shows on the run page twice: as a warning, and as an error part of the failed canvas refresh
- A merged row taken from the cache doesn't show reviews, comments or title edits made after it was cached
- A tracked PR taken from the cache skips the current filters until the next post
- A failed merged search drops its untracked PRs from the saved cache, so the next run re-enriches them
- With `show-merged-prs-in-message` false, the saved merged PR cache holds only what the merged fetch returned, nothing when the canvas is off. Tracked PR refs are still saved and extended, so a later run showing the section again lists their merges, re-fetching what the cache lacks
- A post fetches the merge stats, and loads the previous state, before it knows whether it has anything to send, so a post that sends nothing still spends both
- Two posts computing the stats hours apart give the second a window of hours, read as `Last 1 day`
- A post that computes stats but sends nothing saves no state, so its stats are lost, and the next window starts at the last saved stats time. The recent merge stats it loaded stay only in the state artifact it loaded them from, until that artifact expires
- When later posts send nothing, Monday's merge stats line stays on a message read days later
- The week before's window can differ in length from the shown one, such as after `show-merge-stats-in-message` changes. The medians are still compared
- A team far east of UTC posting early on its Monday posts on a UTC Sunday, so its line lands on its Tuesday post
- A failed canvas write still reports the previous content hash, so the next run retries the same write rather than treating it as already applied
