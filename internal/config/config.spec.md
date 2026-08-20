# config

Parses and validates GitHub Action inputs into a `Config`. See [AGENTS.md](../../AGENTS.md) for the input-name/env-var convention and file-relationship rules.

## Behaviour

- `GetConfig()` reads every input, collects all parse errors and returns them together (not fail-fast), then runs business-rule validation
- Repository list: uses the `github-repositories` input if set, otherwise falls back to a single repo from the `GITHUB_REPOSITORY` environment
- `CurrentRepository` is always resolved from `GITHUB_REPOSITORY` directly, independent of the repository list — it's the repo state artifacts are fetched from
- `Filters` (`Authors`, `IgnoredAuthors`, `Labels`, `IgnoredLabels`, `IgnoredTerms`) are parsed from JSON, globally (`filters` input) and per repository (`repository-filters`, keyed by `owner/repo` or bare name); unknown JSON fields are rejected
- `Config.GetFiltersForRepository(repo)` looks up `repository-filters` by full path first, then bare name, else falls back to global filters
- `RunMode` is `"post"` (default) or `"update"`
- Validation enforces: a Slack channel (ID or name) is set; repository count ≤ `MaxRepositories` (30); no duplicate repositories; every `repository-filters` key matches exactly one configured repository; `pr-list-heading` is set unless `group-by-repository` is true; `state-artifact-name` is set when run mode is `"update"`
- `pr-tracker-canvas-link` is parsed into `PRTrackerCanvasID` (the `F…` ID from the link's `/docs/<TEAM_ID>/<CANVAS_ID>` path) and `PRTrackerCanvasURL` (the input as given, never rebuilt); `ContentInputs.CanvasURL` carries the same URL. Empty input leaves all three empty, which means the canvas feature is off
- A non-empty canvas link that has no `docs` path segment followed by an `F[A-Z0-9]+` segment is a parse error, joined with the other input parse errors
- `Config.Print()` logs the config as JSON with all tokens redacted
- Input getters (string/int/bool/list/mapping) treat an input as required or optional depending on which getter is called

## Doesn't Do

- Doesn't verify repositories, channels, labels, or usernames actually exist — validation is structural only
- Doesn't check that the canvas exists or that the bot can write to it — the link is only parsed for its shape
- `Filters` validation rejects `Authors`+`IgnoredAuthors` together and overlapping `Labels`/`IgnoredLabels`, but doesn't check filter values against anything real
- Integer/boolean inputs default to their zero value (`0`/`false`) when unset, not an error — only an unparsable non-empty value errors

## Oddities

- An input explicitly set to an empty string is treated differently from one left unset: the empty value is kept as-is (no default applied), while an unset input falls back to its default
- List/mapping inputs (`github-repositories`, `repository-filters`, etc.) split entries on `;` if the value contains one, otherwise on newline — so an input can't rely on both separators being literal data at once
- The canvas ID is the first `F[A-Z0-9]+` segment *after* `docs`, not a fixed path position, so a trailing title slug or a trailing slash parses fine. A team ID can't be mistaken for it (`T…`), nor can a slug (lowercase)
- A canvas link must be absolute with a host: a bare `F…` ID is rejected, since `PRTrackerCanvasURL` goes into the reminder message as a link
- `MaxRepositories` (30, enforced here) and `githubclient.MaxPRsToFetch` (50, enforced later in the pipeline) are independent limits with no cross-check between them
