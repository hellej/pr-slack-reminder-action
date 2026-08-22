# Third-party facts

What past work confirmed about things outside this repo: `go-github`, `slack-go`, the
GitHub GraphQL and search APIs, Slack methods and scopes, GitHub token permissions.

- External facts only. Behaviour of a package under `internal/` belongs in its
  `<package>.spec.md`
- Dead ends count as much as confirmations
- An entry is not a source. A plan cites what the entry names, never this file
- Correct a wrong entry in place
- Write entries in the [writing skill](../.agents/skills/writing/SKILL.md)'s style

## Entry format

```
## <the fact, stated as a claim>

- Source: <module cache path at the pinned version, doc URL, or the command that was run>
- Checked: <YYYY-MM-DD>, <what it is pinned to, if anything>

<optional: the query, the numbers, what was ruled out>
```

The heading list is the index, so a heading has to carry the whole claim on its own.

## `Repository.pullRequests` cannot order by merge date

- Source: [IssueOrder input object](https://docs.github.com/en/graphql/reference/issues#input-object-issueorder). `IssueOrderField` is `COMMENTS`, `CREATED_AT`, `UPDATED_AT`
- Checked: 2026-08-22

`UPDATED_AT DESC` cut client-side does not approximate it. Any post-merge comment, label
or cross-reference bumps `updatedAt`, so the first page fills with old merges. Measured
with `gh api graphql`, one 100-node page against PRs merged in the last 7 days:

- `microsoft/vscode`: 348 merged, 33 on the page. 90% missed
- `rust-lang/rust`: 218 merged, 58 on the page. 73% missed
- `kubernetes/kubernetes`: the page held PRs merged in 2014, pulled up by later comments. ~25 days of update activity, 12 years of merges

## GitHub search filters merged PRs by merge date server-side, but cannot sort by it

- Source: [searching issues and pull requests](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests), [sorting search results](https://docs.github.com/en/search-github/getting-started-with-searching-on-github/sorting-search-results)
- Checked: 2026-08-22

`search(type: ISSUE)` with `repo:<owner>/<name> is:pr is:merged merged:>=<YYYY-MM-DD>`
returns the in-window set in one page. Sort fields are comments, created, interactions,
reactions, relevance and updated, so merge order is client-side.

Day granularity returns up to one extra day of merges, cut client-side. Unverified: the
date-time form of the qualifier.

## The 30 requests/minute GitHub search limit is a REST figure

- Source: [rate and node limits](https://docs.github.com/en/graphql/overview/rate-limits-and-node-limits-for-the-graphql-api)
- Checked: 2026-08-22

A GraphQL search connection costs one request, plus one per potential node of a nested
connection divided by 100, against 5,000 points an hour.

## `slack-go` v0.27.0 has `EditCanvas`, and one call replaces a whole canvas

- Source: `slack-go/slack@v0.27.0/canvas.go` in `go env GOMODCACHE`; [`canvases.edit` content operations](https://docs.slack.dev/reference/methods/canvases.edit/#content-operations)
- Checked: 2026-08-19, pinned to `slack-go` v0.27.0 in `go.mod`

`EditCanvasParams{CanvasID, Changes: []CanvasChange{{Operation: "replace", DocumentContent: ...}}}`.
Omitting `section_id` on a `replace` makes it the whole canvas. The method needs only the
`canvases:write` scope.

## A Slack canvas has its own access control, separate from OAuth scopes

- Source: [`canvases.access.set`](https://docs.slack.dev/reference/methods/canvases.access.set)
- Checked: 2026-08-19

`canvases:write` does not grant access to a given canvas. One created outside a channel
the bot is in fails `canvases.edit` until it is shared. One created as a channel tab is
writable.
