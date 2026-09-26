# Surface warnings and errors on the run page

date: 2026-09-26
status: draft

## Goals

- A run that fails shows its error as an annotation on the GitHub Actions run page, not only in
  the job log
- A run shows a warning annotation there for each of these problems:
  - `post` could not load an existing state, so it did not mark the previous message stale
  - `post` found the previous message gone or not editable
  - the recently merged PR fetch failed, in every setup. With the canvas on, the run also fails
    on it, so the error annotation repeats it
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

- No change to what fails a run
- No new action input
- No annotation for expected skips: no state artifact yet, no stored message in an older state,
  the previous message in another channel, `update` keeping its message when a fetch failed (the
  fetch's own warning covers it)
- No `file`, `line` or `col` parameters: the problems are in the run, not in a file

## Target shape

- `main` sends all log output to stdout with `log.SetOutput(os.Stdout)`, the stream the docs
  name for workflow commands
  ([Workflow commands](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-commands)).
  Log lines and annotations then share one ordered stream
- New `logWarning(message)` and `logError(err)` in `cmd/pr-slack-reminder` write annotation
  lines through `log`:

  ```
  ::warning title=PR Slack Reminder::Not marking the previous message stale, its state did not load: failed to list artifacts: … 403 Forbidden
  ::error title=PR Slack Reminder::failed to update Slack message: …
  ::error title=PR Slack Reminder::PR tracker canvas refresh failed: …
  ```

  - The title matches the action's `name:` in `action.yml`
  - `logError` writes one annotation per part of an `errors.Join` error, found through
    `Unwrap() []error` recursively, since the canvas error can itself be a join. An escaped
    newline would show as a space on the run page, running the parts together. See
    docs/third-party-facts.md § An escaped newline is a line break in the job log and the API,
    but a space on the run's summary page
  - It splits an error only when its text is its parts' texts on separate lines, as
    `errors.Join` builds it. `fmt.Errorf` with several `%w` also has `Unwrap() []error`, such as
    slackclient's not-editable update error, and splitting it would drop its own text
  - The canvas error can be a join of the write failure and the merged fetch failure. One
    `fmt.Errorf("PR tracker canvas refresh failed: %w", …)` around it has `Unwrap() error`, so
    `logError` would never split it. `Run` prefixes each part of the canvas error with
    `PR tracker canvas refresh failed: ` instead and keeps them a join, splitting them with the
    helper `logError` uses. A single canvas error still reads
    `PR tracker canvas refresh failed: <err>`
  - The message escapes `%`, CR and LF as `%25`, `%0D` and `%0A`, so a part with a newline stays
    one annotation. See docs/third-party-facts.md § A workflow command's message escapes `%`, CR
    and LF as `%25`, `%0D` and `%0A`
  - Each line replaces the log line it stands for. The runner prints it in the log as
    `##[warning]…` or `##[error]…`
- GitHub caps annotations at 10 warnings and 10 errors per step, and truncates a message past
  4096 characters. See docs/third-party-facts.md § `::warning::` and `::error::` annotations cap
  at 10 per type per step and 4096 characters each. A run emits at most ~5 warnings and 4 errors
- New sentinel `githubclient.ErrNoArtifactFound`, so `post` can tell a missing state from a failed
  load. A missing artifact shows only as an empty list: see docs/third-party-facts.md § GitHub's
  "List artifacts" with a `name` filter returns 200 and an empty list when nothing matches

### Action inputs and permissions

- No input added or changed. No new permission

## Breaking change

Minor: a new visible behaviour on the run page, with no input, state or exit code change. No
persisted data changes, so upgrading needs nothing.

## Summary

- 1: Write annotations, and report a failing run's error as one
- 2: Tell a missing state artifact apart from a failed load
- 3: Warn on the listed problems

## Steps

### 1. Write annotations, and report a failing run's error as one

- `main.go`: `log.SetOutput(os.Stdout)`; a failing `Run` goes through `logError` and exits 1, in
  place of `log.Fatalf`
- New file `cmd/pr-slack-reminder/annotations.go` with `logWarning` and `logError`
- `run.go`: `Run` prefixes each part of the canvas error, as the Target shape says
- New `testhelpers.CaptureLog` and `testhelpers.LogLinesStartingWith` capture the `log` output in
  tests, restoring its output and flags after
- Tests in `annotations_test.go` pin the escaping, the line format, one line per part of a
  nested join, nil parts skipped, one line for a `fmt.Errorf` with two `%w`, and one prefixed
  line per failed canvas part
- Update `run.spec.md`

### 2. Tell a missing state artifact apart from a failed load

- `githubclient.FetchLatestArtifactByName` returns `ErrNoArtifactFound`, wrapped, when the list
  holds no artifact with the name. No change to `state`: `Load` already returns the fetch error
  as it is
- Every other failure keeps its current error: list or download failure, a missing JSON file in
  the zip, invalid JSON
- Update `githubclient.spec.md`, whose Doesn't Do already says a missing artifact is an error

### 3. Warn on the listed problems

- `run.go`, each through `logWarning` in place of its log line:
  - `markPreviousMessageStale`: a state load failure other than `ErrNoArtifactFound`, which stays
    a plain log line; and `ErrMessageNotEditable`
  - `Run`: the recently merged PR fetch failure
  - `runUpdateMode`: the tracked PR fetch failure, and a failed delete
- `markPreviousMessageStale` logs a missing artifact as `Not marking the previous message stale,
  there is no previous state: …`
- The delete warning reads `Keeping the Slack message with nothing left to show, its delete
  failed: …`, in place of the log line `Warning: failed to delete message: …`
- Integration tests in `main_test.go` capture the log and pin each warning line, that it shows
  once in the log, and none for a missing artifact or the other expected skips
- Update `run.spec.md`
- Done also means a live check on the implementation branch: temporarily pass an invalid
  `github-token-for-state` in `pr-reminder.yml`, dispatch with `run-mode=post` and
  `build-first=true`. The run page shows the state load warning and the run stays green. Revert
  before merge. Removing `actions: read` does not work here: see docs/third-party-facts.md
  § A public repository's artifacts list and download without `actions: read`
  - Done: run 36233911019 showed the warning for a 401 on the artifact list, and stayed green

## Consequences

### Positive

- A lost feature, such as marking stale on a 403, shows on the run page instead of passing
  silently
- A red run says why on the run page

### Negative

None

### Caveats

- If a workspace's message edit window applies to bot edits and is shorter than the time between
  posts, every `post` warns about the not-editable previous message. Unverified whether such
  windows apply to bots: see docs/third-party-facts.md § `chat.update` errors: `message_not_found`,
  `cant_update_message`, and `edit_window_closed` from the workspace's edit settings

### Neutral

- All log output moves from stderr to stdout. Nothing in the repo reads the streams separately
