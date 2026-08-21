# messagecontent

Structures parsed PRs and content settings into a `Content` value ready for `messagebuilder`.

## Behaviour

- `GetContent(openPRs, contentInputs)` branches three ways: no PRs, grouped by repository, or a flat list
- No-PRs case: only the "no PRs" message is set
- Otherwise a summary line reports the open PR count (singular phrasing for exactly 1)
- Flat case: the configured heading has its `<pr_count>` placeholder replaced with the PR count
- Grouped case: PRs are bucketed by repository via `prparser.GroupPRsByRepositories` (alphabetical by repository path); this package adds each bucket's heading prefix, link label and link to the repository's GitHub pulls page (`models.Repository.GetPullsURL`)
- `Content.HasPRs()` reports whether there's anything to show, in either mode
- Every branch carries the PR tracker canvas URL through to `Content.CanvasURL`, empty when the canvas feature is off

## Doesn't Do

- Doesn't sort or filter PRs within a group — order is whatever was already parsed
- Only the `<pr_count>` placeholder is substituted in the heading; any other placeholder-like syntax passes through unchanged

## Oddities

- Grouped mode ignores the configured PR-list heading entirely — it only applies in the non-grouped case (this is also why the heading input is required only when grouping is off)
