# githubclient

Fetches and enriches PR data from GitHub. See [AGENTS.md](../../../AGENTS.md) for its place in the pipeline.

## Behaviour

- `Client.FindOpenPRs` lists open PRs across configured repositories, returning an `OpenPRsResult`; `Client.GetPRs` fetches specific PR refs (used by "update" run mode), returning enriched `PR`s; `Client.FindRecentlyMergedPRs` searches the PRs merged since a given moment, returning them newest merge first
- Results are filtered by `config.Filters`: author and label allow+block lists, plus ignored terms matched as case-sensitive substrings of the title
- Draft PRs are excluded, unless `FindOpenPRs` is called with `PRFetchOptions{IncludeDrafts: true}`. `GetPRs` always excludes them
- Result count is capped at `MaxPRsToFetch` (50); when over the cap, only the newest PRs (by creation time, then update time) are kept
- With `IncludeDrafts` on, drafts are capped in their own bucket at `MaxDraftPRsToFetch` (10) by update time (newest kept), so they can never displace open PRs; open PRs keep the cap and sort above
- `OpenPRsResult` carries `OpenPRsCapped` and `DraftPRsCapped`, each true only when that bucket was trimmed to its cap
- Each returned PR carries `ApprovedByUsers` (users with an approving review) and `CommentedByUsers` (reviewers/commenters who didn't approve, excluding the PR author); both are deduped by login and exclude bot accounts
- Each returned PR also carries three flags saying how close it is to a merge, read off the same enrichment request: `Conflicting` (`mergeable` is `CONFLICTING`; `UNKNOWN` is GitHub still computing it and reads as not conflicting), `HasThreadWaitingForAuthor` and `HasNonApprovingReview`
- `HasThreadWaitingForAuthor` is true when a thread is unresolved and its last comment is neither the PR author's nor a bot's, so the thread is waiting on the author rather than on a reviewer: the author replying hands the thread back to the reviewer, an author's note on their own diff never blocks, and a review bot's thread would otherwise hand every new PR back to its author before a person had looked. An unresolved thread with no comments, or one whose last commenter GitHub no longer reports, counts as blocking
- `HasNonApprovingReview` is true when a submitted review by someone other than the PR author is `COMMENTED` or `CHANGES_REQUESTED`. `DISMISSED` doesn't count, having been withdrawn, and neither does a bot's review, on the same reasoning as the threads: a review bot is not a person asking for changes. The author's own reviews are excluded because a bare inline comment on one's own diff arrives as a `COMMENTED` review
- The canvas reads a PR's last activity off `UpdatedAt`, the PR's own update time, selected by the open-PR listing and by `GetPRs`. A zero value means unknown activity
- `FindRecentlyMergedPRs` takes the window start as a parameter, never a clock read, so the merged list and the canvas footer share one "now". `RecentlyMergedWindow` (7 days) is the length the caller is expected to pass, `MaxMergedPRsToFetch` (6) the cap
- The merged search runs one aliased `search(type: ISSUE, first: 100)` per repository in a single request, with the query string `repo:<owner>/<name> is:pr is:merged merged:>=<YYYY-MM-DD>`. `search` filters on merge date server-side, which `Repository.pullRequests` cannot do: it orders by comments, creation or update time only, and any post-merge comment or label bumps the update time
- The cutoff qualifier is a day, so the search returns up to one extra day of merges; the exact `mergedAt >= mergedSince` cut, the repository filters, the merge ordering and the cap are all applied client-side. `search` cannot sort by merge date either
- Merged PRs are enriched like open ones, so a merged row names its approvers and commenters
- All three PR-reading paths use the GraphQL API: `FindOpenPRs` lists every repository's open PRs in one request, then fetches reviews and comments for the capped set; `GetPRs` fetches the referenced PRs directly; `FindRecentlyMergedPRs` searches in one request, then fetches reviews and comments for the capped set. All three fetch in batches of 25 PRs per request, so the merged cap (6) always fits one batch. `FetchLatestArtifactByName` is the only path that uses REST
- Batches run at most `defaultGitHubAPIConcurrencyLimit` (3) requests at a time
- Per PR 100 reviews, 100 timeline comments and 100 review threads are read (GitHub's maximum page size), oldest first by GitHub's default connection order since the query sets none; a thread carries its last comment's author only, and review comments are not read at all (their authors always have a review of their own)
- A collaborator carries a display name only when GitHub returns one for a user, and the login otherwise
- Per-call timeouts: `pullRequestListTimeout` (30s) for the open PR listing and for the merged PR search, `reviewsFetchTimeout` (10s) per batch; each covers that request's retry as well
- A PR with an active `/snooze [pr-reminder] for N (day|days|d)` comment (case-insensitive; most recent matching comment wins) is excluded from `FindOpenPRs` results until the snooze expires. `GetPRs` and `FindRecentlyMergedPRs` keep such a PR and only record the expiry on it: a snooze suppresses a request for attention, and the merged rows those two serve ask for nothing
- `FetchLatestArtifactByName` downloads the newest GitHub Actions artifact matching a given name and decodes a named JSON file from it into a caller-supplied target, used by [internal/state](../../state/state.spec.md) to load prior-run state

## Doesn't Do

- No retries on the REST artifact calls; a failed GraphQL request is retried once after a fixed 1s wait (5xx, 429, network errors, unparseable bodies), never on another 4xx and never on an HTTP 200 carrying an errors array; the rate-limit headers are never read
- Doesn't page past a repository's 100 newest open PRs, nor past the first 100 merges a repository has inside the window. A repository over that gets a truncated merged list and a log line saying so, read off `issueCount`
- Doesn't report a merged search per repository: any error fails the whole merged search, the only part of the merged fetch that can fail it
- In `FindOpenPRs` and `FindRecentlyMergedPRs`, a failure scoped to one PR (its `pullRequest`, or the `reviews`, `comments` and `reviewThreads` below it) doesn't fail the call: that PR is returned without reviewer info. A query- or repository-scoped error, and any transport or decode failure, fails `FindOpenPRs` and drops `FindRecentlyMergedPRs` back to unenriched merged rows
- In `GetPRs`, a missing PR fails the call, as does any error scoped to the query, a repository or a `pullRequest`; an error below one of those (on `reviews`, `comments` or `reviewThreads`) is only logged
- Doesn't read a review thread's `isOutdated`: an unresolved thread blocks whether or not newer commits moved it. Doesn't read `reviewDecision` either, which is null on an approved PR in a repository without required-reviewer rules and so cannot report approval
- `FetchLatestArtifactByName` doesn't treat a missing artifact as empty state: no artifact matching the name is an error wrapping `ErrNoArtifactFound`, so a caller can tell it from a failed load. GitHub reports the missing artifact only as an empty list

## Oddities

- `search` reports a nonexistent repository, one the token cannot read and an empty window identically, as an empty result with no error, so a misspelled repository is silent on the merged path. Not silent overall: every canvas refresh lists open PRs through `repository(owner:,name:)` first, and that fails the run on an unreadable repository
- The merged search selects fewer fields than the other two paths, so a merged PR carries a zero `UpdatedAt` and `Draft: false` on the shared `PullRequest` struct. Nothing reads them: the merged list is ordered on `MergedAt`
- `MergedAt` is filled on the `GetPRs` path too, which is what dates a merged row in the reminder message. `Merged` is what puts the PR in that section
- GitHub's PR search index lags a merge by seconds, so a merge from the last moments before the run can be missing from the list
- `GetAuthenticatedClient` accepts a second, optional GitHub token used only for artifact list/download calls, needed because the "update" run mode may require `actions: read` on a token/scope different from the main PR-fetching token
- `GetPRs` truncates its input to the first `MaxPRsToFetch` refs if more are passed, before fetching anything
- A GraphQL response carries HTTP 200 with an errors array; an error is scoped by its path to the whole query, a repository, a `pullRequest`, or a field below one, and when several arrive the most severe wins (query over repository over pull request)
- A PR left without reviewer info by a failed enrichment is also left without its snooze, so a snoozed PR reappears in the reminder
- A PR with over 100 review threads is judged on the first 100, so an unresolved thread past that is missed and the PR can read as nobody's turn
- A review a person posts through a bot integration counts for neither flag, so real feedback delivered by a bot leaves the PR looking untouched
- A bot is an account GitHub types as `Bot`, which means a GitHub App. A CI or service account posting under a user login is a person to both flags, so its review comments do put a PR under its author's turn
- A PR left without reviewer info by a failed enrichment gets `Conflicting`, `HasThreadWaitingForAuthor` and `HasNonApprovingReview` all false, the same values a PR nobody has touched has
- A PR closed or merged between `FindOpenPRs`'s two phases is still returned as open, since enrichment doesn't re-read `state`; a PR deleted between them is returned as open and without reviewer info
- GraphQL returns bot logins without the `[bot]` suffix, so the client appends it; an author GitHub reports as null, such as a deleted account, yields a collaborator with no login at all
- A user with any `APPROVED` review counts as an approver, so a later `CHANGES_REQUESTED` review from the same user doesn't cancel it
- `PENDING` reviews contribute no reviewer or commenter, since such a review is visible only to its own author's token
- `GetPRs` renders any state other than `CLOSED` or `MERGED` as open, so an unexpected or missing state leaves the PR in the reminder's open sections
- A PR's first 100 labels are read (GitHub's maximum page size), so a PR with more labels can slip past `ignored-labels` or fail a `labels` allow-list
- Snooze detection reads raw timeline comments, not the bot-filtered set used for reviewer/commenter extraction, so a bot-authored comment can still trigger a snooze
- A snooze comment must be exactly the command, matched against the untrimmed body, so surrounding text, a second line, a trailing space or a trailing newline all stop the match
- A `/snooze ... for 0 days` comment matches and "succeeds" (expiration = comment creation time), but is already in the past so has no effect
- Snooze day counts above 365 are silently capped to 365 rather than rejected
- `UpdatedAt` overstates freshness as an activity time: a comment or a label change moves it without a push. It bounds the last push from above, so a PR is never wrongly dropped as inactive
- The JSON file inside the artifact zip is matched by base name, so the directory part of the given path is ignored
