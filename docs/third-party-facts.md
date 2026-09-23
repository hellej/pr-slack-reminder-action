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
- <one claim per bullet>
  - <the numbers, the query, what it rules out>
```

The heading list is the index, so a heading has to carry the whole claim on its own, and the
date it was last checked. Re-checking an entry moves that date.

## `Repository.pullRequests` cannot order by merge date [2026-08-22]

- Source: [IssueOrder input object](https://docs.github.com/en/graphql/reference/issues#input-object-issueorder)
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
- `search(type: ISSUE)` with `repo:<owner>/<name> is:pr is:merged merged:>=<YYYY-MM-DD>`
  returns the in-window set in one page
- Sort fields: comments, created, interactions, reactions, relevance, updated. Merge order
  is client-side
- Day granularity returns up to one extra day of merges, cut client-side
- Unverified: the date-time form of the qualifier

## A GitHub search query string is capped at 256 characters and five operators [2026-08-22]

- Source: [REST search limits](https://docs.github.com/en/rest/search/search)
- The 256 count the query text, excluding operators and qualifiers
- At most five `AND`, `OR` or `NOT`
- Bounds how many repositories one query string can name
- Unverified: OR semantics for repeated `repo:` qualifiers, the alternative to one aliased
  search per repository

## In GraphQL, `pullRequests` filters only by label; `search` also filters authors and drafts [2026-08-22]

- Source: `gh api graphql` against `microsoft/vscode`, comparing `issueCount` and `totalCount` per qualifier; `__type(name:"Repository")` introspection for the argument list

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

`repository.pullRequests` has no title argument at all, and `search` matches whole words,
case-insensitively. Of 2491 open PRs: `epo in:title` matched 0 while `repo in:title`
matched 15, and `Fix in:title` and `fix in:title` both matched 1005. A case-sensitive
substring filter has to run client-side.

Negating a free-text term needs `NOT`, not `-`: `NOT Fix in:title` matched 1486, the exact
complement of 1005, while `-Fix in:title` matched 1005, the same as no negation at all. The
`-` prefix does work on qualifiers such as `-label:` and `-author:`.

## The 30 requests/minute GitHub search limit is a REST figure [2026-08-22]

- Source: [rate and node limits](https://docs.github.com/en/graphql/overview/rate-limits-and-node-limits-for-the-graphql-api)
- A GraphQL search connection costs one request, plus one per potential node of a nested
  connection, divided by 100
- The budget is 5,000 points an hour

## The GraphQL cost formula overestimates: read `rateLimit.cost` instead [2026-08-22]

- Source: `gh api graphql` against `microsoft/vscode`, `rust-lang/rust`, `kubernetes/kubernetes`, `facebook/react`; [rate and node limits](https://docs.github.com/en/graphql/overview/rate-limits-and-node-limits-for-the-graphql-api)
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
| Enrichment batch, 25 PRs, the same plus `reviewThreads(first: 100)` | 1 |

- The enrichment batch was measured with a `commits(last: 1)` selection it no longer carries
- A fourth nested connection per alias left the cost at 1, taking the batch from 7,500 to
  10,000 requested nodes. Individual calls cap at 500,000 nodes, so that is 2% of it

## GitHub GraphQL `search` reports an unreadable repository as an empty result, never an error [2026-08-22]

- Source: `gh api graphql`, `search(type: ISSUE)` with a `repo:` qualifier
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
- `Query.search(query: String!, type: SearchType!, first/last/after/before)` returns
  `SearchResultItemConnection!` with `issueCount`, `nodes`, `pageInfo`
- `... on PullRequest` resolves inside `nodes`. `mergedAt`, `author` and
  `labels(first: 100)` all come back populated
- `first: 100` is the hard maximum, and the connection caps at 1,000 results

## GitHub's PR search index lags a merge by seconds [2026-08-22]

- Source: `gh api graphql`, polling `NixOS/nixpkgs` and `ClickHouse/ClickHouse` every ~7s
- One merge caught in flight: absent 3 seconds after its `mergedAt`, present at 10
- No GitHub doc acknowledges or quantifies a lag
- Irrelevant to a scheduled job. Relevant to one triggered by a merge webhook

## Merging a PR bumps its `updatedAt`, but GitHub never documents what does [2026-08-22]

- Source: `gh api graphql`, 301 merged PR nodes across `golang/go`, `kubernetes/kubernetes`, `microsoft/vscode`, `facebook/react`, `rust-lang/rust`; [PullRequest object](https://docs.github.com/en/graphql/reference/objects#pullrequest)
- Zero of the 301 had `updatedAt` earlier than `mergedAt`
- The schema says only "the date and time when the object was last updated". REST says
  nothing at all
- [Community #79024](https://github.com/orgs/community/discussions/79024) reports
  reactions not bumping `updated_at`, so the triggers are ad hoc
- An observation, not a contract. Ordering hints only, never correctness

## `PullRequest.mergedAt` is a nullable `DateTime` [2026-08-22]

- Source: [PullRequest object](https://docs.github.com/en/graphql/reference/objects#pullrequest)
- `mergedAt` is `DateTime`, not `DateTime!`
- `state` is `OPEN`, `CLOSED` or `MERGED`; `merged` is a `Boolean!`
- `states: MERGED` implies both, so neither has to be selected

## `slack-go` v0.27.0 has `EditCanvas`, and one call replaces a whole canvas [2026-08-19]

- Source: `slack-go/slack@v0.27.0/canvas.go` in `go env GOMODCACHE`; [`canvases.edit` content operations](https://docs.slack.dev/reference/methods/canvases.edit/#content-operations)
- Pinned to `slack-go` v0.27.0 in `go.mod`
- `EditCanvasParams{CanvasID, Changes: []CanvasChange{{Operation: "replace", DocumentContent: ...}}}`
- Omitting `section_id` on a `replace` makes it the whole canvas
- Needs only the `canvases:write` scope

## A Slack canvas has its own access control, separate from OAuth scopes [2026-08-19]

- Source: [`canvases.access.set`](https://docs.slack.dev/reference/methods/canvases.access.set)
- `canvases:write` does not grant access to a given canvas
- Created outside a channel the bot is in: `canvases.edit` fails until it is shared
- Created as a channel tab: writable

## `search` returns private-repository PRs, but the permission granting it is undocumented [2026-08-22]

- Source: `gh api graphql`, `repo:<private repo> is:pr is:merged merged:>=` and `is:pr is:merged is:private`; [permissions for fine-grained PATs](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens); GitHub's docs data for `/search/issues` (`permissions: []`, `allowPermissionlessAccess: true`, `serverToServer: true`)
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
- One community report, not confirmed by Slack
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
- Available to `concurrency`: `github`, `inputs`, `vars`. So a group can be computed from the
  event name and a `workflow_dispatch` input, and split one workflow's runs into several groups
- `github.token` and `github.job` are the properties restricted to step execution. `run_id` is
  not among them
- For a non-`workflow_dispatch` event `inputs` is null, so `inputs.x == 'y'` is simply false
- With `cancel-in-progress` unset or false, a newly queued run still cancels any *pending* run in
  the group. Only the run in progress and the newest queued one survive a burst
- A shared group therefore lets frequent triggers cancel a pending scheduled run

## A `pull_request` `types:` list replaces the default `opened, synchronize, reopened` [2026-09-06]

- Source: [events that trigger workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows)
- "By default, a workflow only runs when a `pull_request` event's activity type is `opened`,
  `synchronize`, or `reopened`". Naming any `types:` narrows the trigger to exactly that list,
  so `types: [closed, ready_for_review]` never fires on a newly opened PR
- The full type list: `assigned`, `unassigned`, `labeled`, `unlabeled`, `opened`, `edited`,
  `closed`, `reopened`, `synchronize`, `converted_to_draft`, `locked`, `unlocked`, `enqueued`,
  `dequeued`, `milestoned`, `demilestoned`, `ready_for_review`, `review_requested`,
  `review_request_removed`, `auto_merge_enabled`, `auto_merge_disabled`
- `closed` fires for a merge and for a close without merging. `github.event.pull_request.merged`
  tells them apart
- `pull_request_review` (`submitted`, `edited`, `dismissed`) and `pull_request_review_comment`
  (`created`, `edited`, `deleted`): "By default, all activity types trigger workflows"

## `issue_comment` fires for pull request comments, and `github.event.issue.pull_request` filters them [2026-09-06]

- Source: [events that trigger workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows)
- "The `issue_comment` event occurs for comments on both issues and pull requests. You can use
  the `github.event.issue.pull_request` property in a conditional to take different action
  depending on whether the triggering object was an issue or pull request"
- Actions documents three types for it: `created`, `edited`, `deleted`. A bare `issue_comment:`
  takes all three, so an issue-only repository comment triggers a run unless the conditional
  guards it

## A fork's `pull_request` and `pull_request_review` runs get no secrets, and events fire only in the repo holding the workflow [2026-09-06]

- Source: [events that trigger workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows);
  [create a repository dispatch event](https://docs.github.com/en/rest/repos/repos#create-a-repository-dispatch-event)
- "With the exception of `GITHUB_TOKEN`, secrets are not passed to the runner when a workflow is
  triggered from a forked repository. The `GITHUB_TOKEN` has read-only permissions in pull
  requests from forked repositories". The page repeats this under `pull_request` and under
  `pull_request_review`; the `issue_comment` section says nothing about forks or secrets
- So a fork PR triggers the base repository's workflow with no `SLACK_BOT_TOKEN`
- "For pull requests from a forked repository to the base repository, GitHub sends the
  `pull_request`, `issue_comment`, `pull_request_review_comment`, `pull_request_review`, and
  `pull_request_target` events to the base repository. No pull request events occur on the
  forked repository"
- A PR in another repository cannot trigger this repository's workflow. The documented
  cross-repository path is `POST /repos/{owner}/{repo}/dispatches` (`repository_dispatch`) or
  the `workflow_dispatch` API, both needing a PAT or App token, never `GITHUB_TOKEN`
- Workflow file version per event: `pull_request` and `pull_request_review` run the PR merge
  ref's copy; `issue_comment`, `schedule` and `push` run the copy on the ref the event names,
  and "this event will only trigger a workflow run if the workflow file exists on the default
  branch" for `issue_comment` and `schedule`

## A single inline diff comment fires `pull_request_review: submitted` [2026-09-06]

- Source: live test in this repository, PR #58 (closed), workflow run 34025576473
- One comment posted with `POST /repos/{owner}/{repo}/pulls/58/comments`, the endpoint the
  "Add single comment" button calls. No review started, none submitted
- GitHub wrapped it in an implicit review: id 5124970470, `state: COMMENTED`, empty body
- The `PR Reminder` workflow, whose only `pull_request_review` type is `submitted`, ran two
  seconds later: comment at 09:46:12Z, run at 09:46:14Z. So the implicit review submits
- A `pull_request_review_comment` trigger is therefore redundant for catching a lone diff comment
- Not checked: the UI button itself, only the REST endpoint behind it

## `PullRequest.reviewThreads` exposes `isResolved` and `isOutdated` as independent booleans, 100 per page [2026-09-12]

- Source: GraphQL introspection of `PullRequestReviewThread`; [GraphQL pulls reference](https://docs.github.com/en/graphql/reference/pulls); `gh api graphql` against `kubernetes/kubernetes`
- `reviewThreads(first: n)` returns `PullRequestReviewThreadConnection!`, with `totalCount`
- `first: 101` fails with `EXCESSIVE_PAGINATION`, so 100 is the page maximum. `totalCount` is
  the only way to see that a PR has more
- `isResolved`, `isOutdated` and `isCollapsed` are all `Boolean!`
  - `isResolved`: "Whether this thread has been resolved"
  - `isOutdated`: "Indicates whether this thread was outdated by newer changes"
- Outdated does not imply resolved. All four combinations occur across 30 sampled open
  `kubernetes/kubernetes` PRs: PR 141851 held 3 threads outdated and unresolved, PR 141732
  held 3 outdated and resolved
- Which changes make a thread outdated is not documented beyond "newer changes"
- `isCollapsed` matched `isResolved` on all threads in that sample, but GitHub never states
  they are the same, so they are not interchangeable

## `PullRequest.reviewDecision` is null on an approved PR in a repository without required-reviewer rules [2026-09-12]

- Source: [community discussion 24375](https://github.com/orgs/community/discussions/24375); `gh api graphql` against `kubernetes/kubernetes` and `hellej/pr-slack-reminder-action`
- Rules it out as an approval signal. Derive approval from `reviews.nodes[].state` instead
- The field is nullable, with only `APPROVED`, `CHANGES_REQUESTED` and `REVIEW_REQUIRED`
- 59 of 60 sampled open `kubernetes/kubernetes` PRs returned null, including PRs 141630 and
  141345 which each carry an approving review. That repository enforces review through Prow
  and OWNERS, not through GitHub
- Every PR in `hellej/pr-slack-reminder-action` returned non-null, under a ruleset setting
  `requiredApprovingReviewCount: 1`
- GitHub Support states only that the field "will be null if it is not in one of the
  following states", declining to name the setting that drives it. The link to
  required-reviewer rules is this run's measurement, not a documented contract

## `PullRequest.mergeable` returns `UNKNOWN` while GitHub computes it, and only REST documents the retry [2026-09-12]

- Source: [MergeableState enum](https://docs.github.com/en/graphql/reference/pulls); [REST get a pull request](https://docs.github.com/en/rest/pulls/pulls?apiVersion=2022-11-28#get-a-pull-request); `gh api graphql` against `kubernetes/kubernetes`
- `mergeable: MergeableState!` is `MERGEABLE`, `CONFLICTING` or `UNKNOWN`, described as
  "whether or not the pull request can be merged based on the existence of merge conflicts"
- `UNKNOWN` means "the mergeability of the pull request is still being calculated"
- A single query does return it: `kubernetes/kubernetes` PR 142050 came back `UNKNOWN` in a
  30-PR batch, then `MERGEABLE` on each of the next three polls seconds later
- REST says to resubmit after giving the background job time. No GraphQL page repeats that
  advice, and the REST-null to GraphQL-`UNKNOWN` mapping is inferred from the enum
  description rather than stated
- `mergeStateStatus` carries richer detail, and returned `BLOCKED` for that PR. It has
  needed a preview `Accept` header on some deployments, so it needs its own check

## No GitHub page documents the token permission for any GraphQL field [2026-09-12]

- Source: [permissions for fine-grained PATs](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens)
- That page is REST-endpoint only. It never mentions GraphQL
- The closest signal for `reviews`, `comments` and `reviewThreads` is that their REST
  equivalents, `pulls/{n}/reviews` and `pulls/{n}/comments`, both sit under Pull requests at
  read with nothing additional
- Confirming a field needs a run with a fine-grained PAT holding only `pull-requests: read`
  plus the automatic `metadata: read`, checking the connection comes back populated
- The failure mode is silent: an ungranted connection can return empty rather than
  `FORBIDDEN`. Same gap as the `commits` entry above

## `mergeable` and `reviewThreads` both populate under `pull-requests: read` on `GITHUB_TOKEN` [2026-09-12]

- Source: workflow run 34711484631, `pr-reminder` on branch `canvas-open-pr-buckets`, whose
  `reminder` job grants `contents: read`, `actions: read`, `pull-requests: read`
- Both fields came back populated on a public repository: PR 61 logged 1 review thread with
  `mergeable: "MERGEABLE"`, and PR 3 logged `mergeable: "CONFLICTING"`, so neither field is
  the silent-empty failure the entry above warns about
- `reviewThreads`' nested `comments(last: 1){ nodes { author { login __typename } } }`
  resolved too: PR 61's own thread, last commented by the PR author, was read as answered
  rather than as a thread with no comments
- This narrows the gap above rather than closing it. The job grants `contents: read` as well,
  so it does not isolate `pull-requests: read`, and the repository is public. Closing it still
  needs a private repository under a fine-grained PAT holding only `pull-requests: read`

## `mergeable` rides on an endpoint needing Pull requests read OR Contents read, not both [2026-09-12]

- Source: [REST get a pull request](https://docs.github.com/en/rest/pulls/pulls?apiVersion=2022-11-28#get-a-pull-request); [permissions for fine-grained PATs](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens)
- The endpoint page states the token "must have at least one of the following permission
  sets: Pull requests repository permissions (read), Contents repository permissions (read)"
- So a checkmark in the Contents table against this endpoint is the other arm of the OR, not a
  second requirement. The endpoints that genuinely need Contents are the merge ones, at write
- `pulls/{n}/reviews` and `pulls/{n}/comments` carry no such marker at all: plain Pull requests
  at read
- Unauthenticated `curl` against a public repository returns the `mergeable` attribute, `null`
  on the first call and `true` on the second, which is the background compute, not permission
- Unverified: that the GraphQL field behaves like its REST equivalent under a restricted token.
  Confirming it needs a fine-grained PAT holding only `pull-requests: read` against a private
  repository

## GitHub's GraphQL API has no anonymous access, so REST's public-resource carve-out does not transfer [2026-09-12]

- Source: `curl -X POST https://api.github.com/graphql`; [workflow syntax `permissions`](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#permissions); [choosing permissions for a GitHub App](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/choosing-permissions-for-a-github-app)
- A tokenless GraphQL POST returns HTTP 403. The same-shaped REST read returns 200
- REST documents per endpoint that it "can be used without authentication or the aforementioned
  permissions if only public resources are requested". No page extends that to GraphQL
- Specifying any permission in a workflow sets every unspecified one to `none`
- `GITHUB_TOKEN` is an App installation token, and the implicit public-read fallback GitHub
  documents is scoped to user access tokens, "when acting on behalf of a user". For an
  installation token the page says success "only depends on the app's permissions"
- So a green run in a public repository whose job grants `contents: read` proves the query
  works, never that a permission was unnecessary

## A Slack `header` block takes a `level` of 1-4, and it is a message's only text larger than bold [2026-09-18]

- Source: [header block](https://docs.slack.dev/reference/block-kit/blocks/header-block);
  [rich text block](https://docs.slack.dev/reference/block-kit/blocks/rich-text-block);
  [formatting message text](https://docs.slack.dev/messaging/formatting-message-text);
  `slack-go@v0.29.0/block_header.go`, `block_rich_text.go`
- `level` is an optional integer, 1-4 for H1-H4. `slack-go` builds it with
  `NewHeaderBlock(textObj, HeaderBlockOptionLevel(n))`
- `header` text must be a `plain_text` object, at most 150 characters: no links, no bold,
  no italics inside it
- `rich_text` has no heading element. Its element types are section, list, quote and
  preformatted, and `RichTextSectionTextStyle` carries bold, italic, strike, code,
  underline, highlight and unlink, no size
- `mrkdwn` in a `section` block has no `#` heading syntax

## One Slack `rich_text` block holds any mix of sections and lists, with no documented element cap [2026-09-18]

- Source: [rich text block](https://docs.slack.dev/reference/block-kit/blocks/rich-text-block);
  `slack-go@v0.29.0/block_rich_text.go`
- `elements` is an array of `rich_text_section`, `rich_text_list`, `rich_text_quote` and
  `rich_text_preformatted` objects, freely mixed. The only limit the page states on the block
  is `block_id` at 255 characters
- `slack-go` builds it with the variadic `NewRichTextBlock(blockID, elements...)`, over
  `Elements []RichTextElement`
- So a heading run, sub-heading runs and their lists fit in one block, and block count need not
  grow with the number of lists rendered
- How Slack spaces adjacent elements inside one block is not documented: only a live post shows it

## An undeclared `with:` input warns on every run, it does not fail it [2026-09-19]

- Source: [actions/runner#514](https://github.com/actions/runner/issues/514)
- The runner emits `##[warning]Unexpected input '<name>', valid inputs are [...]` and continues
- So removing an input from `action.yml` leaves every workflow that still sets it with a yellow
  annotation on each run, until the caller deletes the line
- [Metadata syntax](https://docs.github.com/en/actions/reference/workflows-and-actions/metadata-syntax)
  documents `INPUT_<VARIABLE_NAME>` creation for declared inputs only, and does not cover this case

## `chat.update` and `chat.delete` take a channel ID only, where `chat.postMessage` also takes a name [2026-09-19]

- Source: live posts to the dev channel with `.agents/skills/slack-message-probe/send.sh`;
  [chat.update](https://docs.slack.dev/reference/methods/chat.update)
- `chat.postMessage` with `"channel": "pr-reminders-test"` returns `ok` and the channel ID
- The same name on `chat.update` returns `ok: false`, `error: "channel_not_found"`
- So an edit needs the ID the post's response carried

## Slack rewrites a unicode emoji inside a `rich_text` text run into an `emoji` element [2026-09-19]

- Source: live post to the dev channel, comparing the sent payload with the `message.blocks` the
  response returned
- `{"type": "text", "text": "✅ Ready to merge", "style": {"bold": true}}` comes back as an
  `emoji` element with `name: "white_check_mark"` plus a `text` element holding `" Ready to merge"`,
  each keeping the style
- Rendering is unchanged, so a payload does not need to send `emoji` elements itself
- A read-back of a message is therefore not byte-comparable with what was sent

## Slack renders a timestamp in the reader's own timezone with `<!date^unix^{token}|fallback>` [2026-09-23]

- Source: [formatting message text](https://docs.slack.dev/messaging/formatting-message-text);
  live post to the dev channel
- Tokens: `{date_num}`, `{date}`, `{date_short}`, `{date_long}`, the three `_pretty` variants,
  `{time}`, `{time_secs}`, `{ago}`
- `{time}` renders 12-hour or 24-hour by the reading client's own setting. No token forces either
- `{date}` renders `February 18th, 2014`, `{date_short}` `Feb 18, 2014`, `{date_long}`
  `Tuesday, February 18th, 2014`. Each omits the year within six months of now
- The `_pretty` variants read `yesterday`, `today` or `tomorrow` where it applies, otherwise their
  base token's form
- The text after `|` shows when a client cannot process the date, so it carries the timezone the
  sender means
- It works inside a `context` block's `mrkdwn` element, and `_`-wrapping the whole line italicises
  the rendered time with it

## `slack-go` v0.29.0 builds a context block from `MixedElement`s, and `*TextBlockObject` is one [2026-09-19]

- Source: `slack-go@v0.29.0/block_context.go`, `block_object.go`
- `NewContextBlock(blockID string, mixedElements ...MixedElement) *ContextBlock`
- `*TextBlockObject` implements `MixedElement` through `MixedElementType()`, so an mrkdwn context
  line needs no wrapper type: `NewContextBlock("", NewTextBlockObject("mrkdwn", text, false, false))`

## A Slack canvas renders `<!date^…>` as raw text, so canvas markdown has no local-time option [2026-09-19]

- Source: live `canvases.edit` `insert_at_end` against the dev channel canvas, four variants read
  back by eye in the Slack client
- `<!date^1789807636^{time}|08:47 UTC>` renders literally, angle brackets and all. So do
  `{date_num} {time}` and `{ago}`, and the fallback after `|` never takes over
- The token works in message `mrkdwn`, canvas markdown is a different renderer
- A canvas timestamp therefore has to name its timezone in the text, as `canvasbuilder`'s
  `_Updated <date> <time> UTC_` does
- `canvases.create` is refused on a free workspace (`free_teams_cannot_create_standalone_canvases`),
  and `conversations.canvases.create` on a channel that already has one
  (`free_team_canvas_tab_already_exists`). A probe has to edit the existing canvas
- `conversations.info` gives the canvas's file ID under `channel.properties.tabs[]`, where a
  `type: "canvas"` tab carries `data.file_id`. `properties.canvas` was absent

## Two adjacent Slack `header` blocks stack their padding, and Block Kit exposes no spacing control [2026-09-21]

- Source: live `post` runs of this action against the dev channel off
  `message-sections-by-next-action`, read in the Slack client
- A `header` block carries its own vertical padding. An H2 header immediately followed by an H3
  header leaves a gap wider than either alone, with nothing between them in the payload
- No block or element field sets spacing. The only lever is an extra block, a `section` block
  holding a single space
- A spacing block cannot sit inside a `rich_text` block, whose elements are section, list, quote
  and preformatted only. So rows that need a spacing block between them have to be split across
  blocks
- Where a heading precedes a list with no gap wanted, a bold text run at the head of the list's
  own `rich_text` block renders tighter than a `header` block does

## An action input's `default:` applies only when `with:` omits the key, so an explicit `""` stays empty [2026-09-22]

- Source: `actions/runner`, `src/Runner.Worker/ActionRunner.cs` on `main`, line 214:
  `if (!inputs.ContainsKey(key)) { inputs[key] = manifestManager.EvaluateDefaultInput(...) }`
- The runner decides on whether the caller's `with:` block contains the key at all, never on the
  value. A workflow that sets an input to `""` keeps `""`; only omitting the key entirely gets the
  `action.yml` default
- This holds for a string-typed input the same as `old-pr-threshold-hours` (int-shaped) or
  `group-by-repository` (bool-shaped): every action input is a string to the runner regardless of
  how its value reads

## `slack-go` v0.21.1 and later return no `blocks` from `UnsafeApplyMsgOptions` [2026-09-23]

- Source: `slack-go@v0.29.0/CHANGELOG.md`, `## [0.21.1]`; `chat.go` `UnsafeApplyMsgOptions`,
  `formSender.BuildRequestContext`
- Blocks are marshalled at send time inside `formSender.BuildRequestContext`, as
  `json.Marshal(blockSet)`. `UnsafeApplyMsgOptions` returns the values before that, so they carry
  no `blocks` key
- The bytes a message is sent with are therefore `json.Marshal(message.Blocks.BlockSet)`
- `chat.update` goes through the same `formSender` as `chat.postMessage`:
  `sendConfig.BuildRequestContext` picks it for every mode but `chatResponse`

## `slack.BlockFromJSON` re-sends one block's JSON byte for byte [2026-09-23]

- Source: `slack-go@v0.29.0/block_json.go`
- Returns a `RawJSONBlock` whose `MarshalJSON` returns the stored bytes unchanged
- Given a JSON array, it keeps only the first block. A stored message has to be split into
  per-block `json.RawMessage` values first
- `MsgOptionBlocks` sends blocks through `json.Marshal`, so a `RawJSONBlock` goes out as stored
- Unmarshalling into `slack.Blocks` also covers today's block types, but nothing in the library
  tests that round trip for `header`, `rich_text` lists or `context`

## A Slack message link without the workspace subdomain opens the message [2026-09-23]

- Source: the maintainer clicked `https://slack.com/archives/<channel ID>/p<ts>` in the Slack
  client, and it opened the message
- `p<ts>` is the message timestamp with its dot removed: `1790146735.683649` becomes
  `p1790146735683649`
- The link Slack's own "Copy link" gives is `https://<workspace>.slack.com/archives/...`. The
  subdomain is not in any `chat.postMessage` response, and `auth.test` would be an extra call

## `chat.update` errors: `message_not_found`, `cant_update_message`, and `edit_window_closed` from the workspace's edit settings [2026-09-23]

- Source: [chat.update](https://docs.slack.dev/reference/methods/chat.update)
- `message_not_found`: "No message exists with the requested timestamp"
- `cant_update_message`: "Authenticated user does not have permission to update this message",
  also returned for message types the method cannot update
- `edit_window_closed`: "The message cannot be edited due to the team message edit settings".
  Unverified: whether those settings apply to a bot editing its own message
- `block_mismatch`: "Rich-text blocks cannot be replaced with non-rich-text blocks"
- A message with a `bot_id` never shows the `(edited)` label
- Slack always renders from `blocks` when given, and uses `text` only for notifications

## `chat.update` documents no `unfurl_links` or `unfurl_media`, where `chat.postMessage` does [2026-09-23]

- Source: [chat.update](https://docs.slack.dev/reference/methods/chat.update) arguments;
  `slackapi/slack-api-specs` `web-api/slack_web_openapi_v2.json`, `paths["/chat.update"]`
  parameters; `slackapi/node-slack-sdk` `main`, `packages/web-api/src/types/request/chat.ts`
  `ChatUpdateArguments`; `slackapi/python-slack-sdk` `main`, `slack_sdk/web/client.py`
  `chat_update`
- `chat.update` takes `as_user`, `attachments`, `blocks`, `channel`, `link_names`, `parse`,
  `text`, `ts`. The Node SDK adds `file_ids`, `reply_broadcast` and metadata, still no unfurl
  arguments
- slack-go's `MsgOptionDisableLinkUnfurl` sets `unfurl_links=false` on whatever endpoint the
  message goes to (`slack-go@v0.29.0/chat.go`), so it sends on an update, but Slack does not
  document honouring it there
- So an edit that adds a link has no documented way to suppress its preview
- [Unfurling links in messages](https://docs.slack.dev/messaging/unfurling-links-in-messages):
  in its `chat.postMessage` example, a link to text content does not unfurl unless
  `unfurl_links: true` is passed. Media links unfurl by default, inside Block Kit blocks too. The
  page says nothing about edits
- The maintainer sees no preview on PR links, including ones an update run's `chat.update` adds to
  the message after the post
