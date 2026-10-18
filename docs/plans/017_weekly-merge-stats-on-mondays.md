# Weekly Merge Stats on Mondays

date: 2026-10-17
status: draft

## Requirements

- Once a week, the team sees how fast its PRs got reviewed and merged, without a daily notification spent on it
- Goals:
  - Monday's message carries one line above the update-time footer: `Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge`
  - Computed once, by the post run on a Monday in UTC. Update runs read it from state and render it again, unchanged, for as long as Monday's message is the live one
  - Weekday time, as the old-PR check counts it: Saturdays and Sundays left out as whole UTC days. No working hours. The 7-day window stays calendar days
  - Every PR merged in the window counts, bot-authored ones included
  - An optional `show-merge-stats-in-message` input overrides the Monday default in post mode: `true` computes the stats on every post, `false` on none. Unset keeps the Monday default. Update mode ignores it
  - The line names its real window: the time since the previous merge stats, at most 7 days. `Last 1 day`, `Last 3 days`, `Last 7 days`
- Non-goals:
  - The canvas
  - Trends, targets or per-person numbers
  - Telling a scheduled post from a manual one: every post run on a Monday computes the stats
- Serves AGENTS.md § Purpose: a weekly feedback loop on review and merge speed, at the cost of one line on a message the team already gets. In the reference deployment, about 5 to 15 merges a week, a weekly Dependabot batch of about 5 among them
- Lands on a feature branch, through a PR

## Target Shape

- **Merge stats** (`mergestats.MergeStats`): over the PRs merged in the merge stats window:
  - `MergedCount`
  - `MedianTimeToFirstReview`: over the PRs with a first review. Nil when none has one
  - `MedianTimeToMerge`: nil when nothing merged
  - Both durations are weekday time from `PR.ReadyForReviewAt()`, the start a row's age uses, through `prview.WeekdayTimeBetween`. A first review before that time counts as 0
  - The line's text doesn't say weekdays. The README does, as it does for `old-pr-threshold-hours`
  - An even count takes the mean of the two middle values
  - `ComputedAt`: the post run's `generatedAt`. Plan 018 finds the stats a week before by it
  - `MergedSince`: the window's start, JSON key `mergedSince`. An update run re-renders the line's window from it, unchanged
- **Merge stats window**: from the later of the previous merge stats' `ComputedAt` (`State.MergeStatsLastComputedAt`) and `generatedAt` minus `githubclient.RecentlyMergedWindow` (7 days), to `generatedAt`
  - 7 days when no previous state loads, or it has no `mergeStatsLastComputedAt`
  - `State.MergeStatsLastComputedAt`, JSON key `mergeStatsLastComputedAt` with `omitempty`: the `ComputedAt` of the last stats a post computed. A post that computes stats sets it. Every other post, and a post whose stats fetch failed, carries the loaded state's value forward. Update runs carry it through the `With*` copies
- **Merge stats input** (`show-merge-stats-in-message`, `config.ContentInputs.ShowMergeStatsInMessage *bool`): unset or empty is nil. `true` or `false` parses as a bool, anything else fails the config like `show-merged-prs-in-message`
  - A post computes the stats when the input is `true`, or when it is unset on a merge stats day. `false` never does
  - Update mode never reads it: it renders the loaded state's `mergeStats`, as before
- **First review** (`githubclient.PR.FirstReviewAt`): the earliest submitted review by a person other than the PR author, out of the reviews `isSubmittedUserReview` already keeps: not pending, not a bot. Its time is `submittedAt`, else `createdAt`. A PR conversation comment is not a review
- **Merge stats day**: a run whose start falls on a Monday in UTC (`isMondayInUTC`)
- **Merge stats line**: a `context` block with block ID `merge_stats_line`, one plain `mrkdwn` element:
  - The window reads in whole days, `ComputedAt` minus `MergedSince` rounded to the nearest day, at least 1: `Last 1 day`, `Last 3 days`, `Last 7 days`. Examples below show a 7-day window
  - `Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge`
  - `Last 7 days: 2 merged · median 1.0 days to merge` when no merged PR had a first review
  - `Last 7 days: 0 merged` when nothing merged
  - A duration reads in whole minutes under an hour, in hours with one decimal under 24 hours, else in days with one decimal: `24 minutes`, `1 minute`, `4.0 hours`, `1.0 days`
- Where the line shows:
  - A post computing the stats (per the merge stats input) fetches every PR merged in the window, enriched and uncapped, computes the merge stats, renders the line and saves the stats in state under `mergeStats`
  - An update run renders the line whenever the loaded state has `mergeStats`, and saves it back with the state. The line stays on Monday's message while it is the live one, until a later post replaces it
  - A post not computing the stats saves no `mergeStats`
  - The line never counts as something to show: a post with nothing else sends nothing, and an update with nothing else left deletes the message, as today
  - A failed stats fetch, or a merged PR in it without complete reviewer info, leaves the line out, with a `::warning` annotation. The run goes on
- `show-merged-prs-in-message: false` hides the merged section only. The line shows either way
- One new optional input. No permission change

### Message States

Monday's post:

```
👀 Waiting for review
• Fix coach video calls drop on cellular handoff 🚨 7 days old by Lilo
🚀 Recently merged
• Add live tide and surf report widget merged 1 hour ago by Nani (✅ Jumba / 💬 Cobra)
Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge
```

Edited by a later update run:

```
👀 Waiting for review
• Fix coach video calls drop on cellular handoff 🚨 7 days old by Lilo
🚀 Recently merged
• Add live tide and surf report widget merged 1 hour ago by Nani (✅ Jumba / 💬 Cobra)
Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge
Live, updated 14:02
```

Marked stale by the next post, which drops the update-time footer and keeps the line:

```
⚠️ Stale, updated Yesterday at 14:02
👀 Waiting for review
• Fix coach video calls drop on cellular handoff 🚨 7 days old by Lilo
🚀 Recently merged
• Add live tide and surf report widget merged 1 hour ago by Nani (✅ Jumba / 💬 Cobra)
Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge
```

The line above the footer was probed live in the dev channel.

### GitHub API Budget

- Every run: both review selections gain `submittedAt` and `createdAt`. They add no connection, so the formula's cost is unchanged. See docs/third-party-facts.md § `PullRequestReview.submittedAt` is a nullable `DateTime`, and a scalar adds no GraphQL cost by the formula
- A Monday post adds one merged search, about 1 point per repository, and enriches every merge in the window, about 1 point per PR, the shown merged PRs a second time. See docs/third-party-facts.md § The GraphQL cost formula overestimated a search, but matches the enrichment query's logged cost of about 1 per PR: read `rateLimit.cost`
  - Reference deployment: about 10 to 20 points, once a week
  - With `show-merge-stats-in-message: true`, the same on every post, over a window of about 1 day, so fewer merges to enrich
- Post runs load the state artifact before the stats fetch rather than after sending: the same one REST download, no GraphQL cost
- Update runs: no change

## Breaking Change

- Non-breaking, minor: a new line on Monday's message, and one new optional input whose unset default keeps that behaviour
- State keeps its schema version. A state saved before this has no `mergeStats` key, so an update run on it shows no line. The next Monday post saves one
- A downgrade ignores the key: `state.Load` decodes with `json.Unmarshal`, which skips unknown keys

## Summary

1. `PR.FirstReviewAt`, read off review timestamps
2. `FindPRsMergedSince`: every merged PR in a window, enriched, uncapped
3. `internal/mergestats`: `MergeStats` and `FromMergedPRs`
4. `State.MergeStats`
5. The merge stats line in the message
6. Run wiring: the Monday post computes and saves the stats, update runs re-render them
7. The `show-merge-stats-in-message` input
8. The real window: since the previous merge stats, at most 7 days

## Steps

### Step 1: First review time on each PR

- `internal/apiclients/githubclient/`: both `reviews(last: 100)` selections gain `submittedAt createdAt`: `enrichedPullRequestSelection` (open and merged enrichment) and `fullPullRequestSelection` (`GetPRs`). `reviewNode` gains both, `submittedAt` as a pointer
- Add `PR.FirstReviewAt *time.Time`, set by `prWithReviewers` from the reviews `isSubmittedUserReview` keeps, minus the PR author's own
  - Take the minimum time, never the first node: `reviews(last: 100)` is the newest 100, in an order GitHub doesn't document
- `testhelpers/mockgithubclient/`: go-github's `PullRequestReview` has `SubmittedAt` but no `CreatedAt`, so `reviewNodeJSON` renders the fixture's `SubmittedAt` as both times
  - `NewReview` sets `SubmittedAt` to an hour before its call by default. A stats fixture overrides it on the returned review
- Package test, decoding raw JSON with the earliest review last: the earliest human review wins over an earlier bot review, an earlier self-review, an earlier pending review and a later human one, and a null `submittedAt` falls back to `createdAt`
- The existing query text tests pin `submittedAt createdAt` in both selections: the mock answers whatever a query asks
- Every cached merged PR now saves a `FirstReviewAt` key. The snapshot normaliser adds it to its time keys, and 12 `.state.json` snapshots re-record. Read the diff first
- Update githubclient.spec.md § Behaviour, and § Oddities: a PR with over 100 reviews takes its first review from the newest 100
- The Build E2E log shows the enrichment query's `rateLimit.cost` unchanged against the last `main` run. The PR states both

### Step 2: Fetch every PR merged in a window

- `internal/apiclients/githubclient/`: add `FindPRsMergedSince(ctx, repositories, getFiltersForRepository, mergedSince) ([]PR, error)` to `Client`. A method apart from `FindRecentlyMergedPRs`, since the message and canvas rely on that one's cap of 6
  - Same search, window cut, filters and newest-first order as `FindRecentlyMergedPRs`. No cap, no cache. Extract the shared part into one helper both call
  - Enriches every result through `enrichPRsWithReviewInfo`, in batches of 25. A query-scoped enrichment failure returns the error, unlike `FindRecentlyMergedPRs`, which falls back to unenriched rows
- Package test: more than `MaxMergedPRsToFetch` (6) merges in the window all come back enriched, in two enrichment batches when over 25
- Update githubclient.spec.md § Behaviour and § Doesn't Do: still capped at 100 merges per repository by the search

### Step 3: Merge stats package

- `internal/prview/`: export `weekdayTimeBetween` as `WeekdayTimeBetween`, the old-PR check's weekday time. Its existing tests cover it. Update prview.spec.md
- New `internal/mergestats/` with `mergestats.spec.md`
  - `MergeStats`: `MergedCount int`, `MedianTimeToFirstReview *time.Duration`, `MedianTimeToMerge *time.Duration`, `ComputedAt time.Time`, JSON keys `mergedCount`, `medianTimeToFirstReview`, `medianTimeToMerge`, `computedAt`, the pointers with `omitempty`
  - `FromMergedPRs(prs []githubclient.PR, computedAt time.Time) MergeStats`, pure, per § Target Shape
  - A PR with a zero `ReadyForReviewAt()` counts in `MergedCount` but in neither median: `WeekdayTimeBetween` walks from year 1 on a zero start, so its callers guard it, as `isWeekdayAgeOver` does
- Table test: odd and even counts, input out of order, a first review before ready for review, a PR readied on Friday and merged on Monday, PRs without a first review, none at all
- AGENTS.md § Architecture: list the package under "Around the run"

### Step 4: Merge stats in state

- `internal/state/`: add `State.MergeStats *mergestats.MergeStats`, JSON key `mergeStats` with `omitempty`
- `NewPostState` takes it. The `With*` copies carry it through unchanged
- Update state.spec.md § Behaviour, and § Oddities: added without a schema bump, like `MergedPRCache`

### Step 5: The merge stats line

- `internal/messagecontent/`: `Content.MergeStatsText`, the line's whole text per § Target Shape, built by `GetContent` from a new `mergeStats` parameter, ~2 callers. Empty without stats. It never feeds `HasPRs`
- `internal/messagebuilder/`: both message builders render `Content.MergeStatsText` as the merge stats line after the sections, before the update-time footer, when it is non-empty. The builder decides the block, not the wording
  - The content cap drops to 48 blocks, always, leaving a slot for the line and one for the footer or staleness warning
  - `BuildMessageMarkedStale` needs no change: it keeps every stored block but the update-time footer
- Package tests: messagecontent, the minutes-to-hours switch at 1 hour and the hours-to-days switch at 24 hours. messagebuilder, the 48-block cap with the line present, and no line for an empty text
- Update messagecontent.spec.md and messagebuilder.spec.md § Behaviour. messagebuilder § Oddities: grouped, 5 repositories with PRs in all four sections still fit at 44 content blocks, and 6 build 52, of which 4 are now dropped. messagecontent § Oddities: 59.6 minutes reads `60 minutes` and 23.96 hours `24.0 hours`

### Step 6: Wire the Monday stats into the run

- `cmd/pr-slack-reminder/`:
  - `Run` takes `isMergeStatsDay bool`. `main.go` passes `isMondayInUTC(time.Now())`. See [Justification § `Run` takes whether it is a merge stats day](#run-takes-whether-it-is-a-merge-stats-day)
  - Test call sites pass false, 51 of them, mechanical
  - `runPostMode` on a merge stats day calls `FindPRsMergedSince` over the window before `generatedAt`, then `mergestats.FromMergedPRs` with `generatedAt`, and logs the result. A fetch error, or a PR without `HasCompleteReviewerInfo`, logs a `::warning` and leaves the stats nil
  - `runPostMode` passes the stats to `GetContent` and `NewPostState`. Update mode passes the loaded state's
- `internal/state` copies already carry the stats through an update's save, so update mode writes nothing new
- Snapshots, read before re-recording. The normaliser adds `computedAt` to its time keys:
  - Post on a merge stats day: the line, with fixture reviews giving a review median and an even merge count
    - The tests read the real clock, so each fixture span is a whole number of weeks, or a review before the PR turned ready: the weekday time is then the same whatever day the test runs
  - Post on a merge stats day with nothing merged: `Last 7 days: 0 merged`
  - Update with stats in the loaded state: the line above the update-time footer
  - Post on a merge stats day with `show-merged-prs-in-message: false`: the line, no merged section
  - The Monday message, edited by an update run, then marked stale: the line kept, the footer dropped
- `main_test.go`:
  - A post on another day sends no stats fetch (`FetchRecording`) and saves no `mergeStats`
  - A failed stats fetch, and a merged PR without complete reviewer info, each warn, send the message without the line and save no `mergeStats`. The merged section is hidden and the canvas off, so the stats search is the run's only merged one
  - A Monday post with only the line to show sends nothing, and an update with only the line deletes the message
- `isMondayInUTC`: internal table test, Sunday 23:59 and Monday 00:00 UTC, plus a local weekday that differs from UTC's
- Update run.spec.md § Behaviour: the Monday fetch, the warning, the stats' path through state. README: one feature bullet, the line under the message description with its weekday time, and the `show-merged-prs-in-message` row reworded from "list open PRs only" to "hide the merged section"
- Live check, on a Monday in UTC: `gh workflow run pr-reminder.yml --ref <branch> -f run-mode=post -f build-first=true`, then the same with `run-mode=update`
  - Done when the post logs the computed stats, the dev channel's message shows the line, and the update keeps it above the footer
- Build E2E: two posts set `show-merge-stats-in-message: true`, so every PR's E2E runs the stats fetch live and its log shows its `rateLimit.cost`
  - The multi-repository run uploads no state, so its window is always 7 days and it enriches every merge in it
  - The post with filters and canvas link loads the previous E2E run's stats time, so pushes minutes apart give it an empty window. Its update run keeps the line

### Step 7: The `show-merge-stats-in-message` input

- `internal/config/`: `InputShowMergeStatsInMessage = "show-merge-stats-in-message"`, `ContentInputs.ShowMergeStatsInMessage *bool`. Add an `inputhelpers` reader returning nil for an unset or empty input, and the parse error for anything `strconv.ParseBool` rejects
- `action.yml`: the input, not required, no default. README inputs table: unset computes the stats on Monday posts only, `true` on every post, `false` never, post mode only. `go run .github/scripts/check_inputs.go` passes
- `cmd/pr-slack-reminder/run.go`: a post computes the stats when `shouldComputeMergeStats(cfg.ContentInputs.ShowMergeStatsInMessage, isMergeStatsDay)` holds. `Run` keeps its `isMergeStatsDay` parameter and hands it to `runPostMode`
- `testhelpers/confighelpers.go` leaves the input unset unless a test overrides it
- Tests:
  - `config_test.go`: unset, empty, `true`, `false`, and an invalid value failing
  - `main_test.go`, table-driven, post mode: unset, `true` and `false`, each on a merge stats day and on another day. Assert the stats fetch count (`FetchRecording`), the line shown or not, and `mergeStats` saved or not
  - `main_test.go`: an update run with `false` set and `mergeStats` in the loaded state still renders the line
- Update config.spec.md, inputhelpers.spec.md and run.spec.md § Behaviour

### Step 8: The real window

- `internal/mergestats/`: `MergeStats.MergedSince`. `FromMergedPRs` takes it
- `internal/state/`: `State.MergeStatsLastComputedAt *time.Time`. `NewPostState` takes it. The `With*` copies carry it
- `cmd/pr-slack-reminder/run.go`:
  - `runPostMode` loads the previous state once, before the stats fetch, and hands the state and the load error to `markPreviousMessageStale` instead of a second load. The outcomes for marking stale stay as they are. The artifact fetch's own log lines now print before the send; the stale-marking decision logs and the load warning stay after it
  - The window start per § Target Shape. `findPRsMergedSince` takes it in place of the fixed 7 days. `findRecentlyMergedPRs` keeps its 7 days
  - `NewPostState` gets the new stats' `ComputedAt` when stats were computed, else the loaded state's `MergeStatsLastComputedAt`
- `internal/messagecontent/`: the line's window text, `Last 1 day` singular, else `Last N days`
- Tests:
  - `messagecontent` package test: windows of 1 day, 2.4 days reading `Last 2 days`, 2.6 days reading `Last 3 days`, 7 days, and under 12 hours reading `Last 1 day`
  - `main_test.go`: previous state with `mergeStatsLastComputedAt` 1 day before the run: the stats fetch's `mergedSince` is that time, the line reads `Last 1 day`. 10 days before: capped, `Last 7 days`. Absent: `Last 7 days`
  - `main_test.go`: a post not computing stats carries `mergeStatsLastComputedAt` forward. A post computing stats saves its own `ComputedAt`. A failed stats fetch carries the old value
  - `main_test.go`: post mode downloads the state artifact once. `FetchRecording` gains `StateArtifactDownloads` and `MergedPRSearchQueries`: the mock search only sees a day, so the window tests check that day plus the merged count in the line
  - `main_test.go`: a state that fails to load gives the 7-day window
  - Snapshot: a post with `true` and previous stats 1 day old, showing `Last 1 day`
- Existing message snapshots keep `Last 7 days`: their runs load no previous stats. A changed message snapshot means a bug. The merge stats `.state.json` snapshots gain `mergedSince` and `mergeStatsLastComputedAt`. The normaliser adds both to its time keys
  - The stale snapshot's second post loads the first post's stats from moments before, so its own saved message reads `Last 1 day: 0 merged`, per § Caveats
- Update mergestats.spec.md, state.spec.md, messagecontent.spec.md, run.spec.md. messagecontent § Oddities: a window under 12 hours reads `Last 1 day`

## Consequences

### Positive

- A weekly review and merge speed number, in a message the team already reads
- Update runs cost nothing extra

### Negative

None

### Caveats

- Weekends are UTC days, so far from UTC they sit hours off local midnight, as for the old-PR check
- Holidays count as weekdays
- A week of 1 or 2 merges gives a median of 1 or 2 values
- A team east of UTC+9 posting at 09:00 local time posts on Sunday in UTC, so its line lands on Tuesday
- When later posts send nothing, Monday's line stays on a message read days later
- The medians cover merged PRs only: a PR still waiting is not in them
- A first review's timestamp is unverified for a single inline comment. `createdAt` stands in when `submittedAt` is null
- A grouped message loses one content block on every day, line or not
- Two posts hours apart with `show-merge-stats-in-message: true` give the second a window of hours, read as `Last 1 day`

### Neutral

- State carries one small object from a Monday post until the next post replaces it, and every cached merged PR gains one key

## Justification

### `Run` takes whether it is a merge stats day

- `prview` reads the wall clock for ages, so a test cannot fix the run's time
- A boolean keeps every existing test off the stats path on a real Monday, and lets a stats test run on any day
