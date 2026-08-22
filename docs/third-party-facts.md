# Third-party facts

What past work confirmed about things outside this repo: `go-github`, `slack-go`, the
GitHub GraphQL and search APIs, Slack methods and scopes, GitHub token permissions.

- External facts only. Behaviour of a package under `internal/` belongs in its
  `<package>.spec.md`
- Dead ends count as much as confirmations
- An entry is not a source. A plan cites what the entry names, never this file
- Correct a wrong entry in place
- Bullets, not prose. One claim each, sub-bullets for the detail under it
- Write entries in the [writing skill](../.agents/skills/writing/SKILL.md)'s style

## Entry format

```
## <the fact, stated as a claim>

- Source: <module cache path at the pinned version, doc URL, or the command that was run>
- Checked: <YYYY-MM-DD>, <what it is pinned to, if anything>
- <one claim per bullet>
  - <the numbers, the query, what it rules out>
```

The heading list is the index, so a heading has to carry the whole claim on its own.

## `Repository.pullRequests` cannot order by merge date

- Source: [IssueOrder input object](https://docs.github.com/en/graphql/reference/issues#input-object-issueorder)
- Checked: 2026-08-22
- `IssueOrderField` is `COMMENTS`, `CREATED_AT`, `UPDATED_AT`
- `UPDATED_AT DESC` cut client-side is not an approximation of it. Any post-merge comment,
  label or cross-reference bumps `updatedAt`, so the first page fills with old merges
- Measured with `gh api graphql`, one 100-node page against 7 days of merges:
  - `microsoft/vscode`: 348 merged, 33 on the page. 90% missed
  - `rust-lang/rust`: 218 merged, 58 on the page. 73% missed
  - `kubernetes/kubernetes`: the page reached back to 2014. ~25 days of updates, 12 years
    of merges

## GitHub search filters merged PRs by merge date server-side, but cannot sort by it

- Source: [searching issues and pull requests](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests), [sorting search results](https://docs.github.com/en/search-github/getting-started-with-searching-on-github/sorting-search-results)
- Checked: 2026-08-22
- `search(type: ISSUE)` with `repo:<owner>/<name> is:pr is:merged merged:>=<YYYY-MM-DD>`
  returns the in-window set in one page
- Sort fields: comments, created, interactions, reactions, relevance, updated. Merge order
  is client-side
- Day granularity returns up to one extra day of merges, cut client-side
- Unverified: the date-time form of the qualifier

## A GitHub search query string is capped at 256 characters and five operators

- Source: [REST search limits](https://docs.github.com/en/rest/search/search)
- Checked: 2026-08-22
- The 256 count the query text, excluding operators and qualifiers
- At most five `AND`, `OR` or `NOT`
- Bounds how many repositories one query string can name
- Unverified: OR semantics for repeated `repo:` qualifiers, the alternative to one aliased
  search per repository

## The 30 requests/minute GitHub search limit is a REST figure

- Source: [rate and node limits](https://docs.github.com/en/graphql/overview/rate-limits-and-node-limits-for-the-graphql-api)
- Checked: 2026-08-22
- A GraphQL search connection costs one request, plus one per potential node of a nested
  connection, divided by 100
- The budget is 5,000 points an hour

## The GraphQL cost formula overestimates: read `rateLimit.cost` instead

- Source: `gh api graphql`, two aliased `search(first: 100)` fields in one operation; [rate and node limits](https://docs.github.com/en/graphql/overview/rate-limits-and-node-limits-for-the-graphql-api)
- Checked: 2026-08-22
- The formula predicted 3 points for that query. GitHub charged 2, and 1 with the nested
  `labels(first: 100)` removed
- Cost is per operation, not per alias: adding repositories is nearly free, nested
  connections are what cost

## GitHub GraphQL `search` reports an unreadable repository as an empty result, never an error

- Source: `gh api graphql`, `search(type: ISSUE)` with a `repo:` qualifier
- Checked: 2026-08-22
- Indistinguishable: a nonexistent repository, a private one the token cannot read, and a
  genuinely empty window. All three are `{"issueCount": 0, "nodes": []}`, no `errors` key,
  `data` not null
- No `NOT_FOUND`, unlike `repository(owner:,name:)`
- A malformed qualifier (`merged:>=NOT-A-DATE`) is equally silent
- An empty query string does not error either. It searches all of GitHub: `issueCount`
  707,797,662
- Errors that do occur take the whole operation down:
  - `first: 200` on one alias returned `EXCESSIVE_PAGINATION`, `path: ["s0"]`, and no
    `data` key at all, so a valid second alias returned nothing either
  - Node-limit errors come back pathless
  - Neither carries `extensions.code`

## Several aliased `search` fields work in one GraphQL operation

- Source: `gh api graphql`, `s0`/`s1` over `microsoft/vscode`, `kubernetes/kubernetes`, `rust-lang/rust`; [GraphQL queries reference](https://docs.github.com/en/graphql/reference/queries#search)
- Checked: 2026-08-22
- `Query.search(query: String!, type: SearchType!, first/last/after/before)` returns
  `SearchResultItemConnection!` with `issueCount`, `nodes`, `pageInfo`
- `... on PullRequest` resolves inside `nodes`. `mergedAt`, `author` and
  `labels(first: 100)` all come back populated
- `first: 100` is the hard maximum, and the connection caps at 1,000 results

## GitHub's PR search index lags a merge by seconds

- Source: `gh api graphql`, polling `NixOS/nixpkgs` and `ClickHouse/ClickHouse` every ~7s
- Checked: 2026-08-22
- One merge caught in flight: absent 3 seconds after its `mergedAt`, present at 10
- No GitHub doc acknowledges or quantifies a lag
- Irrelevant to a scheduled job. Relevant to one triggered by a merge webhook

## Merging a PR bumps its `updatedAt`, but GitHub never documents what does

- Source: `gh api graphql`, 301 merged PR nodes across `golang/go`, `kubernetes/kubernetes`, `microsoft/vscode`, `facebook/react`, `rust-lang/rust`; [PullRequest object](https://docs.github.com/en/graphql/reference/objects#pullrequest)
- Checked: 2026-08-22
- Zero of the 301 had `updatedAt` earlier than `mergedAt`
- The schema says only "the date and time when the object was last updated". REST says
  nothing at all
- [Community #79024](https://github.com/orgs/community/discussions/79024) reports
  reactions not bumping `updated_at`, so the triggers are ad hoc
- An observation, not a contract. Ordering hints only, never correctness

## `PullRequest.mergedAt` is a nullable `DateTime`

- Source: [PullRequest object](https://docs.github.com/en/graphql/reference/objects#pullrequest)
- Checked: 2026-08-22
- `mergedAt` is `DateTime`, not `DateTime!`
- `state` is `OPEN`, `CLOSED` or `MERGED`; `merged` is a `Boolean!`
- `states: MERGED` implies both, so neither has to be selected

## `slack-go` v0.27.0 has `EditCanvas`, and one call replaces a whole canvas

- Source: `slack-go/slack@v0.27.0/canvas.go` in `go env GOMODCACHE`; [`canvases.edit` content operations](https://docs.slack.dev/reference/methods/canvases.edit/#content-operations)
- Checked: 2026-08-19, pinned to `slack-go` v0.27.0 in `go.mod`
- `EditCanvasParams{CanvasID, Changes: []CanvasChange{{Operation: "replace", DocumentContent: ...}}}`
- Omitting `section_id` on a `replace` makes it the whole canvas
- Needs only the `canvases:write` scope

## A Slack canvas has its own access control, separate from OAuth scopes

- Source: [`canvases.access.set`](https://docs.slack.dev/reference/methods/canvases.access.set)
- Checked: 2026-08-19
- `canvases:write` does not grant access to a given canvas
- Created outside a channel the bot is in: `canvases.edit` fails until it is shared
- Created as a channel tab: writable

## `search` returns private-repository PRs, but the permission granting it is undocumented

- Source: `gh api graphql`, `repo:<private repo> is:pr is:merged merged:>=` and `is:pr is:merged is:private`; [permissions for fine-grained PATs](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens); GitHub's docs data for `/search/issues` (`permissions: []`, `allowPermissionlessAccess: true`, `serverToServer: true`)
- Checked: 2026-08-22
- Confirmed with a classic token holding `repo`: the per-repository query returns a private
  repository's merged PRs, `repository.isPrivate` true, cost 1
- The endpoint needs no fine-grained permission to call
- What it returns is "private repositories you can access", and no doc defines which
  permission makes a repository accessible to the search index
- Untested: a fine-grained token holding only `pull-requests: read`, which also carries the
  auto-granted `metadata: read`
- The failure mode if that is not enough: silently empty results
