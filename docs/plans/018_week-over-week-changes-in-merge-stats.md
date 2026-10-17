# Week-over-Week Changes in Merge Stats

date: 2026-10-17
status: draft

## Requirements

- Monday's merge stats line, from plan 017, shows each median's relative change against the Monday 7 days before: `Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge (+15 %, +38 %)`
- Goals:
  - Compare only against stats a Monday post computed exactly 7 days earlier
  - Last week's stats ride along in state, carried forward by every post
  - Plain text, like the rest of the line
- Non-goals:
  - A change for the merged count
  - Trends beyond one week
  - The canvas
  - Any new action input
  - Downloading a week-old artifact: the README and this repository set `retention-days: 4`, an expired artifact can't be downloaded, and each update run uploads one (385 in the dev channel). See docs/third-party-facts.md § GitHub's repository-level "List artifacts" has no sort parameter and documents no order
- Depends on plan 017 being implemented
- Serves AGENTS.md § Purpose: a direction, so the medians read as better or worse than last week instead of in isolation. In the reference deployment, about 5 to 15 merges a week
- Lands on a feature branch, through a PR

## Target Shape

- **Recent merge stats** (`State.MergeStatsByDate`, JSON key `mergeStatsByDate`, `omitempty`): `map[string]mergestats.MergeStats`, keyed by the UTC date (`2006-01-02`) of each entry's `ComputedAt`
  - Every post saves the loaded map, plus its own stats on a merge stats day, minus entries dated more than 7 days before its run. A Sunday post still carries the previous Monday's entry, and the map holds at most 2
  - Update runs carry it unchanged, through the existing `With*` copies
- **Week before**: the entry keyed 7 days before the date of the shown stats' `ComputedAt`. Absent when that Monday computed nothing, or its state didn't reach this run
- **Percent change**: `(shown − week before) / week before`, from the stored durations, never the rounded text. Rounded to a whole percent, rendered `+15 %`, `-12 %`, `0 %`
  - Only for a median both weeks have, with a week-before value above 0
  - The parenthetical shows only when every median the line shows has a change
- The merge stats line:
  - `Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge (+15 %, +38 %)`
  - `Last 7 days: 2 merged · median 1.0 days to merge (-20 %)` with no first review this week
  - `Last 7 days: 0 merged`: no change
  - Without a week before: the plan 017 line, unchanged
- Post mode loads the previous state before building its message, once, for both the week-before lookup and marking the previous message stale. Same warnings as today on a failed load
- A second Monday post on the same day finds the week before in the map, not the first post's entry from today
- No change to action inputs or permissions. No new API call: the post's state load moves earlier

### Message States

- Posted on Monday: the line with the parenthetical
- Edited by a later update run: the same line, the week before looked up again from the loaded map, above the update-time footer
- Marked stale by the next post: the stored line, parenthetical included

## Breaking Change

- Non-breaking, minor: a new part of an existing line, no input change
- A state saved before has no `mergeStatsByDate`. The first Monday after upgrading shows no change, the next one does
- A downgrade ignores the key: `state.Load` decodes with `json.Unmarshal`, which skips unknown keys

## Summary

1. `mergestats.PercentChanges`
2. `State.MergeStatsByDate` and its 7-day prune
3. The parenthetical in the message
4. Run wiring: post mode loads state first, every post carries the map, both modes look up the week before

## Steps

### Step 1: Percent changes

- `internal/mergestats/`: `PercentChanges(shown, weekBefore MergeStats)`, returning the first review and the merge change, each nil when not computable
- Table test: rounding, a week-before value of 0, a median missing in either week, a drop
- Update mergestats.spec.md

### Step 2: Recent merge stats in state

- `internal/state/`: add `State.MergeStatsByDate`
- `WithRecentMergeStats(state, loadedMergeStatsByDate, newMergeStats, asOf)`: adds the new entry when there is one, drops entries dated more than 7 days before `asOf`
- Package test: the prune boundary at 7 and 8 days
- Update state.spec.md § Behaviour, and § Oddities: added without a schema bump

### Step 3: The parenthetical

- `internal/messagecontent/`: `Content.MergeStatsWeekBefore`, passed through by `GetContent`
- `internal/messagebuilder/`: appends the parenthetical per § Target Shape
- Update messagecontent.spec.md and messagebuilder.spec.md § Behaviour

### Step 4: Wire the week before into the run

- `cmd/pr-slack-reminder/`:
  - `Run` loads the previous state before the merged fetch in post mode too. `markPreviousMessageStale` takes the loaded state and its load error instead of loading
  - Post mode passes no merged PR cache to the merged fetch, as today
  - Post mode on a merge stats day looks up the week before in the loaded map. Every saved post state goes through `WithRecentMergeStats`
  - Update mode looks up the week before off the loaded `MergeStats.ComputedAt`
- Snapshots, read before re-recording:
  - Monday post with a week before
  - The same message edited by an update run
  - Monday post without a week before
- `main_test.go`:
  - A non-Monday post carries the map and prunes it
  - A second Monday post compares against the week before, not today's entry
  - An entry 6 or 8 days old is not used
- Update run.spec.md § Behaviour, AGENTS.md § Architecture's state line, and README's line description
- Live check, after release, on the reference workflow's channel: the second Monday's line shows the parenthetical. The dev channel's daily posts run the released binary, so a branch build can't keep its state chain for a week

## Consequences

### Positive

- A weekly direction for both medians, at no API cost

### Negative

None

### Caveats

- A week of 1 or 2 merges swings the change widely
- A broken state chain, no saved run for over 4 days at the README's retention, loses the week before for one Monday
- A week with no first review, after one that had one, shows no parenthetical at all

### Neutral

- State carries up to 2 small entries
- Post mode loads state earlier in the run, which changes the log line order
