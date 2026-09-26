# Surface warnings and errors on the run page

date: 2026-09-26
status: draft

## Goals

- A run that fails shows its error as an annotation on the GitHub Actions run page, not only in
  the job log
- A run shows a warning annotation there for each problem it carries on past:
  - `post` could not load an existing state, so it did not mark the previous message stale
  - `post` found the previous message gone or not editable
  - the recently merged PR fetch failed
  - `update` could not fetch the tracked PRs its own fetches left unresolved
  - `update` failed to delete its message
- No warning where nothing went wrong: a missing state artifact stays a plain log line, since a
  first run and a setup that only runs `post` have none
- The work lands on its own branch and PR from `main`, after PR #67 merges: it builds on that
  PR's state load in `post`

Serves § **Purpose**: today a silent degradation, such as a 403 on the state artifact, is visible
only to someone reading the job log, so the reminder quietly loses features. Sized against
§ **Reference Deployment**: the daily `post` and a handful of `update` runs a day each add at most
a few annotations, and only when something went wrong.

### Non-goals

- No change to what fails a run: a warning never turns it red, an error still does
- No new action input
- No annotation for expected skips: no state artifact yet, no stored message in an older state,
  the previous message in another channel, `update` keeping its message when a fetch failed (the
  fetch's own warning covers it)
- No `file`, `line` or `col` parameters: the problems are in the run, not in a file

## Target shape

- An annotation is one line the Go binary writes to stdout, the stream the docs name for
  workflow commands ([Workflow commands](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-commands)):

  ```
  ::warning title=PR Slack reminder::Not marking the previous message stale, its state did not load: failed to list artifacts: … 403 Forbidden
  ::error title=PR Slack reminder::PR tracker canvas refresh failed: …%0Afailed to update Slack message: …
  ```

  - The message escapes `%`, CR and LF as `%25`, `%0D` and `%0A`, so a multiline
    `errors.Join` error stays one annotation. See docs/third-party-facts.md § A workflow
    command's message escapes `%`, CR and LF as `%25`, `%0D` and `%0A`
  - The title `PR Slack reminder` names the source in a job with several steps. It escapes `:` and
    `,` like any property, and holds neither
  - The line replaces the log line it stands for: the runner prints it in the log as
    `##[warning]…`, so the log keeps the text once
- GitHub caps annotations at 10 warnings and 10 errors per step, and truncates a message past
  4096 characters. See docs/third-party-facts.md § `::warning::` and `::error::` annotations cap
  at 10 per type per step and 4096 characters each. A run emits at most ~5 warnings and 1 error
- `githubclient.FetchLatestArtifactByName` returns a sentinel for "no artifact with that name",
  so `post` can tell a missing state from a failed load

### Action inputs and permissions

- No input added or changed. No new permission

## Breaking change

Minor: a new visible behaviour on the run page, with no input, state or exit code change. No
persisted data changes, so upgrading needs nothing.

## Summary

- 1: Write annotations, and report a failing run's error as one
- 2: Tell a missing state artifact apart from a failed load
- 3: Warn on the problems a run carries on past

## Steps

### 1. Write annotations, and report a failing run's error as one

- New file in `cmd/pr-slack-reminder` with the annotation writer: `warning` and `error`
  lines as in Target shape, message escaped with the toolkit's `escapeData` rules
- `main.go`: a failing `Run` prints its error as an `::error::` annotation and exits 1, in place
  of `log.Fatalf`
- Tests pin the escaping (`%`, CR, LF, and a joined multiline error) and the line format
- Update `run.spec.md`

### 2. Tell a missing state artifact apart from a failed load

- `githubclient`: new sentinel `ErrNoArtifactFound`, returned wrapped by
  `FetchLatestArtifactByName` when the list has no artifact with the name. `state.Load` keeps it
  reachable with `errors.Is`
- Every other failure keeps its current error: list or download failure, a missing JSON file in
  the zip, invalid JSON
- Update `githubclient.spec.md`, whose Doesn't Do already says a missing artifact is an error

### 3. Warn on the problems a run carries on past

- `run.go`, each as a warning annotation in place of its log line:
  - `markPreviousMessageStale`: a state load failure other than `ErrNoArtifactFound`, which stays
    a plain log line
  - `markPreviousMessageStale`: `ErrMessageNotEditable`
  - the recently merged PR fetch failure
  - `runUpdateMode`: the tracked PR fetch failure, and a failed delete
- The other skips keep their plain log lines: no stored message, another channel, keeping the
  message after a failed fetch
- Integration tests in `main_test.go` capture the run's stdout and pin one warning per case,
  including none for a missing artifact
- Update `run.spec.md`
- Done also means a live check: dispatch `pr-reminder.yml` with `build-first=true` and a
  `github-token-for-state` lacking `actions: read`, or a state artifact name with a corrupt JSON
  file. The run page shows the warning, and the run stays green

## Consequences

### Positive

- A lost feature, such as marking stale on a 403, shows on the run page instead of passing
  silently
- A red run says why on the run page

### Negative

None

### Caveats

- Unverified: whether the annotation UI shows an escaped newline as a line break
- Past 10 warnings in one step, the rest print to the log only. A run emits at most ~5
- The runner also parses stderr, but only stdout is the documented stream, so annotations go to
  stdout while the rest of the log stays on stderr. The two streams can interleave out of order
  in the job log

### Neutral

- The job log shows each annotated line tagged `##[warning]` or `##[error]` instead of plain
