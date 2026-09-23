# canvascontent

Structures PR views into the sections of the PR tracker canvas, ready for `canvasbuilder`.

## Behaviour

- `GetContent(prs, mergedPRs, contentInputs, options)` splits the first list itself on `PR.IsDraft()`/`PR.IsOpen()`: drafts go to the WIP section, everything else to the open sections. The caller passes one unsplit fetch result. Merged PRs come as their own list, from their own fetch
- The open PRs are bucketed by `PR.GetNextAction()` into `ReadyToMerge`, `WaitingForAuthor` and `WaitingForReview`, so a canvas reader picks their next action off a heading. Drafts and merged PRs never reach the rule
- Bucketing filters the list rather than sorting each bucket, so every bucket keeps the given order
- Each section is a `PRSection` on `Content`: the three open ones, `WIP` and `Merged`. A section is bucketed by repository into its `Groups` via `prview.GroupPRsByRepositoriesInGivenOrder` when `GroupByRepository` is on, and otherwise stays its flat `PRs` list. One `PRSection` constructor fills one shape, so both are never filled at once
- Each section is bucketed in its own order, so the leading repository is the one holding the section's leading PR: the oldest PR of that open bucket, the most recently touched WIP PR, the most recently merged PR. Bucketing never re-sorts PRs within a bucket, and nothing dedupes a repository across sections
- WIP PRs are sorted most recent activity first via `prview.SortPRsNewestFirst` on `PR.LastActivityAt()`. Unknown activity sorts last, keeping the given order among such PRs
- Drafts whose update time is older than `MaxDraftPRInactivity` (60 days) are left out, via `prview.PR.IsActiveAsOf(GeneratedAt, MaxDraftPRInactivity)`. A draft with a zero update time is kept: unknown is not stale
- At most `MaxInactiveWIPPRs` (5) drafts without recent activity reach the WIP section, the 5 most recently touched of them, via `prview.PR.IsActiveAsOf(GeneratedAt, prview.RecentActivityThreshold)`. Recently touched drafts are never capped, nor are drafts with a zero update time. The cap runs after the staleness prune, on the whole WIP list rather than per repository
- Merged PRs are sorted newest merge first via `prview.SortPRsNewestFirst` on `MergedAt`. They are neither pruned nor capped here: the fetch already did both
- `Content.MergedPRsUnavailable` comes from the options, and says the merged fetch failed rather than that nothing was merged
- `Content.GeneratedAt`, `OpenPRsCapped` and `WIPPRsCapped` come from the options. `OpenPRsCapped` is one flag over all three open sections: it reports what the fetch capped, and the fetch knows nothing about buckets. The cap flags are never derived from section length: the staleness prune and the inactive cap shrink the WIP list further
- Logs how many PRs each of the three open buckets, the WIP section and the merged section ended up with, plus how many inactive drafts the cap left out, so a missing draft has an explanation

## Doesn't Do

- Doesn't read the clock: `GeneratedAt` is given by the caller, keeping `canvasbuilder`'s output deterministic under test
- Doesn't read `NoPRsMessage`: canvas headings and fallback lines are fixed strings owned by `canvasbuilder`
- Doesn't have a whole-canvas "nothing to show" case: each section falls back on its own, and an empty open bucket is `canvasbuilder`'s to hide
- Doesn't re-sort within a bucket, and filters the open PRs only by next action: no PR the fetch returned is dropped

## Oddities

- Draft staleness and inactivity are measured against `GeneratedAt`, not against the wall clock, so a zero-value `GeneratedAt` puts both cutoffs in year 0: every draft is kept, however long dead, and none counts as inactive. The caller always sets it
- Grouping an empty section yields an empty grouped slice rather than a group with no PRs
