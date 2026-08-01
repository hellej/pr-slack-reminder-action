# githubclient

Fetches and enriches PR data from GitHub. See [AGENTS.md](../../../AGENTS.md) for its place in the pipeline.

## Behaviour

- `Client.FindOpenPRs` lists open PRs across configured repositories; `Client.GetPRs` fetches specific PR refs (used by "update" run mode) — both return enriched `PR`s
- Results are filtered by `config.Filters` (author/label/term allow+block lists) and draft PRs are always excluded
- Result count is capped at `MaxPRsToFetch` (50); when over the cap, only the newest PRs (by creation time, then update time) are kept
- Each returned PR carries `ApprovedByUsers` (users with an approving review) and `CommentedByUsers` (reviewers/commenters who didn't approve, excluding the PR author); both are deduped by login and exclude bot accounts
- Review/comment data collected per PR is capped (200 reviews, 100 PR comments, 100 timeline comments) — a PR with more is silently truncated
- Repository/PR fetches are throttled to `DefaultGitHubAPIConcurrencyLimit` (3) concurrent GitHub API calls
- A PR with an active `/snooze [pr-reminder] for N (day|days|d)` comment (case-insensitive; most recent matching comment wins) is excluded from results until the snooze expires
- `FetchLatestArtifactByName` downloads the newest GitHub Actions artifact matching a given name and decodes a named JSON file from it into a caller-supplied target — used by [internal/state](../../state/state.spec.md) to load prior-run state

## Doesn't Do

- No retries or rate-limit backoff on API errors
- A failed repository/PR list fetch fails the whole `FindOpenPRs`/`GetPRs` call; a failed reviews/comments fetch for one PR does not — that PR is just returned without reviewer info

## Oddities

- `GetAuthenticatedClient` accepts a second, optional GitHub token used only for artifact list/download calls — needed because the "update" run mode may require `actions: read` on a token/scope different from the main PR-fetching token
- `GetPRs` silently truncates its input to the first `MaxPRsToFetch` refs if more are passed, before fetching anything
- Snooze detection reads raw timeline comments, not the bot-filtered set used for reviewer/commenter extraction — a bot-authored comment can still trigger a snooze
- A `/snooze ... for 0 days` comment matches and "succeeds" (expiration = comment creation time), but is already in the past so has no effect
- Snooze day counts above 365 are silently capped to 365 rather than rejected
