# canvascontent

Structures parsed PRs into the three sections of the PR tracker canvas, ready for `canvasbuilder`.

## Behaviour

- `GetContent(prs, mergedPRs, contentInputs, options)` splits the first list itself on `GetDraft()`: drafts go to the WIP section, everything else to the open section. The caller passes one unsplit fetch result. Merged PRs come as their own list, from their own fetch
- Open PRs keep their given order (oldest first, as `prparser.ParsePRs` left them)
- Open PRs are bucketed by repository via `prparser.GroupPRsByRepositories` when `GroupByRepository` is on, into `OpenPRsGroupedByRepository`; otherwise they stay a flat `OpenPRs` list. Only one of the two is ever filled
- WIP PRs are always a flat list, whatever `GroupByRepository` says, sorted most recent activity first via `prparser.SortPRsNewestFirst` on `LastActivityAt`
- Drafts whose last activity is older than `MaxDraftPRInactivity` (60 days) are left out. A draft with unknown activity is kept: unknown is not stale
- Merged PRs are always a flat list, whatever `GroupByRepository` says, sorted newest merge first via `prparser.SortPRsNewestFirst` on `MergedAt`. They are neither pruned nor capped here: the fetch already did both
- `Content.MergedPRsUnavailable` comes from the options, and says the merged fetch failed rather than that nothing was merged
- `Content.GeneratedAt`, `OpenPRsCapped` and `WIPPRsCapped` come from the options. The cap flags report what the fetch capped, and are never derived from section length: the staleness prune shrinks the WIP list further
- Logs how many open PRs, WIP PRs, merged PRs and inactive drafts the canvas ended up with, so a missing draft has an explanation

## Doesn't Do

- Doesn't read the clock: `GeneratedAt` is given by the caller, keeping `canvasbuilder`'s output deterministic under test
- Doesn't read `PRListHeading` or `NoPRsMessage`: canvas headings and fallback lines are fixed strings owned by `canvasbuilder`, so there is no `<pr_count>` substitution either
- Doesn't have a whole-canvas "nothing to show" case: each section falls back on its own
- Doesn't filter or re-sort the open section

## Oddities

- Draft staleness is measured against `GeneratedAt`, not against the wall clock, so a zero-value `GeneratedAt` puts the cutoff in year 0 and keeps every draft, however long dead. The caller always sets it
- Grouping with no open PRs yields an empty `OpenPRsGroupedByRepository` rather than a group with no PRs
