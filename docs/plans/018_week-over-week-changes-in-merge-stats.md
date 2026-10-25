# Week-over-Week Changes in Merge Stats

date: 2026-10-17
status: draft

## Requirements

- The merge stats line, from plan 017, shows each median's relative change against the stats a post computed 7 days before: `Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge (+15 %, +38 %)`
- Goals:
  - Compare only against stats a post computed exactly 7 days earlier: on Mondays by default, on any day with `show-merge-stats-in-message` true
  - Last week's stats ride along in state, carried forward by every post that saves state
  - Plain text, like the rest of the line
- Non-goals:
  - A change for the merged count
  - Trends beyond one week
  - The canvas
  - Any new action input. Plan 017's `show-merge-stats-in-message` decides which posts compute the stats
  - Matching the two windows' lengths
  - Downloading a week-old artifact: state artifacts expire after 4 days (`retention-days: 4`)
- Depends on plan 017 being implemented, including its `show-merge-stats-in-message` input and the window since the previous stats
- Serves AGENTS.md § Purpose: a direction, so the medians read as better or worse than last week instead of in isolation. In the reference deployment, about 5 to 15 merges a week
- Lands on a feature branch, through a PR

## Target Shape

- **Recent merge stats** (`State.MergeStatsByDate`, JSON key `mergeStatsByDate`, `omitempty`): `map[string]mergestats.MergeStats`, keyed by the UTC date (`2006-01-02`) of each entry's `ComputedAt`
  - The state a post saves holds the loaded map, plus the post's own stats when it computes them and the map holds no entry of that day, minus entries dated more than 7 days before the post's run. A Sunday post still carries the previous Monday's entry. The map holds at most 2 entries by default, 8 when every post computes the stats
  - Update runs carry it unchanged, through the existing `With*` copies
- **Week before**: the entry keyed 7 days before the date of the shown stats' `ComputedAt`. Absent when no post computed stats that day, or when the state that post saved was not loaded in this run (expired artifact, failed load)
- **Percent change**: `(shown − week before) / week before`, from the stored durations, never the rounded text. Rounded to a whole percent, rendered `+15 %`, `-12 %`, `+0 %`
  - Only for a median both weeks have, with a week-before value above 0
  - The parenthetical shows only when every median the line shows has a change
- The merge stats line:
  - `Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge (+15 %, +38 %)`
  - `Last 7 days: 2 merged · median 1.0 days to merge (-20 %)` with no first review this week
  - `Last 7 days: 0 merged`: no change
  - Without a week before: the plan 017 line, unchanged
- Post mode loads the previous state before building its message, once, for both the week-before lookup and marking the previous message stale. Same warnings as today on a failed load
- A second post on the same day finds the week before in the map, not the first post's entry from today. The saved map keeps the first post's entry, because the second post's window covers only the hours since the first post
- No change to action inputs or permissions, and no GraphQL change. A post that sends nothing, or whose send fails, now warns on a failed state load. It loaded state before too, so its API calls don't change

### Message States

- Posted with the stats: the line with the parenthetical
- Edited by a later update run: the same line, the week before looked up again from the loaded map, above the update-time footer
- Marked stale by the next post: the stored line, parenthetical included

## Breaking Change

- Non-breaking, minor: a new part of an existing line, no input change
- A state saved before this change has no `mergeStatsByDate`. The first post to show a change is the one 7 days after the first post that saved its stats in the map
- A downgrade ignores the key: `state.Load` decodes with `json.Unmarshal`, which skips unknown keys

## Summary

- R1: post mode loads the previous state before building its message
- 1: `mergestats.PercentChanges`
- 2: `State.MergeStatsByDate` and its 7-day prune
- 3: the parenthetical in the message
- 4: run wiring: every post carries the map, both modes look up the week before

## Steps

### Step R1: Load post mode's previous state first

- `cmd/pr-slack-reminder/`: `Run` loads the previous state before the merged fetch in post mode too, as it does in update mode
  - Post mode turns the load result into the previous post's state or nil (`previousPostStateOrLogWhyMissing`), logging the skip or warning after the merged-PR fetch, before the stats fetch and the send, so a post that sends nothing warns too. Same texts as today
  - `markPreviousMessageStale` takes that loaded state instead of loading, and runs only when there is one
  - Post mode still passes no merged PR cache to the merged fetch: `Run` would pass the loaded cache whenever state is loaded, so `mergedPRCacheToReuse(runMode, loadedState)` tells the modes apart
- Behaviour changes only in the load order, and in a post that sends nothing, or whose send fails, now warning on a failed state load. Existing tests and snapshots pass unchanged
- Update run.spec.md § Behaviour: the load order, and the warning on a post that sends nothing

### Step 1: Percent changes

- `internal/mergestats/`: `PercentChanges(shown, weekBefore MergeStats)`, returning the first review and the merge change, each nil when not computable
- Table test: rounding, a week-before value of 0, a median missing in either week, a drop
- Update mergestats.spec.md

### Step 2: Recent merge stats in state

- `internal/state/`: add `State.MergeStatsByDate`
- `WithRecentMergeStats(state, loadedMergeStatsByDate, newMergeStats, asOf)`: adds the new entry when there is one and the day has none yet, drops entries dated more than 7 days before `asOf`
- `MergeStatsWeekBefore(mergeStatsByDate, shown)`: the entry dated 7 days before `shown.ComputedAt`, so the date key format stays in this package
- Package test: the prune boundary at 7 and 8 days, by UTC date for an `asOf` and a `ComputedAt` outside UTC
- Update state.spec.md § Behaviour, and § Oddities: added without a schema bump

### Step 3: The parenthetical

- `internal/messagecontent/`: `GetContent` takes a new `mergeStatsWeekBefore` parameter, right after `mergeStats`. The `Content.MergeStatsText` wording calls `mergestats.PercentChanges` on both and appends the parenthetical per § Target Shape. Plan 017 moved the line's wording here from `internal/messagebuilder/`, which renders the text as given
- Update messagecontent.spec.md § Behaviour

### Step 4: Wire the week before into the run

- `cmd/pr-slack-reminder/`:
  - Post mode computing the stats looks up the week before in the loaded map, through `state.MergeStatsWeekBefore`. Every saved post state goes through `WithRecentMergeStats`
  - Update mode looks up the week before off the loaded `MergeStats.ComputedAt`
- Snapshots, read before re-recording. The normaliser, which today rewrites values only, also rewrites the `mergeStatsByDate` keys to placeholders relative to the run, such as `<run date>` and `<run date - 7>`:
  - Monday post with a week before
  - An update run's edit a day after a Monday post: the shown stats were computed the day before the run, so the week before is dated 8 days before the run. A lookup off the run's clock (7 days back) misses it
  - Monday post without a week before: the existing merge stats snapshots, their messages unchanged, their states gaining the map
  - A second Monday post the same day, marking the first stale: the stale message keeps the parenthetical. The second post's window starts at the first post, so its own line has no medians, and its saved map keeps the first post's entry
- `main_test.go`:
  - A post computing no stats carries the map and prunes it
  - An entry 6 or 8 days old is not used, one 7 days old is, also next to today's entry from an earlier post
  - A Monday whose stats are left out carries the map without a new entry
- Update run.spec.md § Behaviour, AGENTS.md § Architecture's state line, README's line description, the `action.yml` descriptions of `github-token-for-state` and `state-artifact-name` with their README inputs rows, and the README `actions: read` comment: post now reads state for the merge stats too
- Live check, after release: `pr-reminder.yml`'s scheduled daily posts to the `github` dev channel, which run the binary committed on `main`
  - The first Monday after the release shows no change
  - Done when the second Monday's post shows the parenthetical, and an update run's edit of that post keeps it

## Consequences

### Positive

- A weekly direction for both medians, at no API cost

### Negative

None

### Caveats

- A week of 1 or 2 merges swings the change widely
- A broken state chain loses the week before for one post: no saved run for over 4 days at the README's retention, or a post whose state load failed or found no artifact
- A week with a first review median, after a week without one, shows no parenthetical at all

### Neutral

- State carries up to 2 small entries by default, up to 8 when every post computes the stats
- Post mode loads state earlier in the run, which changes the log line order
- A failed state load now warns on a post that sends nothing
