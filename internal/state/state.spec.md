# state

Persists and reloads the tracked PRs, Slack message reference, last written message, merged PR cache and recent merge stats, so "update" mode can find and edit that message without re-enriching merged PRs, and the next "post" can mark it stale and compare its merge stats against the week before. See [AGENTS.md](../../AGENTS.md) for the two run-modes' overall flow.

## Behaviour

- State carries a schema version, `MessagePostedAt`, `MessageRef` (the sent Slack message's channel/timestamp), the tracked PRs (every PR the message has listed in an open section since it was posted), the hash of the markdown last written to the PR tracker canvas, and `LastWrittenMessage`: the message's blocks as last sent or edited, its summary text, and the `generatedAt` its content was built at
- State also carries `MergedPRCache`, merged PRs as enriched, and `MergeStats`, the [internal/mergestats](../mergestats/mergestats.spec.md) value the post computed, nil when it computed none
- `MergeStatsByDate` holds the recent merge stats, keyed by the UTC date (`2006-01-02`) of each entry's `ComputedAt`, so a post computing stats finds the week before
- `Load()` fetches the most recent saved state for a repository (via a GitHub Actions artifact)
- `NewPostState()` builds state from a "post" run's PR views, Slack send result, summary text, `generatedAt`, merge stats and `MergeStatsLastComputedAt`. The only place stamping the schema version and `MessagePostedAt`, and it leaves the canvas hash empty for the caller to fill in
- `WithLastWrittenMessage()` returns a copy of a state with `LastWrittenMessage` replaced by an edit's blocks, summary text and `generatedAt`
- `WithTrackedPRsAdded()` returns a copy of a state with an edit's listed PR views added to the tracked PRs: loaded refs keep their order, refs not already tracked append in the views' order
- `WithMergedPRCache()` returns a copy of a state with `MergedPRCache` replaced by the given PRs that are merged and have complete reviewer info, one per ref, the first kept
- `WithRecentMergeStats(state, loadedMergeStatsByDate, newMergeStats, asOf)` returns a copy of a state with `MergeStatsByDate` set to the loaded entries plus `newMergeStats` when not nil and the loaded map has no entry of its date: the day's first stats stay, as a later post that day covers only the hours since. It drops every entry dated more than 7 days before the UTC date of `asOf`, so one exactly 7 days before stays. It leaves the loaded map unchanged
- `MergeStatsWeekBefore(mergeStatsByDate, shown)` returns the entry dated exactly 7 days before the UTC date of `shown.ComputedAt`. Nil when `shown` is nil or there is no such entry
- `MergeStats` is what this state's message shows. `MergeStatsLastComputedAt` is when any post last computed stats, where the next window starts
  - When `MergeStats` is set, the two come from the same fetch: `MergeStatsLastComputedAt` equals `MergeStats.ComputedAt`
  - A post computing no stats, or whose stats fetch failed, saves nil `MergeStats` and carries `MergeStatsLastComputedAt` forward from the loaded state
  - `NewPostState` stores both as given. The run picks them: see [run.spec.md](../../cmd/pr-slack-reminder/run.spec.md) § Behaviour
- The other `With*` copies carry `MergeStats`, `MergeStatsByDate` and `MergeStatsLastComputedAt` through unchanged
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
- `MergedPRCache` (JSON key `mergedPRCache`) was added the same way. An artifact saved before it decodes no cache, and an empty cache saves no key
- `MergeStats` (JSON key `mergeStats`) was added the same way. An artifact saved before it decodes no stats, and nil stats save no key
- `MergeStatsByDate` (JSON key `mergeStatsByDate`) was added the same way. An artifact saved before it decodes no entries, and an empty map saves no key
- `MergeStatsLastComputedAt` (JSON key `mergeStatsLastComputedAt`) was added the same way. An artifact saved before it decodes nil, and nil saves no key
- An update run rewrites only `LastWrittenMessage`, the canvas hash, the tracked PRs and the merged PR cache. `MessageRef`, `MessagePostedAt`, `MergeStats`, `MergeStatsByDate` and `MergeStatsLastComputedAt` stay the post run's
- `Save()` indents the stored blocks with the rest of the file, so they load back with the same JSON but not the same bytes as sent
- `MessagePostedAt`, `MessageRef` and `TrackedPRs` save as `messagePostedAt`, `messageRef` and `trackedPRs`. Loading still reads the keys older releases wrote, `createdAt`, `slackMessage` and `pullRequests`, when the new key is absent or empty. A later release drops that fallback
- A PR ref's repository saves as `"Owner"` and `"Name"`, capitalised: `models.Repository` has no JSON tags. A merged PR cache entry is a `githubclient.PR` saved the same way, its keys its Go field names. The state snapshots in `cmd/pr-slack-reminder` pin these keys. Renaming one of those Go fields decodes it empty from older state files
