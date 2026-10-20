# mergestats

Computes the merge stats a post's message shows: the PRs merged in a window and how long they took, and their change against the week before. See [AGENTS.md](../../AGENTS.md) for its place in the run.

## Behaviour

- `FromMergedPRs(prs, mergedSince, computedAt)` returns a `MergeStats` over the given merged PRs. Pure: it reads no clock, and the window is the caller's
- `MergedCount` is the number of PRs given, bot-authored ones included
- The PRs counted in the medians are those with `HasCompleteReviewerInfo`, a ready-for-review time and a merge time
- `MedianTimeToMerge` is the median time from `githubclient.PR.ReadyForReviewAt()` to the merge, over the PRs counted in the medians. Nil when no PR is counted in the medians
- `MedianTimeToFirstReview` is the median time from the same start to `githubclient.PR.FirstReviewAt`. It covers the PRs counted in the medians whose `FirstReviewAt` is set. Nil when no PR counted in the medians has `FirstReviewAt` set
  - A first review before the ready-for-review time counts as 0
- Both are weekday time, through `prview.WeekdayTimeBetween`: Saturdays and Sundays left out as whole UTC days. See [internal/prview](../prview/prview.spec.md)
- An even count takes the mean of the two middle values
- `MergedSince` and `ComputedAt` carry the given window start and `computedAt`, so the window can be rendered again later
- `PercentChanges(shown, weekBefore)` returns the first review and the merge median's change from `weekBefore` to `shown`, as `100 × (shown − weekBefore) / weekBefore` rounded half away from zero to a whole percent. Pure
  - Nil for a median either week lacks, or whose week-before value is 0
  - Computed from the stored durations, so two weeks reading the same rounded text can still differ
  - Compares the medians whatever each window's length
- JSON keys: `mergedCount`, `medianTimeToFirstReview`, `medianTimeToMerge`, `mergedSince`, `computedAt`. A nil median saves no key. A median saves as integer nanoseconds

## Doesn't Do

- Doesn't render the stats or their changes: the line's text is [internal/messagecontent](../messagecontent/messagecontent.spec.md)'s, and its block [internal/messagebuilder](../messagebuilder/messagebuilder.spec.md)'s
- Doesn't pick the week before: the caller passes it
- Doesn't check that a PR is merged or within any window: the caller passes what it fetched

## Oddities

- A PR without complete reviewer info, a zero ready-for-review time or no merge time counts in `MergedCount` but in neither median, so the medians can cover fewer PRs than `MergedCount`. Without complete reviewer info, the PR may lack its reviews and its ready-for-review event
