# canvascontent

Structures parsed PRs into the three sections of the PR tracker canvas, ready for `canvasbuilder`.

## Behaviour

- `GetContent(prs, mergedPRs, contentInputs, options)` splits the first list itself on `GetDraft()`: drafts go to the WIP section, everything else to the open section. The caller passes one unsplit fetch result. Merged PRs come as their own list, from their own fetch
- Open PRs keep their given order (oldest first, as `prparser.ParsePRs` left them)
- All three sections are bucketed by repository via `prparser.GroupPRsByRepositoriesInGivenOrder` when `GroupByRepository` is on, into `OpenPRsGroupedByRepository`, `WIPPRsGroupedByRepository` and `MergedPRsGroupedByRepository`; otherwise they stay the flat `OpenPRs`, `WIPPRs` and `MergedPRs` lists. Only one of the two shapes is ever filled
- Each section is bucketed in its own order, so the leading repository is the one holding the section's leading PR: the oldest open PR, the most recently touched WIP PR, the most recently merged PR. Bucketing never re-sorts PRs within a bucket
- WIP PRs are sorted most recent activity first via `prparser.SortPRsNewestFirst` on `UpdatedAt`. Unknown activity sorts last, keeping the given order among such PRs
- Drafts whose update time is older than `MaxDraftPRInactivity` (60 days) are left out. A draft with a zero update time is kept: unknown is not stale
- At most `MaxInactiveWIPPRs` (5) drafts without recent activity reach the WIP section, the 5 most recently touched of them. Inactive means the update time is at least `prparser.RecentActivityThreshold` (24 hours) before `GeneratedAt`, the boundary the WIP row styling uses. Recently touched drafts are never capped, nor are drafts with a zero update time. The cap runs after the staleness prune, on the whole WIP list rather than per repository
- Merged PRs are sorted newest merge first via `prparser.SortPRsNewestFirst` on `MergedAt`. They are neither pruned nor capped here: the fetch already did both
- `Content.MergedPRsUnavailable` comes from the options, and says the merged fetch failed rather than that nothing was merged
- `Content.GeneratedAt`, `OpenPRsCapped` and `WIPPRsCapped` come from the options. The cap flags report what the fetch capped, and are never derived from section length: the staleness prune and the inactive cap shrink the WIP list further
- Logs how many open PRs, WIP PRs and merged PRs the canvas ended up with, plus how many inactive drafts the cap left out, so a missing draft has an explanation

## Doesn't Do

- Doesn't read the clock: `GeneratedAt` is given by the caller, keeping `canvasbuilder`'s output deterministic under test
- Doesn't read `PRListHeading` or `NoPRsMessage`: canvas headings and fallback lines are fixed strings owned by `canvasbuilder`, so there is no `<pr_count>` substitution either
- Doesn't have a whole-canvas "nothing to show" case: each section falls back on its own
- Doesn't filter or re-sort the open section

## Oddities

- Draft staleness and inactivity are measured against `GeneratedAt`, not against the wall clock, so a zero-value `GeneratedAt` puts both cutoffs in year 0: every draft is kept, however long dead, and none counts as inactive. The caller always sets it
- Grouping an empty section yields an empty grouped slice rather than a group with no PRs
