# messagecontent

Structures PR views into the sections a reminder message shows, and words its summary, no-open-PRs and merge stats texts, as a `Content` value ready for `messagebuilder`.

## Behaviour

- `GetContent(openPRs, trackedPRs, recentlyMergedPRs, mergeStats, mergeStatsWeekBefore, messagePostedAt, generatedAt, contentInputs)` fills four sections: `ReadyToMerge`, `WaitingForAuthor`, `WaitingForReview` and `Merged`
- `openPRs` are open right now and already draft-filtered by the caller; `trackedPRs` are the PRs the message has listed in an open section since it was posted, as re-fetched, in whatever state they are now
- Open sections: `openPRs` sorted oldest to newest by ready-for-review time via `prview.SortPRsOldestToNewest`, then bucketed by `prview.PR.GetNextAction()`, each bucket keeping that order
- Merged section, ordered newest merge first:
  - every `trackedPRs` entry that has since merged, however long ago
  - every `recentlyMergedPRs` entry merged after `messagePostedAt`
  - the newest `MaxUntrackedPRsMergedBeforePost` (3) other `recentlyMergedPRs` entries not tracked. The fetch is sorted before it is capped, so the 3 kept are the 3 newest whatever order the fetch arrived in
- `Merged` stays empty when `ShowMergedPRsInMessage` is false, whatever `trackedPRs` and `recentlyMergedPRs` hold. `HasPRs` then counts the open sections only
- A zero `messagePostedAt` means no message is posted yet, so every untracked merge counts against the cap
- A tracked PR is recognised by its `models.PullRequestRef`, from `prview.PR.GetPullRequestRef()` rather than through `state`
- Closed-but-not-merged `trackedPRs` reach no section
- Each section is a `prview.PRSection`. It is bucketed by repository when configured, through `prview.GroupPRsByRepositoriesInGivenOrder`, so each section's repositories are ordered by its own PR order
- Each flat section, or each repository group, turns its PRs into rows through `prview.RowsCollapsingPRsFromAuthors` with `CollapsePRsFromAuthors`, so collapsing runs after every sort, cap and bucket. When grouped, the 2 PR threshold counts within one repository
- `SummaryText`, the text Slack shows in notifications, counts the open PRs per next action, e.g. `"2 PRs to review, 1 waiting for author, 1 to merge 👀"`. It is never empty
  - Parts in the order review, author, merge. A zero count is left out
  - Only the first part names the noun, `PR` or `PRs` by that part's own count: `"1 PR waiting for author, 3 to merge 👀"`
  - Counts are PRs per bucket, before collapsing, so each collapsed PR counts
  - `"Nothing waiting for review 🎉"` when no open PR is listed
- `NoOpenPRsText` carries the configured `no-prs-message`, set only when no open PR is listed
- `GeneratedAt` carries the run timestamp through for the footer
- `MergeStatsText` is the merge stats line's wording, plain text without markup ([internal/messagebuilder](../messagebuilder/messagebuilder.spec.md) sets it in italics), built from the given merge stats and those of the week before, whatever `ShowMergedPRsInMessage` says. Empty when the stats are nil
  - `Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge`
  - The window reads in whole days, `ComputedAt` minus `MergedSince` rounded to the nearest day, at least 1: `Last 1 day`, `Last 3 days`, `Last 7 days`
  - A nil median drops its part: `Last 7 days: 2 merged · median 1.0 days to merge`, and with both nil, `Last 7 days: 0 merged`
  - A duration reads in whole minutes under an hour, in hours with one decimal under 24 hours, else in days with one decimal: `24 minutes`, `1 minute`, `4.0 hours`, `1.0 days`
  - With the week before given, the line ends with each shown median's change from [internal/mergestats](../mergestats/mergestats.spec.md)'s `PercentChanges`, in the medians' order: `… 1.3 days to merge (+15 %, -12 %)`, `… 1.0 days to merge (-20 %)`. A rise or no change reads `+`: `+15 %`, `+0 %`
  - The parenthetical shows only when every shown median has a change. A median without a change drops the whole parenthetical. A line without medians, such as `Last 1 day: 0 merged`, never gets a parenthetical
- `Content.HasPRs()` reports whether any of the four sections has a row, collapsed rows included. `MergeStatsText` never counts

## Doesn't Do

- Doesn't filter `openPRs` at all: everything in that list is open and reaches a section
- Doesn't read `trackedPRs` for anything but their merged state

## Oddities

- A section fills `Rows` or `Groups`, never both, so a reader has to know which by `Content.GroupedByRepository`
- A merge stats window under 12 hours rounds to 0 days and reads `Last 1 day`, as does any window from 12 to 36 hours
- The merge stats durations' rounding can read one unit past a switch: 59.6 minutes reads `60 minutes`, and 23.96 hours `24.0 hours`
- A merged PR with no merge timestamp sorts last among the merged rows, since an unknown time reads as unknown rather than old. It counts as merged before the post
