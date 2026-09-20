# messagecontent

Structures PR views into the sections a reminder message shows, as a `Content` value ready for `messagebuilder`.

## Behaviour

- `GetContent(openPRs, trackedPRs, recentlyMergedPRs, generatedAt, contentInputs)` fills four sections: `ReadyToMerge`, `WaitingForAuthor`, `WaitingForReview` and `Merged`
- `openPRs` are open right now and already draft-filtered by the caller; `trackedPRs` are the PRs the message was posted with, as re-fetched, in whatever state they are now
- Open sections: `openPRs` sorted oldest to newest, then bucketed by `prview.PR.GetNextAction()`, each bucket keeping that order
- Merged section: every `trackedPRs` entry that has since merged, however long ago, plus the newest `MaxUntrackedMergedPRs` (3) entries of `recentlyMergedPRs` that aren't already among them, the whole result ordered newest merge first. The fetch's half is sorted before it is capped, so the 3 kept are the 3 newest whatever order the fetch arrived in
- A tracked PR is recognised by its `models.PullRequestRef`, built here rather than through `state`
- Closed-but-not-merged `trackedPRs` reach no section
- Each section is bucketed by repository when configured, through `prview.GroupPRsByRepositoriesInGivenOrder`, so each section's repositories are ordered by its own PR order. This package adds each bucket's link label and its link to the repository's GitHub pulls page (`models.Repository.GetPullsURL`)
- `SummaryText`, Slack's plain-text fallback, reports the open PR count (singular phrasing for exactly 1), or is `"Nothing waiting for review 🎉"` when no open PR is listed. It is never empty
- `NoOpenPRsText` carries the configured `no-prs-message`, set only when no open PR is listed
- `GeneratedAt` carries the run timestamp through for the footer
- `Content.HasPRs()` reports whether any of the four sections has a PR; `PRSection.HasPRs()` reports it for one

## Doesn't Do

- Doesn't filter `openPRs` at all: everything in that list is open and reaches a section
- Doesn't read `trackedPRs` for anything but their merged state
- Doesn't cap the tracked merges, only the untracked ones

## Oddities

- A `PRSection` fills `PRs` or `Groups`, never both, so a reader has to know which by `Content.GroupedByRepository`
- A merged PR with no merge timestamp sorts last among the merged rows, since an unknown time reads as unknown rather than old
