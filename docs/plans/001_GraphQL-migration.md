# GraphQL migration

date: 2026-08-15
status: done

Move `githubclient`'s PR fetching from GitHub's REST API to its GraphQL API. The artifact path stays on REST.

## Goals

- Same PRs, same filtering, same message output — except author and reviewer display names, which start rendering (see "Display names").
- No new action inputs, no new token permissions, no change to `action.yml`.
- Cut the per-PR call fan-out.
- Land before the PR tracker canvas ([002](002_PR-tracker-canvas.md)), which is the feature that most needs what GraphQL provides — the head-commit date and real display names.

Measured against the live API, 2026-08-09.

- **Rate limit.** `GITHUB_TOKEN` gets [1,000 points/hour per repository on GraphQL](https://docs.github.com/en/graphql/overview/rate-limits-and-node-limits-for-the-graphql-api) and [1,000 requests/hour per repository on REST](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api). At the action's own caps that is 31 `post` runs/hour against today's 4-5 (see "Cost model"). In practice the budget is larger: see "Observed budget".
- **Separate buckets.** After draining GraphQL to 4893/5000, `GET /rate_limit` still reported `core: 5000/5000`. REST spend and GraphQL spend do not compete, so the action stops eating the REST budget shared with every other workflow in the repository.
- **Head-commit date.** `commits(last: 1) { nodes { commit { committedDate } } }` replaces a paginated commit scan with a SHA cross-check, a 250-commit ceiling and an `updated_at` fallback that overstates freshness. That timestamp is what 002's WIP section sorts, marks and prunes by.
- **Display names.** REST's PR-list and review payloads carry no `name` field at all (verified on `/pulls` and `/pulls/{n}/reviews`), so `Collaborator.Name` is always empty today and `GetGitHubName()` always falls back to the login. GraphQL returns names.

### Non-goals

- Migrating `FetchLatestArtifactByName`. Actions artifacts are REST-only, so `githubclient` keeps both clients.
- Retries or rate-limit backoff beyond the HTTP-level retry Step 1 adds. `githubclient` has none today.
- Paginating past the first page of open PRs per repository. Today's REST path fetches one page of 100 and caps; the GraphQL path matches that exactly.
- Any canvas work. 002 is replanned against this once it lands (see Consequences).

## Target shape

- **`githubclient.Client` interface is unchanged.** `FindOpenPRs`, `GetPRs` and `FetchLatestArtifactByName` keep their signatures, so no consumer above the client changes how it calls it. R1 still retypes a few field reaches in `prparser` and `state`.
- **`githubclient.PR` stops embedding `*github.PullRequest`** and embeds an own `*PullRequest` instead, exposing the same getters the pipeline already calls: `GetNumber`, `GetTitle`, `GetHTMLURL`, `GetCreatedAt`, `GetUpdatedAt`, `GetState`, `GetMerged`, `GetDraft`.
- **New `internal/apiclients/githubclient/graphql.go`**: a stdlib GraphQL transport. `net/http` + `encoding/json`, one `Do` entry point (Step 1). go-github has no GraphQL client; a query-builder library buys nothing at two queries.
- **Two-phase fetch**, mirroring the list-then-enrich shape the REST path already has:
  - Phase 1, one request: aliased `repository(owner:, name:)` per configured repository, each with `pullRequests(states: OPEN, first: 100, orderBy: {field: CREATED_AT, direction: DESC})`, selecting scalars, author and labels — everything filtering and capping need.
  - Phase 2, batched requests: aliased `repository(...) { pullRequest(number:) }` for the capped set only, selecting `reviews`, `comments` and `commits(last: 1)`.
- **Test shape.** The E2E tests through `main.Run` (`cmd/pr-slack-reminder/main_test.go`) are the primary net, and R0 adds payload snapshots there — that is what proves the swap changed nothing. Per-package tests below it serve the step that adds them: one table-driven test with named scenario rows per behaviour, not a new test function per case. Coverage that already exists is kept; the suite doesn't grow a case per fixture variant.
- **Permissions unchanged.** GraphQL authorizes the same objects through the same model — permissions are a property of the token, not the API surface. GitHub's wording: *"The data that you are requesting will dictate which scopes or permissions you will need"* ([forming calls](https://docs.github.com/en/graphql/guides/forming-calls-with-graphql)).
- **`issues: read` stays required.** GitHub publishes no per-field GraphQL permission mapping — *"you should test your app to ensure that it has the required permissions"* ([choosing permissions](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/choosing-permissions-for-a-github-app)). REST's `/issues/{n}/comments`, which `pullRequest.comments` replaces, is listed under **both** Issues and Pull requests ([permissions required](https://docs.github.com/en/rest/authentication/permissions-required-for-github-apps)), so `pull-requests: read` may already cover it. Unverified for GraphQL and unverifiable in Step 7, so the README keeps requiring it.

### Cost model

Points are charged per **connection resolution**, summed, divided by 100, rounded, minimum 1 — not per node returned.

A nested connection is charged once per parent slot *requested*, whatever comes back, and its own page size is free: `labels` under `pullRequests(first: 100)` costs 100 per repository at any `first`, so `labels(first: 20)` and `labels(first: 100)` both put phase 1 at 30 × (1 + 100) / 100 = 30 points, ~1 per repository, independent of PR count. Leaf pages are therefore sized for correctness.

Measured at the action's own caps — `MaxRepositories` 30 (`config.go`), `MaxPRsToFetch` 50 (`githubclient.go`):

| Query | Cost | `nodeCount` | Wall time |
| --- | --- | --- | --- |
| Phase 1, 30 repositories, 2,653 PRs listed | 30 | 63,000 (`labels(first: 20)`) / 303,000 (`labels(first: 100)`) | 8-11 s |
| Phase 2, 25 aliases | 1 | 5,025 | ~2 s |
| Phase 2, 50 aliases in one request | 2 | 10,050 | ~2.5 s |

So a worst-case `post` run costs **32 points** (30 + two batches of 25) and a worst-case `update` run skips phase 1 and costs **2 points**.

### Observed budget

Step 7's `pr-reminder.yml` dispatch ran on `secrets.GITHUB_TOKEN` and logged `remaining 4999 of 5000`, not the documented 1,000. Both phases cost 1 point, so that one-repository `post` run spent 2 of 5,000.

The documented 1,000 is the figure the headline above is derived from, and it stays the number to plan against: one observation on one public repository does not establish the ceiling every repository gets, and `rateLimit.limit` reports what the token is granted, not what is enforced. Read the headline as a floor. At the observed 5,000 a worst-case `post` run of 32 points fits 156 times an hour.

Step 7 was meant to prove `GITHUB_TOKEN` reaches GraphQL at all. It did, and the budget came back larger than planned for.

### Query size limits

One query covering everything is not viable. Nested `reviews`/`comments` under `pullRequests` across 10 repositories returned **676 `RESOURCE_LIMITS_EXCEEDED` errors** with HTTP 200 and otherwise-valid data, each scoped to one nested connection (`path: ["r7","pullRequests","nodes",59,"comments"]`) — those PRs arrive with reviewers silently missing. Not the documented 500,000-node ceiling: the failing query was ~61,000 nodes while phase 1 returns 303,000 cleanly, so it behaves like an undocumented server-side execution budget. Phase 2's measured boundary:

| Aliases per request | Result |
| --- | --- |
| 25 | clean, 1 point |
| 50 (the whole capped set in one request) | clean, 2 points |
| 65 | clean, 2 points, 6/6 on repeat |
| 100 | clean, 3 points |
| 150 | HTTP 502 |
| 300 | `"We couldn't respond to your request in time"` |

Phase 2 therefore batches at **25 aliases per request**, putting the worst case an order of magnitude below the observed boundary. Cost barely moves: the 1-point-per-query minimum makes two batches 2 points, the same as one 50-alias request.

### Reviewer derivation

The REST path derives approvers and commenters from three calls: `ListReviews`, `ListComments` (review comments) and the issue-comments timeline. GraphQL needs only two connections, and review comments are not fetched at all.

Three participant kinds:

| Participant | Covered by |
| --- | --- |
| Comments on the diff without submitting a review | `reviews` — GitHub wraps the comment in an implicit review record |
| Submits a full review (approved / changes-requested / commented) | `reviews` directly |
| Comments only on the timeline | `comments` |

**Every review comment carries a non-null `pull_request_review_id`**, bare diff comments included — those get an implicit `COMMENTED` review with an empty body and the same author (`cli/cli#14104`: comment by `williammartin` → review `4885327638`). Measured across every review comment in 15 PRs with review activity in `cli/cli`, `grafana/grafana` and `prometheus/prometheus`, plus a REST-vs-GraphQL set diff of `(author, approvers, commenters)` over 22 PRs in three query shapes: **zero mismatches** beyond bot logins. Review-comment authors are therefore always a subset of review authors, and nesting comments under reviews would add ~20 connection resolutions per PR for nothing.

`comments` on `PullRequest` is the issue-comment timeline, the same data `/issues/{n}/comments` returns today, so it feeds both commenter derivation and `/snooze` parsing.

### Collaborator mapping

`author` is a nullable `Actor` whose possible types are `Bot`, `EnterpriseUserAccount`, `Mannequin`, `Organization` and `User` (schema introspection, 2026-08-09). One mapper covers every author position — PR author, review author, comment author:

- `author: null` (deleted account) → zero-value `Collaborator`. This matches today: `newCollaboratorFromUser(nil)` yields empty `Login`/`Name` through go-github's nil-safe getters.
- `__typename == "Bot"` → `Login` gains a `[bot]` suffix (see "Bot logins"), `Name` empty. Bots have no `name` field.
- `__typename == "User"` → `Login` and `Name` as returned; `name` is often null on real accounts, and `GetGitHubName()` falls back to the login there.
- Any other `__typename` → `Login` as returned, `Name` empty, no suffix. `... on User { name }` does not match them.

Bot **exclusion** happens where `__typename` is read, not in derivation: review and comment nodes with a `Bot` author, or an empty login, are dropped before collaborator lists are built. That reproduces today's `hasValidUserData` guard (`models.go`), which filters bots out of reviews, review comments and timeline comments before extraction. Consequently the `[bot]` suffix only ever reaches the PR author — the one position that is never filtered — and reviewer derivation never sees a bot.

Snooze detection keeps reading raw timeline comments including bot-authored ones, per `githubclient.spec.md`.

### Display names

`Collaborator.Name` starts being populated, so `GetGitHubName()` renders display names where it has always rendered logins. Two positions differ:

- **Reviewers always change**, when GitHub has a name for them. `getReviewersElements` (`messagebuilder.go:160-214`) renders `GetGitHubName()` for every approver and commenter and never mentions.
- **Authors change only when unmapped.** `getUserNameElement` (`messagebuilder.go:149-158`) renders a Slack mention when `Author.SlackUserID` is set, and falls back to `GetGitHubName()` otherwise.

Accounts with no `name` set keep rendering the login. `SlackUserIdByGitHubUsername` keys on `Login`, which is unaffected, so mentions and mappings behave identically.

### Bot logins

GraphQL and REST disagree on bot logins — REST returns `dependabot[bot]`, GraphQL returns `dependabot`. `authors`, `ignored-authors` and `github-user-slack-user-id-mapping` all match on the login string, so the mapper appends `[bot]` when `__typename == "Bot"`. Without it, every existing `ignored-authors: dependabot[bot]` config silently stops matching.

### Error handling

GraphQL reports failure as HTTP 200 with an `errors` array, so status codes stop being the signal:

```json
{"data":{"r0":null},
 "errors":[{"type":"NOT_FOUND","path":["r0"],
            "message":"Could not resolve to a Repository with the name 'owner/repo'."}]}
```

Errors are scoped to their `path`, so one unreadable repository nulls its own alias while every other alias still returns data, giving four classes:

| Class | `path` shape | Example | Behaviour |
| --- | --- | --- | --- |
| Repository-level | `["r0"]` | `NOT_FOUND`, `FORBIDDEN` on a `repository` alias | Fail the fetch, naming the repository — matches today's REST 404 branch |
| PR-level | `["p0","pullRequest"]` | `NOT_FOUND` on a requested PR number | In `GetPRs`, fail the fetch naming the PR — matches today's `fetchPR` 404 branch. In `FindOpenPRs`, degrade to field-level (Step 4) |
| Field-level | `["p0","pullRequest","comments"]` and deeper | `RESOURCE_LIMITS_EXCEEDED` on a nested connection | Log per PR, continue — matches today's per-PR partial-failure pattern |
| Whole-query | absent, empty, or not rooted at a known alias | `RATE_LIMITED` (no `path`), a validation error (`path: ["query", …]`) | Fail the fetch, surfacing `type`/`extensions.code` and `message` |
| Transport | — | HTTP 502, timeout body | Retry once, then fail |

PR-level is called out separately because it is not deeper-is-softer: a missing PR returns `{"type":"NOT_FOUND","path":["p0","pullRequest"]}` (verified live), and a length-based rule would demote today's hard error to a logged warning.

Classification is **positive, never by elimination**: an error is field-level only if its `path` starts with an alias the caller generated; everything else is whole-query and fails. Two real shapes need that, both arriving as HTTP 200:

- Rate-limit exhaustion returns `{"data":null,"errors":[{"type":"RATE_LIMITED", …}]}` with no `path` at all.
- A malformed query returns **no `data` key** and a path rooted at `query` — verified live: `{"errors":[{"path":["query","repository","notAField"],"extensions":{"code":"undefinedField"}, …}]}`, with no `type` field, so `extensions.code` is the identifier.

Under a rule that treats "deeper than an alias" as field-level, both would be logged per PR and the fetch would return an empty list — an empty reminder in `post`, a deleted message in `update`. A missing or null `data` is likewise a hard failure, never an empty result.

`NOT_FOUND` covers both "doesn't exist" and "no permission", exactly as REST's 404 does, so today's *"check the repository name and permissions"* and *"check the path and permissions"* wordings stay correct.

### Timeout budget

`cmd/pr-slack-reminder/run.go:57,105` wrap the whole fetch in `prFetchTimeout` (60s). Retries and per-request timeouts fit inside it:

- `Do` makes at most two attempts and both share the caller's context, so a phase's per-request timeout covers the pair. An attempt that exhausts the window leaves no time for a second.
- Retry on 5xx, 429, network errors and unparseable bodies, after `retryDelay` (1s). Never on 401 or 403.
- `post`: phase 1 (`PullRequestListTimeout`, 30s) then phase 2 (`ReviewsFetchTimeout`, 10s per batch, with at most two batches running concurrently at limit 3) = 40s worst case.
- `update`: phase 2 only = 10s worst case.

### Risks

| Risk | Mitigation | Residual |
| --- | --- | --- |
| Undocumented execution budget rejects a query that passes today | Batch phase 2 at 25; per-request timeouts | A server-side budget change could still bite; field-level errors degrade rather than fail |
| Field-level errors ignored → reviewers silently missing | Inspecting `errors` is mandatory in the transport, with a test asserting a `RESOURCE_LIMITS_EXCEEDED` payload surfaces as a per-PR failure | None once tested |
| Field-level error on `comments` → snoozed PRs reappear | Same inspection; the per-PR log line names the lost connection, and a test covers the snooze case specifically | A snoozed PR can still surface for one run, as it can today when the timeline call fails |
| Bot login format change breaks `ignored-authors` | `[bot]` re-appended for `Bot` authors, table-driven test | None |
| Review-comment authors ⊆ review authors is GitHub's observed behaviour, not a documented guarantee | See "Reviewer derivation" | An orphan review comment would drop its author from 💬; display-only, no PR dropped or mis-ordered |
| Repository-level or PR-level error mapped wrongly → misconfigured repo becomes a silent empty list | Explicit `errors[].type`/`path` mapping, test per class | None once tested |
| Test suite is rewritten wholesale — the fetch path's only regression net | R0 pins the rendered Slack payload byte-for-byte before anything moves, so Steps 5-6 must leave every snapshot unchanged; R2 moves derivation under direct unit tests that survive the mock swap, and filtering stays on `main_test.go`'s cases, which Step 5 keeps | A query-shape regression the mock renderer reproduces faithfully (a wrong `orderBy`, a wrong page size) is invisible to the snapshots and still rests on Step 7's five live runs — three e2e messages plus the two `pr-reminder.yml` dispatches |
| `GITHUB_TOKEN`'s GraphQL budget behaviour is documentation, not observation | Step 7 dispatches `pr-reminder.yml`, which runs on `secrets.GITHUB_TOKEN`, before the REST path is deleted | Closed: the `post` dispatch reached GraphQL and reported a 5,000-point budget (see "Observed budget") |
| GraphQL needs a permission REST does not on **private** repositories | Documented permission model; Step 7 confirms `GITHUB_TOKEN` reaches GraphQL | Unverifiable here: this repository's `GITHUB_TOKEN` reaches only this repository, which is public, so its permission block is not binding for PR reads |
| Ordering differs from REST when a repo has >100 open PRs | Phase 1 pins `CREATED_AT DESC`, matching REST's default (`sort=created`, `direction=desc`) | None |

## Breaking change classification

**Minor.** No input, output or permission changes, and no behaviour change to filtering, ordering, capping or snoozing. Display names newly render where logins did, which is visible to every user, so this is more than a patch — but it restores intended behaviour rather than breaking existing behaviour.

## Summary of steps

- R0. Refactor: snapshot the sent Slack payload in the E2E tests
- R1. Refactor: replace the embedded `*github.PullRequest` with an own type, and retype filtering onto it
- R2. Refactor: move reviewer derivation and snooze input onto neutral types, under direct tests
- R3. Refactor: consolidate the duplicated review and timeline-comment fixture helpers
1. GraphQL transport, error classification and client wiring
2. Collaborator mapper
3. Phase 1: list open PRs
4. Phase 2: enrich the capped set
5. `FindOpenPRs` on GraphQL
6. `GetPRs` on GraphQL
7. Verify against the live API with the existing workflows
8. Delete the REST PR path
9. Docs and spec sync

## Steps

### R0. Refactor: snapshot the sent Slack payload (`cmd/pr-slack-reminder/snapshot_test.go`, `cmd/pr-slack-reminder/testdata/snapshots/`, `Makefile`, `AGENTS.md`)

- The migration's regression net, recorded against today's REST output before anything moves. R1-R3 and Steps 5-6 are then checked by the git diff over `testdata/snapshots/`, which must be empty.
- Nothing in production changes. New `snapshot_test.go` in `package main_test`, reusing `main_test.go`'s fixtures and mocks and driving the full pipeline through `main.Run`.
- The payload is already plumbed end to end: `SendMessage`/`UpdateMessage` return `SentMessageInfo.JSONBlocks` (`slackclient.go:18`) and `run.go:151` passes them to `state.SaveSentSlackBlocks`, which writes them as indented JSON. Under the mocks the bytes come from `mockslackclient.getJSONBlocks` (`mockslackclient.go:202`), which marshals `message.Blocks.BlockSet` directly. The snapshot therefore pins blocks, not the summary text — that stays on `main_test.go`'s `expectedSummary` assertions (`:702-711`), which Steps 5-6 keep.
- The test points `config.EnvSentSlackBlocksFilePath` and `config.EnvStateFilePath` at `t.TempDir()` files — `confighelpers.go:42-43,67-68` defaults both under `/tmp`. The state file keeps the basename `pr-slack-reminder-state.json`: `FetchLatestArtifactByName` matches inside the artifact zip by basename (`fetchartifact.go:95`), and `mockgithubclient.createMockArtifactZip` (`mockgithubclient.go:251`) always writes that name, so any other basename fails update mode with *"json file … not found inside artifact zip"*.
- Compare the blocks file against `cmd/pr-slack-reminder/testdata/snapshots/<slug>.json`, committed. Case names carry spaces and commas, so the slug is `t.Name()` with every non-alphanumeric run collapsed to `-`. A missing snapshot file fails the case, naming `make update-test-snapshots`.
- One table with named scenario rows, each row a config + fixture set:
  - grouped by repository, two repositories
  - reviewers present: one approver and one commenter on the same PR, plus a second PR whose only reviewer input is a timeline comment — that one renders `messagebuilder.go:169`'s `" (💬 "`-only prefix, which R2's `deriveReviewers` and Steps 5-6's re-sourced connections would otherwise be free to drop
  - a PR past `old-pr-threshold-hours`, rendering the age indicator
  - an author mapped through `github-user-slack-user-id-mapping`, rendering a Slack mention — the other rows leave authors unmapped and pin the `GetGitHubName()` fallback, so without this row no snapshot covers `getUserNameElement`'s mention branch (`messagebuilder.go:149-158`)
  - the no-PRs message
- Update mode gets its own function (it needs `MockStateForUpdateMode` and reads `UpdateMessage`'s payload): one snapshot over an open PR, a merged PR and a closed-but-not-merged PR. `GetPRs` drains the same completion-ordered channel, so the distinct-age rule below covers it too.
- Recording: `var updateSnapshots = flag.Bool("update-snapshots", false, …)` in `snapshot_test.go`; when set, a case writes its file instead of comparing. `go test ./... -update-snapshots` fails in every package that doesn't register the flag (*"flag provided but not defined: -update-snapshots"*), so the target scopes to the one package:

  ```make
  update-test-snapshots:
  	go test ./cmd/pr-slack-reminder -count=1 -update-snapshots
  	@git add -N cmd/pr-slack-reminder/testdata && git diff --stat -- cmd/pr-slack-reminder/testdata
  ```

  `git add -N` first: a first recording writes untracked files, which `git diff` does not show.

- Set `Number`, `Title`, `AuthorLogin`, `AuthorName` and `Labels` on every PR. `getTestPR` (`main_test.go:41`) fills unset fields from `testhelpers.RandomString(10)` / `testhelpers.RandomPositiveInt()`, seeded from `time.Now().UnixNano()` (`testhelpers.go:14,26,33`). Labels never reach the payload, but leaving one field defaulted hides the rule.
- Every PR in a case gets a **`CreatedAt` that stays distinct after minute rounding** — fixture ages at least a minute apart. `getTestPR` rounds to whole minutes (`ageMinutes = math.Round(AgeHours*60)`), so 0.083 and 0.084 both give 5 minutes. Ties order arbitrarily: `sortPRsOldestToNewest` (`prparser.go:88-96`) falls back to `UpdatedAt`, which the fixtures never set, and `addReviewerInfoToPRs` (`githubclient.go:327-397`) drains a buffered channel in completion order. `getTestPRs` (`main_test.go:96`) gives PR2 and PR3 the same `AgeHours: 3` and cannot be used unmodified. `SomePRItemTextIsEqualTo` (`mockslackclient/models.go:95`) is order-insensitive, which is why today's assertions pass.
- Ages are not frozen: `GetPRAgeText` (`prparser.go:37-49`) and `isOlderThan` (`prparser.go:98-106`) read `time.Now()`, not `main_test.go:39`'s fixed `now`, so every rendered age grows with wall time. Keep ages clear of:
  - the branch's `math.Round` half-boundary — 30 s of headroom in minutes, 30 min in hours, up to 12 h in days at a bucket midpoint. Prefer hours and days.
  - the branch edges at 1 h and 24 h, from below: an age in [23.5 h, 24 h) renders "24 hours" now and "1 days" later.
  - `old-pr-threshold-hours`.
- `GetTestPROptions` gains an `HTMLURL` field, set on every snapshot fixture. It stays unset by default, so `main_test.go`'s cases are unaffected. Without it every PR link records an empty `url`, and nothing else in the repo asserts one — `githubclient_test.go` sets `HTMLURL` but never reads it back — so a wrong `pullRequest.url` mapping in Steps 5-6, or a zero-valued `GetHTMLURL` from R1's new type, would leave every snapshot byte-identical.
- List `make update-test-snapshots` in AGENTS.md § Development Commands, with the one-package scoping.

### R1. Refactor: own PR type (`githubclient.go`, `models.go`, `prfilter.go`, `internal/prparser`, `internal/state`)

- Add `PullRequest` to `models.go` with the fields the pipeline reads, and getters matching the go-github names already called downstream: `GetNumber`, `GetTitle`, `GetHTMLURL`, `GetCreatedAt`, `GetUpdatedAt`, `GetState`, `GetMerged`, `GetDraft`. `GetCreatedAt`/`GetUpdatedAt` return `time.Time`, dropping go-github's `Timestamp` wrapper.
- Also carry `Labels []string` and `Author Collaborator`, which `githubclient` reads internally, and `HeadSHA string`, which nothing in this plan reads — it is for 002, like `commits(last: 1)` in Step 4.
- `PR` embeds `*PullRequest` instead of `*github.PullRequest`; `PRResult.pr` and `FetchReviewsResult.pr` switch with it. `Repository`, `Author`, `ApprovedByUsers`, `CommentedByUsers`, `SnoozedUntil` are unchanged.
- `Author` lands on both types. Both stay: `PR`'s own field shadows the promoted one, `githubclient` reads `r.pr.Author` and consumers read `pr.Author` (`prparser.go:72`).
- Add a mapper from `*github.PullRequest` to the new type in `models.go`, below the `PullRequest` getters, called from `getPRResultMapper`, so the REST path keeps working through Step 7 and only Step 8 removes it.
- Outside `githubclient`, two files reach fields rather than getters: `internal/prparser/prparser.go:38` (`time.Since(pr.CreatedAt.Time)` → `time.Since(pr.GetCreatedAt())`) and `internal/state/state.go:43` (`Number: *pr.Number` → `pr.GetNumber()`).
- Inside `githubclient`, five more places drop the go-github wrappers: `githubclient.go:304` and `githubclient.go:351,358,365` (`*pr.Number` → `pr.GetNumber()`), `githubclient.go:317-320` (`capPRsToLimit` loses the `.Time` reaches; sort semantics unchanged), `models.go:72` (`r.pr.GetUser().GetLogin()` → `r.pr.Author.Login`) and `models.go:94` (`newCollaboratorFromUser(r.pr.GetUser())` → `r.pr.Author`).
- `includePR` (`prfilter.go:11`) retypes in the same step: `githubclient.go:240` passes `result.pr` to it, so leaving it on `*github.PullRequest` breaks the package. It takes `*PullRequest` and reads `Labels []string` and `Author.Login` instead of `[]*github.Label` and `GetUser().GetLogin()`.
- No new `prfilter_test.go`. `main_test.go` already covers `includePR` end to end and Step 5 keeps every case: labels inclusion (`:355`) and exclusion (`:362`), `authors` (`:380`), `ignored-authors` (`:369`, `:387`), repository-scoped filters (`:414`, `:432`, `:450`). `IgnoredTerms` is covered only by `githubclient_test.go:458-490`, which survives Step 5 as a fixture change.
- `prparser.go:90,91,93` — `sortPRsOldestToNewest`'s `GetCreatedAt().Time` / `GetUpdatedAt().Time` reaches lose the `.Time` too. `prparser.go:102,105` compile either way, since go-github's `Timestamp` embeds `time.Time`.
- `messagecontent` and `messagebuilder` compile unchanged — they only call getters.
- Test files that construct `githubclient.PR` directly move to the new type: `internal/prparser/prparser_test.go`, `internal/state/state_test.go`, `internal/messagebuilder/messagebuilder_test.go`. Their fixtures get shorter, since they stop needing `github.Ptr` for every field.
- Fixtures that feed the REST service mocks stay on go-github types: `cmd/pr-slack-reminder/main_test.go`'s `getTestPR` and every `*github.*` field on `testhelpers/mockgithubclient`'s `MockGitHubClientOptions`. Those types are the mock's input format and survive the migration (Steps 5-6 render them into GraphQL JSON); go-github stays in `go.mod` for the artifact path regardless.
- Confirm with `make test` before moving on; any downstream compile error means a getter was missed.

### R2. Refactor: derivation and snooze input on neutral types (`models.go`, `models_test.go`, `snooze.go`, `snooze_test.go`, `githubclient.go`)

- Move reviewer derivation out of `FetchReviewsResult.asPR()` (`models.go`) into `deriveReviewers(authorLogin string, approvers, reviewAuthors, reviewCommentAuthors, timelineCommenters []Collaborator) (approvedBy, commentedBy []Collaborator)`, over already-extracted lists. Both fetch paths then feed the same derivation, and Step 5 can't quietly change it.
- Review-comment authors stay a separate input even though GraphQL will always pass nil for them. REST still fetches them until Step 8, and today's `asPR()` feeds them from `r.comments` (`models.go:75,82`). Collapsing the two inputs early would change REST behaviour inside a refactor step.
- `deriveReviewers` keeps today's output order: `commentedBy` is `slices.Concat(reviewCommenters, standaloneCommenters, timelineCommenters)` deduped by login (`models.go:85`). It dedupes the approver input by login itself — the input is raw review authors, and a re-approval after changes gives one login two `APPROVED` reviews. Reviewer names render in list order.
- Order is the one derivation rule no test asserts — `TestFindOneOrNoPRs` compares reviewer logins with `slicesEqualIgnoreOrder` (`githubclient_test.go:580,584`). Add one table over `deriveReviewers`' inputs → `(approvedBy, commentedBy)` with two rows: commenters ordered reviews → review comments → timeline, and dedupe — the same login as a review commenter and a timeline commenter appearing once, and the same login twice among the approvers appearing once. Author exclusion, approver-excluded-from-commenters and the empty case are already asserted by `TestFindOneOrNoPRs` rows (`githubclient_test.go:268, 289, 315`), which Step 5 keeps.
- `findActiveSnooze` (`snooze.go:34`) takes `[]TimelineComment` instead of `[]*github.IssueComment`, where `TimelineComment` is a new struct in `models.go`:

  ```go
  type TimelineComment struct {
      Body        string
      CreatedAt   time.Time
      Author      Collaborator
      AuthorIsBot bool
  }
  ```

- `AuthorIsBot` is not optional. `Collaborator` (`models.go:45-48`) carries only `Login`/`Name`, but today the timeline is bot-filtered by `hasValidUserData` reading `user.GetType()` (`models.go:57-60,101-104`). Without the flag the bot signal is gone by the time commenters are derived, and `githubclient_test.go:451`'s `NewTimelineComment("bot-commenter","","Bot")` — expected excluded at `:456` — fails. The REST mapper fills it from `user.GetType() == "Bot"`; the GraphQL path fills it from `__typename` (Step 2). Pre-filtering in the mapper is not an option: snooze must keep reading the unfiltered timeline.
- `FetchReviewsResult.timelineComments` becomes `[]TimelineComment`; the REST path maps `*github.IssueComment` into it where that struct is built (`githubclient.go:372-379`), until Step 8 deletes the mapper.
- `models.go:76,83` lose their generic helpers for that field: `hasValidUserData` and `extractUniqueCollaborators` are constrained by `GitHubUserProvider`, which `TimelineComment` does not implement. Replace both with a non-generic pass reading `AuthorIsBot` and `Author.Login`. That pass is REST-only and lives as long as the REST path does — the GraphQL path drops bot nodes with Step 2's predicate instead, so timeline comments are never filtered twice. Reviews and review comments keep the generic helpers until Step 8.
- `TestFindActiveSnooze` (`snooze_test.go:119`) moves its fixtures (`:124-180`) from `[]*github.IssueComment` to `[]TimelineComment`, dropping `github.Ptr` and `github.Timestamp`. `TestParseSnoozeComment` is unaffected — it already takes `(string, time.Time)`.
- Bot exclusion stays out of `deriveReviewers`. Today it runs in `hasValidUserData` before extraction, and under GraphQL it runs in the Step 2 mapper — either way the derivation input is already bot-free. Keep its tests where the exclusion lives.

### R3. Refactor: one fixture dialect for reviews and timeline comments (`testhelpers/mockgithubclient/mockgithubclient.go`, `internal/apiclients/githubclient/githubclient_test.go`, `cmd/pr-slack-reminder/main_test.go`, `cmd/pr-slack-reminder/snapshot_test.go`)

- `githubclient_test.go` carries package-local `NewReview` (`:96`), `NewComment` (`:111`) and `NewTimelineComment` (`:126`), all building go-github structs with signatures of their own. Without this step Step 5 swaps two fixture dialects onto GraphQL instead of one.
- Consolidate onto `testhelpers/mockgithubclient`, which keeps the go-github fixture format through the migration:

  ```go
  func NewReview(login, name, state string, userType ...string) *github.PullRequestReview
  func NewTimelineComment(login, name, body string, createdAt time.Time, userType ...string) *github.IssueComment
  ```

- `NewReview` takes the local signature, which **reorders** the shared helper's params as well as dropping `id` and `body`. Both are string triples, so a call site that only drops arguments still compiles and silently swaps `state` with `login`. Every `mockgithubclient.NewReview` call site is rewritten, not trimmed — in `main_test.go` (18 today) and in R0's `snapshot_test.go`, whose "reviewers present" case is written against the old signature:

  ```go
  mockgithubclient.NewReview(1, "APPROVED", "reviewer1", "", "LGTM 🙏🏻") // before
  mockgithubclient.NewReview("reviewer1", "", "APPROVED")                // after
  ```

- Dropping `id` and `body` is safe: derivation reads `State` and `User` only (`models.go:74,78,120`). `githubclient_test.go`'s 16 call sites already use this signature: their arguments stay as they are, only the `mockgithubclient.` qualifier is added, plus the import — the file doesn't import the package today.
- `NewTimelineComment` is new to `mockgithubclient`; there is no shared timeline helper today. `body` and `createdAt` are parameters because `findActiveSnooze` (`snooze.go:34`) reads both, so `main_test.go:658-665`'s inline `&github.IssueComment{…}` snooze fixture moves onto it, as does `snapshot_test.go:157-168`'s inline timeline comment — the only two inline ones left. That lengthens `githubclient_test.go`'s four call sites (`:448-451`), qualified the same way, whose local helper hardcodes the body — they pass their body and a zero time, and with no `/snooze` in the body the time is never read.
- `NewComment` is out of scope: both copies die in Step 5 with `CommentsByPRNumber`, since review comments stop being fetched. Consolidating them first is churn.

### 1. GraphQL transport, error classification and client wiring (`graphql.go`, `graphql_test.go`, `githubclient.go`, `testhelpers/mockgithubclient/mockgithubclient.go`)

- `graphqlClient` with `Do(ctx context.Context, query string, vars map[string]any, aliases []string, out any) ([]fieldError, error)`: POST `https://api.github.com/graphql`, body `{"query":..., "variables":...}`, `Authorization: Bearer <token>`, decode into `struct{ Data json.RawMessage; Errors []graphqlError }` and unmarshal `Data` into `out`. `aliases` is the caller's alias set, used for positive classification; field-level errors come back alongside the decoded data, so they need their own return value.
- `graphqlError` carries `Type`, `Message`, `Path []any` and `Extensions.Code` — a malformed query has no `type`, leaving `code` the only identifier.
- The injected seam is HTTP-level, below classification: `graphqlTransport { Post(ctx context.Context, body []byte) (status int, responseBody json.RawMessage, err error) }`. Retry lives in `Do`, so a mock drives it by returning a status. Mocks supply status and response JSON only, and every mock-driven test exercises the real classifier.
- `Do` classifies by `path` shape and returns typed errors — it never formats a user-facing message, since it cannot map an alias to a repository or a PR ref:
  - `["<alias>"]` → `repositoryError{alias, type, message}`.
  - `["<alias>","pullRequest"]` → `pullRequestError{alias, type, message}`.
  - Deeper, **and rooted at an alias in `aliases`** → `fieldError{alias, path, type, message}`, returned in the first result with a nil `error`.
  - Anything else — no `path`, a `path` rooted at `query`, or an alias not in `aliases` — → `queryError{type, code, message}`. So does a missing or null `data`.
  - Two rules keep a hard error from taking more with it than it owns, both needed by Step 4, where a `pullRequestError` doesn't fail its batch:
    - `Data` is decoded into `out` whenever it is present and decodable, **also when a hard error is returned** — one closed PR must not wipe the other 24 aliases in its batch. A `Data` that fails to decode is itself a hard error.
    - When several hard errors arrive, the **most severe** is returned, not the first: `queryError` > `repositoryError` > `pullRequestError`. GitHub decides the array order, so first-wins would let a repository-level failure hide behind a PR-level one. `fieldError`s accumulate either way and are returned alongside the hard error.
  - Non-200 status, or a body that doesn't parse as GraphQL → `transportError`. Retried per "Timeout budget", then wrapped and returned. The delay is a package `var retryDelay = 1 * time.Second`, not a `const` like the timeouts beside it (`githubclient.go:145-147`), so in-package `graphql_test.go` can zero it and its many error-path cases don't sleep. Only in-package tests can: `githubclient_test.go`, `fetchartifact_test.go` and `main_test.go` are all external test packages, leaving `main_test.go:308-314` — the one retrying case there — a 1s sleep.
- Wire the client in: `NewClient` gains a GraphQL-transport parameter alongside the existing REST services, and `GetAuthenticatedClient` builds one from `token` (not `tokenForState`, which stays artifact-only). That touches nine call sites. `GetAuthenticatedClient` (`githubclient.go:122`) passes the real transport; the other eight pass `mockgithubclient.UnusedGraphQLTransport`, which errors if called — `testhelpers/mockgithubclient/mockgithubclient.go:83`, `fetchartifact_test.go:298` (which gains the `mockgithubclient` import), and `githubclient_test.go:496, 620, 691, 752, 822, 937`. Nothing calls the transport until Step 5, so every existing test stays green.
- Tests: one table over `(status, response body) → (typed error, field errors, decoded data, attempts)`, with rows for each `path` shape; a nested `RESOURCE_LIMITS_EXCEEDED` returning data plus one `fieldError`; a `RATE_LIMITED` entry with no `path` failing rather than returning empty; a validation error rooted at `query` doing the same; a path rooted at an unknown alias and one with a numeric root doing the same; a null `data` and a missing `data` doing the same; a 502 and a 429 retried once then failing; a 401 and a 403 failing without a retry; a 200 with malformed JSON retried then failing. Rows for the two contract rules as well: a hard error alongside decoded data, a hard error alongside `fieldError`s, and several `fieldError`s in one response. Plus a severity table returning the `repositoryError` from a response that also carries a `pullRequestError`, in both array orders; a second table where the retry succeeds (5xx then 200, network error then 200); a context cancelled mid-retry-wait making one attempt only; and one test asserting the posted body carries the query and variables. Error *strings* are tested where they are produced, in Steps 3, 4 and 6.

### 2. Collaborator mapper (`graphqlmodels.go`, `graphqlmodels_test.go`)

- `graphqlmodels.go` holds the GraphQL response structs shared by both phases, plus:
- `collaboratorFromAuthorNode`, per "Collaborator mapping": null → zero value; `Bot` → login plus `[bot]`, no name; `User` → login and name; any other `__typename` → login only.
- `hasValidAuthorNode`, the predicate for keeping a review or comment node: false on a null author, an empty login, or `__typename == "Bot"`. It replaces `hasValidUserData` on the GraphQL path and runs before collaborator lists are built, so `deriveReviewers` and the `[bot]` suffix never meet.
- `timelineCommentFromNode`, from a GraphQL comment node to R2's `TimelineComment`, filling `Body`, `CreatedAt`, `Author` and `AuthorIsBot` (from `__typename`). Snooze reads the unfiltered list, so this mapping runs before `hasValidAuthorNode`, not after.
- Tests: one table over author nodes → `(Collaborator, hasValidAuthorNode)`, with rows for a `User` with a name, a `User` with `name: null` (falls back to login via `GetGitHubName()`), a `Bot` (login gains `[bot]`, name empty), a `Mannequin`, a null author and an empty login. Rows are raw JSON decoded into the node struct, so the same table pins `authorNode`'s `json` tags — `__typename` above all. Plus a second table over comment nodes → `TimelineComment`, covering the bot flag and a null author, pinning `commentNode`'s tags the same way. `pullRequestNode`'s tags are pinned first by Steps 3, 4 and 6, which decode through it.

### 3. Phase 1: list open PRs (`graphqlfetch.go`, `graphqlfetch_test.go`, `githubclient.go`)

- `graphqlfetch.go` gains the query builder and `listOpenPRs(ctx, repositories) ([]PRResult, error)`. One request, one aliased `repository` per configured repository:

  ```graphql
  query($owner0:String!,$name0:String!,$owner1:String!,$name1:String!){
    rateLimit { cost remaining limit }
    r0: repository(owner:$owner0,name:$name0){ ...prs }
    r1: repository(owner:$owner1,name:$name1){ ...prs }
  }
  fragment prs on Repository {
    pullRequests(states: OPEN, first: 100, orderBy: {field: CREATED_AT, direction: DESC}) {
      nodes {
        number title url isDraft createdAt updatedAt headRefOid
        author { login __typename ... on User { name } }
        labels(first: 100) { nodes { name } }
      }
    }
  }
  ```

- Aliases are generated into the query text (aliases can't be variables); owner and name go in as **variables**, never concatenated. The builder returns a `graphqlQuery` — text, variables, aliases and the alias → `models.Repository` map — which Steps 4 and 6 reuse. `listOpenPRs` uses that map to turn Step 1's `repositoryError` into `fmt.Errorf("repository %s/%s not found - check the repository name and permissions", ...)`, keeping today's wording and `main_test.go:306`'s expectation.
- Every other hard error becomes `fmt.Errorf("error fetching pull requests: %w", err)` — the Step 5 wording, produced here because `listOpenPRs` owns the call. The wrapped typed error prints its own text, so Step 5's `main_test.go:313` expectation is that prefix plus the `transportError` text, not the bare service error.
- A `fieldError` under a repository alias **fails the list**, named as `error fetching pull requests from %s/%s: %w`. Phase 1 has no per-PR unit to degrade to, so degrading would drop every PR of that repository silently; REST fails the whole call when one repository's list fails, so failing preserves today's behaviour. Same wording and same reason for `data.<alias>` being null or absent with no matching error.
- The response's repository aliases are generated, so they decode by name through an `UnmarshalJSON` that splits `rateLimit` from the aliases — `aliasedData[T]`, shared with phase 2 in Step 4. A decode failure stays a hard error, per Step 1's contract.
- `state` and `merged` are not selected. `states: OPEN` guarantees both, so the mapper sets `State: "open"`, `Merged: false` on the own type. `prparser.IsMerged()` and `IsClosedButNotMerged()` then behave as they do today for open PRs.
- `CREATED_AT DESC` is required, not cosmetic: REST's list defaults to `sort=created&direction=desc`, so a repository with more than 100 open PRs must yield the same newest-100 candidate set.
- `first: 100` matches today's `PerPage: 100`, and `MaxRepositories` (30) bounds the alias count.
- `labels(first: 100)`, not 20: REST returns a PR's labels unpaginated, so any nested cap is a behaviour change — a PR with more labels than the cap could slip past `ignored-labels` or fail a `labels` allow-list. 100 is free (measured: same 30-point cost as `first: 20`) and is GitHub's own maximum page size, so it shrinks the gap as far as one page can. The residual cap goes in `githubclient.spec.md` in Step 5, where it becomes the live behaviour.
- Timeout: `PullRequestListTimeout` (`githubclient.go:145`) rises from 10s to 30s and becomes this request's per-request timeout, covering both attempts. The worst-case shape measured 8-11 s wall time, which does not fit inside 10s with any margin.
- Log the `rateLimit` cost and remaining at debug level — it is the only visibility into budget consumption.
- Tests: one query-shape test asserting the generated text carries `states: OPEN`, `first: 100`, `orderBy: {field: CREATED_AT, direction: DESC}` and one aliased `repository` per configured repository, with owner/name only in the variables map. Ordering itself is GitHub's to honour and cannot be tested against canned JSON — `first: 100` means the client never sees more than 100 nodes per alias. Plus one table over canned responses: PRs mapped from both aliases, a `repositoryError` on `r0` producing the preserved error string, a `FORBIDDEN` producing the same, a null alias with a matching error not being read as an empty repository, a whole-query `RATE_LIMITED` failing without naming a repository. Plus one test pinning the node → `PullRequest` mapping field by field, `State: "open"` and `Merged: false` included.

### 4. Phase 2: enrich the capped set (`graphqlfetch.go`, `graphqlfetch_test.go`)

- `enrichPRs(ctx, []PRResult) ([]PR, error)` runs after filtering and capping, so it only ever covers PRs that will be shown. It returns PRs in input order, which is deterministic and matches `capPRsToLimit`'s, replacing today's arbitrary completion order. A test row asserts it.
- Batches of 25 aliases, one request each, batches issued concurrently at `DefaultGitHubAPIConcurrencyLimit` (3), each with `ReviewsFetchTimeout` (10s, unchanged — 25 aliases measured ~2 s):

  ```graphql
  p0: repository(owner:$owner0,name:$name0){ pullRequest(number:$num0){
    number
    commits(last: 1){ nodes { commit { oid committedDate } } }
    reviews(first: 100){ nodes { state author { login __typename ... on User { name } } } }
    comments(first: 100){ nodes { createdAt body author { login __typename ... on User { name } } } }
  } }
  ```

- `rateLimit` is selected here too, and the aliases are generated, so both phases decode through one generic `aliasedData[T]` and assemble their text through one `assembleQuery`. The selection above goes in as a `fragment enrichedPr on PullRequest`, matching phase 1's shape.
- `reviews(first: 100)` replaces today's 200-review cap (two pages of 100); `comments(first: 100)` matches today's 100 timeline comments exactly. Review comments are not fetched at all — their authors are always review authors (see "Reviewer derivation"), so `deriveReviewers`' review-comment input is nil until Step 8 drops it.
- Approvers are the review nodes with `state == "APPROVED"` (`PullRequestReviewState`, schema introspection 2026-08-09), feeding `deriveReviewers` in place of `isApprovingReview` (`models.go:120`). `PENDING` nodes are dropped entirely; every other state contributes a commenter, and `APPROVED` authors are then excluded from commenters by the approver filter, as today.
- `commits(last: 1)` is selected now, unused until 002. It costs one connection resolution per PR and removes the need to revisit this query later.
- Batch failure semantics:
  - All batches share one errgroup limited to `DefaultGitHubAPIConcurrencyLimit`. A `transportError`, `repositoryError` or `queryError` from any batch fails the group and cancels its siblings, wrapped as `error fetching reviews and comments: %w` — or `error fetching reviews and comments from %s/%s: %w` for a `repositoryError`, whose alias names a repository.
  - A `pullRequestError` does **not** fail the group here, unlike in `GetPRs` (Step 6). Enrichment has never been able to fail the call — `githubclient.spec.md` states a failed reviews/comments fetch for one PR does not fail `FindOpenPRs` — and a PR closed between phase 1 and phase 2 would otherwise abort a whole run. It degrades to the field-level path below.
  - `fieldError`s never fail the group. Each is attached to its alias's PR.
  - `data.<alias>.pullRequest == null` with no matching `errors` entry is treated as a `fieldError` covering the whole PR: it keeps its phase-1 scalars and loses reviews, comments and commits. The null is on `pullRequest`, not on the alias — a missing PR returns `{"data":{"p0":{"pullRequest":null}}}` with the alias object intact (verified live). `data.<alias> == null` is the repository-level shape, and only phase 1 produces it.
- A field-level error on one alias means that PR keeps its scalars and loses everything under the failed connection. Log it in the existing *"Unable to fetch reviews/comments for PR #%d"* form, naming the connection through the error's path, and carry on. The per-PR success line drops its review-comment count, which is no longer fetched: *"Found %d reviews and %d timeline comments for PR %v/%d"*.
- A field-level error on `comments` specifically also loses `/snooze`: `findActiveSnooze` sees an empty slice and a snoozed PR reappears in the reminder. Same failure mode as today's timeline-comment call failing, so it degrades rather than fails — the log line is the whole diagnosis.
- Tests: one table over canned phase-2 responses → `([]PR, error)`, with rows for a batch boundary at exactly 25 and at 26, a field-level error on one alias leaving the other 24 intact, a null `pullRequest` with no error behaving the same, a `pullRequestError` on one alias leaving the rest of the batch intact, a `transportError` on one batch failing the whole call, a `repositoryError` failing it while naming the repository, review states mapping onto approvers and commenters with `PENDING` dropped, and snooze parsing off `comments.nodes.body`/`createdAt`. Separately, a field-level error on `comments` letting a snoozed PR through, with the log line asserted.
- The batch-boundary rows also carry the input-order assertion, since two batches complete in an order the test does not control. The fake transport renders a response per posted body, keyed by the `numN` variables, so one fixture set drives both batches.
- Plus a query-shape test asserting whole calls — `commits(last: 1)`, `reviews(first: 100)`, `comments(first: 100)` — with their full selection sets and the author sub-selection, one aliased `pullRequest` per reference, and owner, name and number only in the variables map. A bare `first: 100` substring is satisfied by either connection and does not discriminate.

### 5. `FindOpenPRs` on GraphQL (`githubclient.go`, `githubclient_test.go`, `fetchartifact_test.go`, `testhelpers/mockgithubclient/mockgithubclient.go`, `cmd/pr-slack-reminder/main_test.go`)

- Same shape as today: `listOpenPRs` → `includePR` filter → `capPRsToLimit` → `enrichPRs` (which runs `deriveReviewers`) → `excludeSnoozedPRs`. Only the two fetch calls change.
- `capPRsToLimit`'s semantics are unchanged — newest by creation date, then update date; its body already changed in R1. `logFoundPRs` keeps its output.
- The errgroup fan-out over repositories disappears — phase 1 is one request. `DefaultGitHubAPIConcurrencyLimit` now bounds phase 2's batches instead. `TestFindOpenPRs_ConcurrencyLimit` (`githubclient_test.go:811-866`) loses its subject: `MaxPRsToFetch` 50 at 25 per batch is at most 2 batches, so a limit of 3 never binds. It becomes `TestFindOpenPRs_EnrichmentIsBatched`; its `repoCount` setup becomes a PR count spanning two batches, and its one assertion — `len(prs) != repoCount` (`:863`) — becomes that count.
- `testhelpers/mockgithubclient` changes in this step, not later, because `main_test.go` runs the whole pipeline. It gains a renderer, `NewGraphQLTransport(opts)`, that turns `PRs`, `PRsByRepo`, `ReviewsByPRNumber` and `TimelineCommentsByPRNumber` into phase-1 and phase-2 JSON. Field names, types and meaning are unchanged, so those cases need no rewriting. Step 1's `UnusedGraphQLTransport` stays beside it for `fetchartifact_test.go`, which must not reach GraphQL at all.
- The seam is `Post(ctx, body []byte)`, so the renderer unmarshals the posted body, picks the phase from the query text (`pullRequests(` → phase 1, `pullRequest(number:` → phase 2 and `GetPRs`) and binds each alias from `variables`: `rN` → `ownerN`/`nameN`, `pN` → `ownerN`/`nameN`/`numN`.
- Phase 2 and Step 6's `getPRsByRef` are indistinguishable that way — both are `pN` aliases over `pullRequest(number:`. No test sets both fixture sets, so the tiebreak is the fixtures: an alias's PR scalars come from `PRsByNumber` when it is set, and from `PRs`/`PRsByRepo` otherwise. Reviews and comments come from `ReviewsByPRNumber`/`TimelineCommentsByPRNumber` either way. Both phases render phase 1's scalar selection; `state`, `merged` and `labels` join the phase-2 node in Step 6, which is where they are first read.
- `CommentsByPRNumber` is removed from `MockGitHubClientOptions` (`mockgithubclient.go:25`). Review comments are not fetched, and no test sets the field. Both `NewComment` helpers go with it — `mockgithubclient.NewComment` (`:108`) has no other caller, and `githubclient_test.go`'s copy (`:111`) loses its only case below.
- R3's `NewReview` and `NewTimelineComment` keep their signatures and their go-github return types; only the renderer changes. `userType` renders as `__typename`, and an omitted or empty one renders `"User"` **everywhere the renderer emits an author node**, not just on reviews: an empty `__typename` falls into the mapper's "any other type" branch and drops the name. `getTestPR` (`main_test.go:41`) never sets `User.Type`, which would break `"by Jim"` (`main_test.go:482`), and `main_test.go:960-961` calls `NewReview` without one, expecting `"(✅ Reviewer One, Reviewer Two)"` at `:968`.
- `PRServiceError` and `IssueServiceError` become renderer-level failures in this step, not Step 8 — `main_test.go:682-683` is how every fetch-failure case is driven, so they cannot outlive the REST mocks by a step. Their names stay, and `ListPRsResponseStatus` (`mockgithubclient.go:23`) stays the selector: 404 renders a repository-level `NOT_FOUND` on the alias, any other non-200 renders a transport failure. Three `main_test.go` cases depend on them:
  - `main_test.go:301-307` (*"repo not found"*, `ListPRsResponseStatus: 404` + `PRServiceError`) renders a repository-level `NOT_FOUND` payload on that repository's alias. Its expectation is unchanged. A 404 marks only the aliases of repositories that have no PRs fixture, so one bad repository among good ones stays expressible — `githubclient_test.go`'s `TestFindOpenPRs_ErrorShortCircuits` needs exactly that.
  - `main_test.go:308-314` (*"unable to fetch PRs"*, status 500 + `PRServiceError`) renders a phase-1 transport failure carrying the error. Phase 1 is one request covering all repositories, so a transport failure is no longer per-repository and the message drops the repository: `"error fetching pull requests from %s/%s: %w"` becomes `"error fetching pull requests: %w"`, already in place from Step 3. The wrapped `transportError` prints its own text, so update the expectation at `main_test.go:313` to `error fetching pull requests: GraphQL request failed with status 500`, which `strings.Contains` matches against the full message. Note that `"error fetching pull requests from %s/%s: %w"` survives in Step 3 for the per-repository failures phase 1 still has — a field error on one alias, and a null or absent alias.
  - `main_test.go:339-346` (*"timeline comments fetch error is handled gracefully"*, `IssueServiceError`) renders a field-level error on every PR's `comments` connection in the phase-2 response. All 5 PRs are still returned and the expectations are unchanged.
- `githubclient_test.go` drives the same renderer rather than a fake of its own — its fixtures already go through R3's shared helpers, so the cases themselves are untouched and the phase-and-alias rule lives in one place. Its own REST service mocks shrink to stubs that error when called, which is what proves no REST fetch is left in `FindOpenPRs`; `multiRepoPRService`, `multiRepoIssuesService`, `selectivePRService` and `selectiveIssuesService` go with the fixtures they routed. `fetchartifact_test.go` joins the file list for that: it constructs two of those stubs.
- `TestFindOpenPRs_ReviewsPartialErrors`'s per-PR failure becomes an `ErrByPRNumber` entry, rendered as a PR-level error on that alias — Step 6's rendering, needed a step early. In `FindOpenPRs` it degrades per PR (Step 4), which is what the case asserts.
- The REST service mocks stay wired in `testhelpers/mockgithubclient` until Step 6, since `GetPRs` still uses them.
- Tests move from service mocks to rendered GraphQL JSON: the existing `TestFindOneOrNoPRs`, `TestFetchManyPRs`, `TestFindOpenPRs_MultipleRepositories`, `TestFindOpenPRs_ErrorShortCircuits`, `TestFindOpenPRs_ReviewsPartialErrors` and `TestFindOpenPRs_ConcurrencyLimit` keep their assertions; only their fixtures change.
- One exception, whose **assertion** changes: `githubclient_test.go:397-429` (*"PR with both review comments and standalone comments"*) expects `standalone-commenter`, a login present only in a review comment. That fixture models a state the API does not produce — every review comment has a parent review by the same author (see "Reviewer derivation") — so it is unbuildable once review comments aren't fetched. Rewrite it as a commenter reaching the list through an implicit `COMMENTED` review with an empty body, which is what GitHub actually returns for a bare diff comment. Its other two assertions ride on review comments as well and disappear with the row rather than move — both are already covered: bot commenter excluded by the timeline case (`githubclient_test.go:451`, expected at `:456`), approver-not-duplicated-as-commenter by `:289`.
- `githubclient.spec.md` gains Step 3's residual label cap here, where the GraphQL list becomes the live path: a PR's labels are capped at 100 (GitHub's maximum page size) while REST returned them unpaginated, so a PR with more labels could slip past `ignored-labels` or fail a `labels` allow-list.
- Acceptance: `make test` passes with no REST fetch call left in `FindOpenPRs`, and **every post-mode snapshot from R0 is unchanged** — run without `-update-snapshots`, and treat any diff as a regression to explain before continuing. Display names do not show up in this diff: the mock fixtures already carry `User.Name` on both paths, so the change described in "Display names" is only visible against the live API, in Step 7.

### 6. `GetPRs` on GraphQL (`githubclient.go`, `models.go`, `graphqlfetch.go`, `githubclient_test.go`, `graphqlfetch_test.go`, `testhelpers/mockgithubclient/mockgithubclient.go`, `cmd/pr-slack-reminder/main_test.go`)

- Update mode fetches specific refs, which may be closed or merged, so phase 1 doesn't apply. `getPRsByRef(ctx, []models.PullRequestRef) ([]PR, error)` uses phase 2's aliased shape with a fuller selection set, batched the same way (measured: 25 aliases, 1 point, ~2.5 s):

  ```graphql
  p0: repository(owner:$owner0,name:$name0){ pullRequest(number:$num0){
    number title url isDraft createdAt updatedAt headRefOid state merged
    author { login __typename ... on User { name } }
    labels(first: 100){ nodes { name } }
    commits(last: 1){ nodes { commit { oid committedDate } } }
    reviews(first: 100){ nodes { state author { login __typename ... on User { name } } } }
    comments(first: 100){ nodes { createdAt body author { login __typename ... on User { name } } } }
  } }
  ```

- The scalars, `author` and `labels` are the ones phase 1 selects; `state` and `merged` are added because nothing here guarantees them (all fields verified present on `PullRequest`, schema introspection 2026-08-09).
- Keeps its existing truncation to `MaxPRsToFetch` refs before fetching. `deriveReviewers` and `excludeSnoozedPRs` (`githubclient.go:233`) are unchanged, fed by the same `reviews`/`comments` nodes as phase 2.
- `getPRsByRef` returns unfiltered PRs; `GetPRs` keeps `getPRFilterFunc` → `capPRsToLimit` → `logFoundPRs` (`githubclient.go:222-227`), now after enrichment instead of before. No result change: the input is already ≤ `MaxPRsToFetch`. The three run over `PR` here and over `PRResult` in `FindOpenPRs`, so they go generic over an interface both satisfy — each carries a `*PullRequest` and a `models.Repository`.
- `getPRsByRef` keeps the alias → `models.PullRequestRef` map and turns Step 1's `pullRequestError` into today's wording, *"PR %s/%s/%d not found - check the path and permissions"*. The map joins `graphqlQuery` beside `repositoryByAlias`, and Step 4's alias builder is parameterized by fragment so both PR queries share it, which retypes `graphqlfetch_test.go`'s one `buildEnrichPRsQuery` call.
- The interface the three generic functions take lands in `models.go`, beside `PR` and `PRResult`, which implement it.
- `data.<alias>.pullRequest == null` with no matching `errors` entry is a hard error naming the ref — unlike phase 2 under `FindOpenPRs`, there are no phase-1 scalars to fall back to. A `pullRequestError` is hard here too, for the same reason.
- Timeouts: each batch uses `ReviewsFetchTimeout`. `PullRequestFetchTimeout` (5s) belonged to the per-PR REST `Get` and is deleted with it in Step 8.
- `state` maps `OPEN`/`CLOSED`/`MERGED` onto today's `GetState()`/`GetMerged()` pair: `GetState()` returns `"closed"` for both `CLOSED` and `MERGED`, and `GetMerged()` reads the selected `merged` field rather than deriving it from `state`, since both are selected and GitHub reports them together. This preserves `prparser.IsMerged()` and `IsClosedButNotMerged()` exactly. Verified field values: a merged PR reports `state: MERGED, merged: true`, an open one `state: OPEN, merged: false`.
- An unrecognised or absent `state` maps to `"open"`, not `"closed"`. A dropped selection then renders every PR as open instead of striking through the whole reminder, which is the recoverable direction. The selection set itself is pinned by a query-shape test, so the fallback should stay unreachable.
- The mock renderer gains the `PRsByNumber` and `ErrByPRNumber` paths, and `MakeMockGitHubClientGetter` stops wiring the REST service mocks.
- `ErrByPRNumber` renders as a PR-level GraphQL error on that PR's alias, carrying the configured message. `getPRsByRef` therefore reserves the *"not found - check the path and permissions"* wording for `NOT_FOUND` and wraps `message` for every other type, mirroring today's split between `fetchPR`'s 404 branch and its `%w` branch. That keeps `main_test.go:973-986` (*"update mode fails when fetching individual PR fails"*, asserting the substring `"failed to fetch PR"`) passing unchanged.
- Tests: one table over `state`/`merged` pairs, covering merged and closed-but-not-merged; plus an unknown PR number producing the preserved error string. The table asserts `GetState()`/`GetMerged()` at the client boundary rather than the rendered item, which `main_test.go`'s existing merged-and-closed case and R0's update-mode snapshot already pin.
- Two rules need tests of their own, since no fixture path reaches them: a query-shape test over the fuller selection set, which the mock renderer cannot stand in for because it ignores query text, and a canned response carrying a null `pullRequest` with no matching `errors` entry.
- End-to-end display-name gap to close here, since nothing covers it today: a fixture whose author has a login but `name: null` must render the login, while `main_test.go`'s existing `"🚨 2 days old by Jim"` expectations already pass unchanged — `getTestPR` (`main_test.go:71`) sets `User.Name` to the title-cased login, and `messagebuilder_test.go:215` sets `Collaborator{Login: ...}` directly.
- Acceptance: R0's update-mode snapshot is unchanged, on the same terms as Step 5. With Steps 5 and 6 both in, the whole snapshot set covers the REST-vs-GraphQL block diff, and it must be empty.

### 7. Verify against the live API with the existing workflows (no new files)

- Run the migration branch through `build.yml`, which is `workflow_dispatch` and listed for dispatch against any branch; the code that runs is the selected ref's own copy. It builds the branch and runs `.github/actions/e2e-tests`, which executes the built action three times under a GitHub App token: a basic run, a 3-repository run, and one with `filters`, `repository-filters` and `group-by-repository`.
- Rendering is already pinned by R0's snapshots, so this step is about what canned fixtures cannot show: query shape, ordering, real reviewer sets and real names. Compare the three posted Slack messages against a pre-migration dispatch of the same workflow: same PRs, same order, same reviewers, with names now rendering where logins did. PR state can move between the two dispatches, and the reviewer comparison only means anything if those repositories currently hold a PR with an approval and one with a comment — check that first, and diff a fresh pre-migration dispatch, not an old one.
- Then dispatch `pr-reminder.yml` on the branch with `build-first: true`, `run-mode: post`, and again with `update`. That proves `secrets.GITHUB_TOKEN` — not an App token — reaches GraphQL at all, and that update mode round-trips through the artifact. Its job grants only `contents: read` and `actions: read`, so it says nothing about `pull-requests: read` being sufficient or necessary: the reads succeed because this repository is public.
- Those two dispatches collide with production state. `FetchLatestArtifactByName` takes the newest artifact named `pr-slack-reminder-state` repo-wide, with no branch or run scoping, so the `post` dispatch becomes the repo-wide latest state and orphans the live reminder in `#github`.
- Any `schedule` (daily 09:00), `push: main`, `pull_request`, `pull_request_review` or `issue_comment` run landing between the two dispatches feeds `update` someone else's state, and the round-trip proves nothing. Run the two dispatches back-to-back, and re-run if a third run interleaves.
- Both runs log Step 3's `rateLimit` cost and remaining, so real per-run cost is read off the job log.
- Done, 2026-08-17. `pr-reminder.yml`: `post` (31999212548), then `update` (31999519609, 31999667977). Both phases cost 1 point each over one repository and 3 PRs. `update` loaded the `post` run's own artifact and edited the message. Budget logged 5,000, see "Observed budget".
- Run 31999641932 (`pull_request_review`) ran the pre-migration binary over the same 3 PRs 52 seconds before the second `update`. Same reviewers, same message: a REST vs GraphQL A/B on identical data.
- `build.yml` (32058202149): all three e2e runs green under the App token. Phase 1 costs 1 point per repository alias (1, then 3 over 3 repositories), phase 2 costs 1 per batch, budget 5,150.
- The rendered message closes "Display names" and "Bot logins": `✅ Joose` where REST rendered `hellej`, and `dependabot[bot]` keeps its suffix.
- Still unverified: timeline comments were 0 in every run, so the implicit review from a diff comment and the timeline commenter never ran live. The filters run excluded no PR, and nor did the pre-migration run of 2026-06-27 (28296590244), so the e2e fixtures hold nothing matching `ignore-reminder` or author `alice`.
- Run 32054716033 failed on GitHub's 2026-08-17 incident: GraphQL 503 on both attempts, one retry, error surfaced intact.

### 8. Delete the REST PR path (`githubclient.go`, `models.go`, `githubclient_test.go`, `fetchartifact_test.go`, `testhelpers/mockgithubclient/mockgithubclient.go`)

Only after Step 7 is green.

- Remove the REST enrichment path itself: `addReviewerInfoToPRs` (`githubclient.go:327`) and `FetchReviewsResult` with its `asPR()` and `printResult()` (`models.go:28-43,71`). Steps 5-6 leave them unreferenced but compiling, so nothing forces their removal. `PRResult` stays — phase 1 still produces it.
- Remove `GithubPullRequestsService`, `GithubIssuesService`, their `fetchPR`/`fetchOpenPRsForRepository`/`fetchPRReviews`/`fetchPRComments`/`fetchPRTimelineComments` helpers, `PullRequestFetchTimeout`, `reviewsMaximumPages`, `commentsPerPage`, `timelineCommentsPerPage`, `isApprovingReview`, the go-github → own-type mapper and the go-github → `TimelineComment` mapper from R1/R2.
- The go-github-generic collaborator helpers go together, or not at all: `hasValidUserData`, `getCollaborator` and `extractUniqueCollaborators` (`models.go:101-114`) are all constrained by `GitHubUserProvider` (`models.go:62-64`), so removing the interface without them leaves the package uncompilable. The GraphQL path has its own equivalents from Step 2. `newCollaboratorFromUser` and `isBot` go with them.
- Drop `deriveReviewers`' review-comment-authors parameter, and the review-comment position from R2's order row. R2's non-generic timeline pass goes too, and with it both fields it was the last reader of: `TimelineComment.AuthorIsBot` and `TimelineComment.Author`. The GraphQL path bot-filters with `hasValidAuthorNode` and sources commenters from `commentNode.Author`, so `TimelineComment` is left carrying what snooze reads. `snooze.go` was on this step's file list and needs no edit of its own: R2 already moved `findActiveSnooze` onto that type, and it reads `Body` and `CreatedAt` only. All three delete inputs no caller supplies any more, so `make test` covers them and Step 7 needs no re-dispatch.
- `NewClient` and `GetAuthenticatedClient` drop the two PR service parameters, keeping `actionsService`, the HTTP client and the GraphQL client. `GetAuthenticatedClient`'s second token argument still applies to artifacts only.
- That signature change touches four `NewClient` call sites, not the nine Step 1 wired: `githubclient.go`, `testhelpers/mockgithubclient/mockgithubclient.go`, `fetchartifact_test.go` and `githubclient_test.go`'s `newTestClient` helper, which Steps 5-6 consolidated that file's cases onto. Both test files join this step's file list for that reason alone.
- `testhelpers/mockgithubclient` drops the now-unreferenced `mockPullRequestService` and `mockIssueService` types, and `githubclient_test.go` drops its own copies. `fetchartifact_test.go` and `githubclient_test.go` still construct clients after Steps 5-6, so the parameters — and the mock types — survive until this step.
- go-github stays in `go.mod` for the artifact path, and remains the mock's fixture format.

### 9. Docs and spec sync (`githubclient.spec.md`, `prparser.spec.md`, `messagebuilder.spec.md`, `README.md`)

- `githubclient.spec.md`: two-phase GraphQL fetch, the new per-PR caps (100 reviews, 100 timeline comments, review comments no longer fetched, so the "100 PR comments" cap is gone), the 100-label page, the four error classes, the collaborator mapper's null/bot/other-type handling, bot login normalization, the raised `PullRequestListTimeout`, the transport retry, and that the artifact path remains REST. The "throttled to 3 concurrent calls" line now describes phase-2 batches.
- `prparser.spec.md` and `messagebuilder.spec.md`: author and reviewer rendering now shows display names where available, login otherwise.
- README: note in the permissions section that PR data is read via the GraphQL API and that the permissions are unchanged. `README.md:216`'s trailing comment — `pull-requests: read # listing/fetching PRs, reviews and review comments` — drops the review comments. `README.md:217` (`issues: read # reading PR comments (incl. /snooze comments)`) is unchanged: GraphQL reads the same timeline through `pullRequest.comments`, and no doc establishes that `pull-requests: read` alone covers it (see "Target shape"). Release notes call out that reviewer names, and unmapped author names, start rendering as GitHub display names instead of logins.
- No `action.yml` change, so `go run .github/scripts/check_inputs.go` is unaffected.
- Done, 2026-08-17. Beyond the listed items, the sync added bullets the migrated code made visible: per-call timeouts, the unpaginated first-100 open PRs per repository, an enrichment failure also losing that PR's snooze, `PENDING` reviews contributing no reviewer, an approval never being cancelled by a later `CHANGES_REQUESTED`, and the non-closed state fallback.
- A review pass over the written specs then fixed inaccuracies they inherited: the filters bullet claimed a term allow-list that `config.Filters` never had, `GetPRs`'s failure scoping ignored field errors, the zero-timestamp old-PR bullet ignored the threshold-0 guard, and `messagebuilder.spec.md` called truncation silent while the code logs it.
- Deviation: `prparser.spec.md` got no display-name bullet after all. It only attaches Slack IDs keyed by login; the name resolution belongs to `githubclient.spec.md` and the rendering to `messagebuilder.spec.md`, where both now sit.
- Deviation: `README.md`'s example screenshot (`docs/examples/example_1.png`) still shows reviewer logins, which now render as display names. Not re-recorded.

## Consequences

### Positive

- A run's PR-fetching cost stops scaling with PR count, on a rate-limit bucket the repository's other workflows don't touch.
- 002 is replanned against this once it lands: its Step 2 shrinks to a mapping change, since `commits(last: 1)` arrives with every enriched PR. Three of its figures assume a per-PR fan-out that no longer exists and must be restated there — the `prFetchTimeout` 60s → 90s raise and the "~22 sequential rounds at concurrency 3" behind it (002:229, 002:336, 002:412), and the "~150 calls today" baseline (002:410).
- Fewer moving parts in `githubclient`: no per-PR errgroup fan-out, no per-PR three-way inner group, no review pagination.
- R0 leaves behind a payload regression net the repository never had, useful to every later change to rendering — 002 included.
- R2 pins reviewer ordering, which no test asserted.
- The pipeline stops depending on go-github types, so the fetch layer can change again without touching four packages.

### Negative

- The fetch path's entire test suite is rewritten. Still the change's main cost, but no longer its main risk: R0's snapshots hold the rendered payload fixed across the rewrite, so what is left rests on the mock fixtures' fidelity rather than on the rewrite.
- Failures arrive as HTTP 200 with an `errors` array, so error handling is more code than checking a status code, and getting it wrong fails silently rather than loudly.
- The query-size ceiling that forces batching is undocumented, so the 25-alias batch size rests on measurement that could drift.
- Phase 1 is one request covering all repositories: its transport failures lose the repository from the error message, and one slow response (8-11 s measured across 30) delays all of them.
- `githubclient` runs two clients against two APIs, which is more surface than either alone.

### Neutral

- Names start rendering where logins always did: `hellej` becomes `Joose`.
- `PENDING` reviews stop contributing a commenter, where `asPR()` (`models.go:74,81`) counts every valid-user review today. A pending review is visible only to its own author's token, so the effect is confined to the token's own unsubmitted review.
- Per-PR caps change: reviews 200 → 100 (already truncated silently above that), review comments no longer fetched so their 100 cap is gone, timeline comments unchanged at 100, labels unpaginated → first 100 (a PR with more than 100 labels can now evade `ignored-labels`; 100 is GitHub's own page maximum).
- The head-commit date is the commit's own `committedDate`, not a push time. `pushedDate` is deprecated and returns null, and force-pushing an older commit reports that commit's date.
- `rateLimit` is selected in every query, costing nothing, and its value is only logged.
