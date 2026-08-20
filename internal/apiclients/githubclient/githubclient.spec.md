# githubclient

Fetches and enriches PR data from GitHub. See [AGENTS.md](../../../AGENTS.md) for its place in the pipeline.

## Behaviour

- `Client.FindOpenPRs` lists open PRs across configured repositories, returning an `OpenPRsResult`; `Client.GetPRs` fetches specific PR refs (used by "update" run mode), returning enriched `PR`s
- Results are filtered by `config.Filters`: author and label allow+block lists, plus ignored terms matched as case-sensitive substrings of the title
- Draft PRs are excluded, unless `FindOpenPRs` is called with `PRFetchOptions{IncludeDrafts: true}`. `GetPRs` always excludes them
- Result count is capped at `MaxPRsToFetch` (50); when over the cap, only the newest PRs (by creation time, then update time) are kept
- With `IncludeDrafts` on, drafts are capped in their own bucket at `MaxDraftPRsToFetch` (15) by update time (newest kept), so they can never displace open PRs; open PRs keep the cap and sort above
- `OpenPRsResult` carries `OpenPRsCapped` and `DraftPRsCapped`, each true only when that bucket was trimmed to its cap
- Each returned PR carries `ApprovedByUsers` (users with an approving review) and `CommentedByUsers` (reviewers/commenters who didn't approve, excluding the PR author); both are deduped by login and exclude bot accounts
- Both PR-reading paths use the GraphQL API: `FindOpenPRs` lists every repository's open PRs in one request, then fetches reviews and comments for the capped set; `GetPRs` fetches the referenced PRs directly. Both fetch in batches of 25 PRs per request. `FetchLatestArtifactByName` is the only path that uses REST
- Batches run at most `defaultGitHubAPIConcurrencyLimit` (3) requests at a time
- Per PR 100 reviews and 100 timeline comments are read, oldest first by GitHub's default connection order since the query sets none; review comments are not read at all (their authors always have a review of their own)
- A collaborator carries a display name only when GitHub returns one for a user, and the login otherwise
- Per-call timeouts: `pullRequestListTimeout` (30s) for the listing request, `reviewsFetchTimeout` (10s) per batch; each covers that request's retry as well
- A PR with an active `/snooze [pr-reminder] for N (day|days|d)` comment (case-insensitive; most recent matching comment wins) is excluded from results until the snooze expires
- `FetchLatestArtifactByName` downloads the newest GitHub Actions artifact matching a given name and decodes a named JSON file from it into a caller-supplied target, used by [internal/state](../../state/state.spec.md) to load prior-run state

## Doesn't Do

- No retries on the REST artifact calls; a failed GraphQL request is retried once after a fixed 1s wait (5xx, 429, network errors, unparseable bodies), never on another 4xx and never on an HTTP 200 carrying an errors array; the rate-limit headers are never read
- Doesn't page past a repository's 100 newest open PRs
- In `FindOpenPRs`, a failure scoped to one PR (its `pullRequest`, or the `reviews` and `comments` below it) doesn't fail the call: that PR is returned without reviewer info; a query- or repository-scoped error, and any transport or decode failure, fails the call
- In `GetPRs`, a missing PR fails the call, as does any error scoped to the query, a repository or a `pullRequest`; an error below one of those (on `reviews` or `comments`) is only logged
- `FetchLatestArtifactByName` doesn't treat a missing artifact as empty state: no artifact matching the name is an error

## Oddities

- `GetAuthenticatedClient` accepts a second, optional GitHub token used only for artifact list/download calls, needed because the "update" run mode may require `actions: read` on a token/scope different from the main PR-fetching token
- `GetPRs` truncates its input to the first `MaxPRsToFetch` refs if more are passed, before fetching anything
- A GraphQL response carries HTTP 200 with an errors array; an error is scoped by its path to the whole query, a repository, a `pullRequest`, or a field below one, and when several arrive the most severe wins (query over repository over pull request)
- A PR left without reviewer info by a failed enrichment is also left without its snooze, so a snoozed PR reappears in the reminder
- A PR closed or merged between `FindOpenPRs`'s two phases is still returned as open, since enrichment doesn't re-read `state`; a PR deleted between them is returned as open and without reviewer info
- GraphQL returns bot logins without the `[bot]` suffix, so the client appends it; an author GitHub reports as null, such as a deleted account, yields a collaborator with no login at all
- A user with any `APPROVED` review counts as an approver, so a later `CHANGES_REQUESTED` review from the same user doesn't cancel it
- `PENDING` reviews contribute no reviewer or commenter, since such a review is visible only to its own author's token
- `GetPRs` renders any state other than `CLOSED` or `MERGED` as open, so an unexpected or missing state doesn't strike the PR through in the reminder
- A PR's first 100 labels are read (GitHub's maximum page size), so a PR with more labels can slip past `ignored-labels` or fail a `labels` allow-list
- Snooze detection reads raw timeline comments, not the bot-filtered set used for reviewer/commenter extraction, so a bot-authored comment can still trigger a snooze
- A snooze comment must be exactly the command, matched against the untrimmed body, so surrounding text, a second line, a trailing space or a trailing newline all stop the match
- A `/snooze ... for 0 days` comment matches and "succeeds" (expiration = comment creation time), but is already in the past so has no effect
- Snooze day counts above 365 are silently capped to 365 rather than rejected
- The JSON file inside the artifact zip is matched by base name, so the directory part of the given path is ignored
