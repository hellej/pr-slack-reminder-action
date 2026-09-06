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
## <the fact, stated as a claim> [YYYY-MM-DD]

- Source: <module cache path at the pinned version, doc URL, or the command that was run>
- Checked: <YYYY-MM-DD>, <what it is pinned to, if anything>
- <one claim per bullet>
  - <the numbers, the query, what it rules out>
```

The heading list is the index, so a heading has to carry the whole claim on its own, and the
date it was last checked. The bracketed date and the `Checked:` line always match. Re-checking an
entry moves both.

## `Repository.pullRequests` cannot order by merge date [2026-08-22]

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

## GitHub search filters merged PRs by merge date server-side, but cannot sort by it [2026-08-22]

- Source: [searching issues and pull requests](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests), [sorting search results](https://docs.github.com/en/search-github/getting-started-with-searching-on-github/sorting-search-results)
- Checked: 2026-08-22
- `search(type: ISSUE)` with `repo:<owner>/<name> is:pr is:merged merged:>=<YYYY-MM-DD>`
  returns the in-window set in one page
- Sort fields: comments, created, interactions, reactions, relevance, updated. Merge order
  is client-side
- Day granularity returns up to one extra day of merges, cut client-side
- Unverified: the date-time form of the qualifier

## A GitHub search query string is capped at 256 characters and five operators [2026-08-22]

- Source: [REST search limits](https://docs.github.com/en/rest/search/search)
- Checked: 2026-08-22
- The 256 count the query text, excluding operators and qualifiers
- At most five `AND`, `OR` or `NOT`
- Bounds how many repositories one query string can name
- Unverified: OR semantics for repeated `repo:` qualifiers, the alternative to one aliased
  search per repository

## In GraphQL, `pullRequests` filters only by label; `search` also filters authors and drafts [2026-08-22]

- Source: `gh api graphql` against `microsoft/vscode`, comparing `issueCount` and `totalCount` per qualifier; `__type(name:"Repository")` introspection for the argument list
- Checked: 2026-08-22

`repository.pullRequests` takes `states`, `labels`, `headRefName`, `baseRefName`, `orderBy`
and the four pagination arguments, introspected, nothing else. Of those only `labels`
filters what this project filters on:

- `labels: ["a","b"]` is OR: 5 + 6 = 11, matching an any-of allow-list
- No author argument, no negation of either, no draft argument

`search(type: ISSUE)` covers the rest, through qualifiers:

- `label:a,b` is OR (11); `label:a label:b` is AND (0)
- Repeated `author:` qualifiers are OR: 14 + 11 = 25
- `-author:` and `-label:` exclude: 2491 down to 2477 for one bot author
- `draft:true` / `draft:false` split the set, 678 / 1813

## No GraphQL query filters PRs by a case-sensitive title substring [2026-08-22]

- Source: `gh api graphql` against `microsoft/vscode`, comparing `issueCount` per qualifier
- Checked: 2026-08-22

`repository.pullRequests` has no title argument at all, and `search` matches whole words,
case-insensitively. Of 2491 open PRs: `epo in:title` matched 0 while `repo in:title`
matched 15, and `Fix in:title` and `fix in:title` both matched 1005. A case-sensitive
substring filter has to run client-side.

Negating a free-text term needs `NOT`, not `-`: `NOT Fix in:title` matched 1486, the exact
complement of 1005, while `-Fix in:title` matched 1005, the same as no negation at all. The
`-` prefix does work on qualifiers such as `-label:` and `-author:`.

## The 30 requests/minute GitHub search limit is a REST figure [2026-08-22]

- Source: [rate and node limits](https://docs.github.com/en/graphql/overview/rate-limits-and-node-limits-for-the-graphql-api)
- Checked: 2026-08-22
- A GraphQL search connection costs one request, plus one per potential node of a nested
  connection, divided by 100
- The budget is 5,000 points an hour

## The GraphQL cost formula overestimates: read `rateLimit.cost` instead [2026-08-22]

- Source: `gh api graphql` against `microsoft/vscode`, `rust-lang/rust`, `kubernetes/kubernetes`, `facebook/react`; [rate and node limits](https://docs.github.com/en/graphql/overview/rate-limits-and-node-limits-for-the-graphql-api)
- Checked: 2026-08-22
- The formula predicted 3 points for a two-repository search. GitHub charged 2
- A nested connection is what costs. Aliases carrying one are charged per alias; aliases
  without one are nearly free
- Measured costs of this action's query shapes, against 5,000 points an hour:

| Query | Cost |
|---|---|
| Open PR listing, `pullRequests(first: 100)` + `labels(first: 100)` | 1 per repository |
| Merged PR search, `search(first: 100)` + `labels(first: 100)` | 1 per repository |
| Merged PR search, no `labels` | 1 total, any repository count |
| Enrichment batch, 25 PRs, `reviews(first: 100)` + `comments(first: 100)` | 1 |

- The enrichment batch was measured with a `commits(last: 1)` selection it no longer carries

## GitHub GraphQL `search` reports an unreadable repository as an empty result, never an error [2026-08-22]

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

## Several aliased `search` fields work in one GraphQL operation [2026-08-22]

- Source: `gh api graphql`, `s0`/`s1` over `microsoft/vscode`, `kubernetes/kubernetes`, `rust-lang/rust`; [GraphQL queries reference](https://docs.github.com/en/graphql/reference/queries#search)
- Checked: 2026-08-22
- `Query.search(query: String!, type: SearchType!, first/last/after/before)` returns
  `SearchResultItemConnection!` with `issueCount`, `nodes`, `pageInfo`
- `... on PullRequest` resolves inside `nodes`. `mergedAt`, `author` and
  `labels(first: 100)` all come back populated
- `first: 100` is the hard maximum, and the connection caps at 1,000 results

## GitHub's PR search index lags a merge by seconds [2026-08-22]

- Source: `gh api graphql`, polling `NixOS/nixpkgs` and `ClickHouse/ClickHouse` every ~7s
- Checked: 2026-08-22
- One merge caught in flight: absent 3 seconds after its `mergedAt`, present at 10
- No GitHub doc acknowledges or quantifies a lag
- Irrelevant to a scheduled job. Relevant to one triggered by a merge webhook

## Merging a PR bumps its `updatedAt`, but GitHub never documents what does [2026-08-22]

- Source: `gh api graphql`, 301 merged PR nodes across `golang/go`, `kubernetes/kubernetes`, `microsoft/vscode`, `facebook/react`, `rust-lang/rust`; [PullRequest object](https://docs.github.com/en/graphql/reference/objects#pullrequest)
- Checked: 2026-08-22
- Zero of the 301 had `updatedAt` earlier than `mergedAt`
- The schema says only "the date and time when the object was last updated". REST says
  nothing at all
- [Community #79024](https://github.com/orgs/community/discussions/79024) reports
  reactions not bumping `updated_at`, so the triggers are ad hoc
- An observation, not a contract. Ordering hints only, never correctness

## `PullRequest.mergedAt` is a nullable `DateTime` [2026-08-22]

- Source: [PullRequest object](https://docs.github.com/en/graphql/reference/objects#pullrequest)
- Checked: 2026-08-22
- `mergedAt` is `DateTime`, not `DateTime!`
- `state` is `OPEN`, `CLOSED` or `MERGED`; `merged` is a `Boolean!`
- `states: MERGED` implies both, so neither has to be selected

## `slack-go` v0.27.0 has `EditCanvas`, and one call replaces a whole canvas [2026-08-19]

- Source: `slack-go/slack@v0.27.0/canvas.go` in `go env GOMODCACHE`; [`canvases.edit` content operations](https://docs.slack.dev/reference/methods/canvases.edit/#content-operations)
- Checked: 2026-08-19, pinned to `slack-go` v0.27.0 in `go.mod`
- `EditCanvasParams{CanvasID, Changes: []CanvasChange{{Operation: "replace", DocumentContent: ...}}}`
- Omitting `section_id` on a `replace` makes it the whole canvas
- Needs only the `canvases:write` scope

## A Slack canvas has its own access control, separate from OAuth scopes [2026-08-19]

- Source: [`canvases.access.set`](https://docs.slack.dev/reference/methods/canvases.access.set)
- Checked: 2026-08-19
- `canvases:write` does not grant access to a given canvas
- Created outside a channel the bot is in: `canvases.edit` fails until it is shared
- Created as a channel tab: writable

## `search` returns private-repository PRs, but the permission granting it is undocumented [2026-08-22]

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

## Whether GraphQL `PullRequest.commits` needs `contents: read` is undocumented [2026-08-27]

- Source: [community discussion 62476](https://github.com/orgs/community/discussions/62476); [permissions for fine-grained PATs](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens)
- Checked: 2026-08-27
- No GitHub doc lists a permission for the `commits` connection on a `PullRequest`
- The discussion's opening post says commit endpoints need `contents: read` while PR
  endpoints show commits without it, and asks for that to be made consistent. No staff answer
- One commenter (hkdobrev) reports reading commits off the PR timeline with `pull-requests:
  read` alone
- Untested here: this action never ran a token without `contents: read` against the
  `commits(last: 1)` selection it used to carry, so the requirement was never observed
- The selection was dropped anyway: `updatedAt` serves the canvas, and no permission
  question rides on it

## Rapid `canvases.edit` replaces duplicate headings and rows in an open canvas, until a reload [2026-09-05]

- Source: measured against canvas `F0BPS4FKCEL` on 2026-09-05 with a scratch probe sending
  the same form POST `ReplaceCanvasContent` sends
- Checked: 2026-09-05
- Reproduced: 15 full-canvas replaces 1.5s apart, each one reshaping the document (cycling
  the grouped, flat and no-open-PRs golden files), with the canvas open in the desktop app
  - `## Open` and `## WIP` each rendered twice, one PR row appeared under two sections,
    `_No open PRs_` rendered next to real rows, and the `_Updated_` footer landed
    mid-document with content below it
  - A "New edits" badge showed while the view was wrong
- It is a client-side merge artifact, not stored corruption. Cmd+R re-rendered the last
  payload exactly, and a viewer's own typing survives as a real edit
- Structural churn is the trigger, not a cursor. 12 replaces 2s apart that only moved the
  footer timestamp rendered clean; the same 12 mangled the view when the first of them
  reshaped the document. A parked cursor and typing straight through a write both stayed clean
- No cap on write rate was hit: every replace in every run returned `ok: true`

## Slack accepts simultaneous `canvases.edit` replaces, and documents no way to read a canvas back [2026-09-05]

- Source: [`canvases.edit`](https://docs.slack.dev/reference/methods/canvases.edit/);
  [canvases surface guide](https://docs.slack.dev/surfaces/canvases/); the 2026-09-05 probe
- Checked: 2026-09-05
- Two replaces fired from two threads at once both returned `ok: true`, as did two 1.5s
  apart. `canvas_editing_locked` never appeared, so it can't be relied on to serialize writes
- The docs describe `canvas_editing_locked` as "Another edit to this canvas is currently in
  progress", which rejects a whole call. No documented path applies part of a `replace`
- Nothing documents a viewer's open editor, cursor or selection shielding a section
- No method returns a canvas's markdown, so a write cannot be verified by reading it back
- `document_content.markdown` is capped at 1 MiB per object
- `canvases.create` fails on a free workspace with `free_teams_cannot_create_standalone_canvases`,
  and `conversations.canvases.create` with `free_team_canvas_tab_already_exists`

## Slack's `replace` with a `section_id` has been reported to act like `insert_after` [2026-09-05]

- Source: [slackapi/slack-mcp-plugin issue 30](https://github.com/slackapi/slack-mcp-plugin/issues/30)
- Checked: 2026-09-05, one community report, not confirmed by Slack
- The targeted section stays and the new content lands as a sibling after it, reproduced on
  both a paragraph and a header section. The reporter blames the MCP server's mapping, not
  the Slack method
- A full-canvas `replace` keeps a sticky H1 carrying the canvas title, with an ID stable
  across writes. A body H1 matching the title therefore reads back twice
- Neither applies to this action: `ReplaceCanvasContent` sends no `section_id`, and
  `canvasbuilder` renders no H1

## An Actions artifact cannot be updated in place: each upload is a new artifact, owned by its run [2026-09-05]

- Source: [upload-artifact README](https://github.com/actions/upload-artifact#readme);
  [REST: Actions artifacts](https://docs.github.com/en/rest/actions/artifacts)
- Checked: 2026-09-05
- "Artifacts created by upload-artifact@v4 are immutable". Overwriting one "will give the
  Artifact a new ID, the previous one will no longer exist"
- `overwrite: true` deletes a matching name within the same workflow run only. It cannot touch
  an artifact belonging to an earlier run
- The REST API has list, get, download and delete, and no upload, update or replace endpoint.
  Uploading is the runner's own protocol, so only an in-run step can create an artifact
- Deleting an artifact needs `actions: write`, above the `actions: read` an update run uses
- So a state artifact re-uploaded every run accumulates one artifact per run until retention
  expires them

## A workflow-level `concurrency` key can read the `inputs` context, and cancels pending runs [2026-09-05]

- Source: [context availability](https://docs.github.com/en/actions/reference/workflows-and-actions/contexts#context-availability);
  [workflow syntax: concurrency](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#concurrency)
- Checked: 2026-09-05
- Available to `concurrency`: `github`, `inputs`, `vars`. So a group can be computed from the
  event name and a `workflow_dispatch` input, and split one workflow's runs into several groups
- `github.token` and `github.job` are the properties restricted to step execution. `run_id` is
  not among them
- For a non-`workflow_dispatch` event `inputs` is null, so `inputs.x == 'y'` is simply false
- With `cancel-in-progress` unset or false, a newly queued run still cancels any *pending* run in
  the group. Only the run in progress and the newest queued one survive a burst
- A shared group therefore lets frequent triggers cancel a pending scheduled run
