# mergestats

Computes the merge stats a post's message shows: the PRs merged in a window and how long they took. See [AGENTS.md](../../AGENTS.md) for its place in the run.

## Behaviour

- `FromMergedPRs(prs, mergedSince, computedAt)` returns a `MergeStats` over the given merged PRs. Pure: it reads no clock, and the window is the caller's
- `MergedCount` is the number of PRs given, bot-authored ones included
- `MedianTimeToMerge` is the median time from `githubclient.PR.ReadyForReviewAt()` to the merge. Nil when no PR is given
- `MedianTimeToFirstReview` is the median time from the same start to `githubclient.PR.FirstReviewAt`, over the PRs that have one. Nil when none has
  - A first review before the ready-for-review time counts as 0
- Both are weekday time, through `prview.WeekdayTimeBetween`: Saturdays and Sundays left out as whole UTC days. See [internal/prview](../prview/prview.spec.md)
- An even count takes the mean of the two middle values
- `MergedSince` and `ComputedAt` carry the given window start and `computedAt`, so the window can be rendered again later
- JSON keys: `mergedCount`, `medianTimeToFirstReview`, `medianTimeToMerge`, `mergedSince`, `computedAt`. A nil median saves no key. A median saves as integer nanoseconds

## Doesn't Do

- Doesn't render the stats: the line's text is [internal/messagebuilder](../messagebuilder/messagebuilder.spec.md)'s
- Doesn't check that a PR is merged or within any window, nor that its reviewer info is complete: the caller passes what it fetched

## Oddities

- A PR with a zero ready-for-review time, or no merge time, counts in `MergedCount` but in neither median
