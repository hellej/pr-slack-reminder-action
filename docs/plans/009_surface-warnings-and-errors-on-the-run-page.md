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
- New `logWarning(message)` and `logError(message)` in `cmd/pr-slack-reminder` write one
  annotation line each through `log`:

  ```
  ::warning title=PR Slack Reminder::Not marking the previous message stale, its state did not load: failed to list artifacts: … 403 Forbidden
  ::error title=PR Slack Reminder::failed to update Slack message: …%0APR tracker canvas refresh failed: …
  ```

  - The title matches the action's `name:` in `action.yml`
  - The message escapes `%`, CR and LF as `%25`, `%0D` and `%0A`, so a multiline `errors.Join`
    error stays one annotation. See docs/third-party-facts.md § A workflow command's message
    escapes `%`, CR and LF as `%25`, `%0D` and `%0A`
  - Each line replaces the log line it stands for. The runner prints it in the log as
    `##[warning]…` or `##[error]…`
- GitHub caps annotations at 10 warnings and 10 errors per step, and truncates a message past
  4096 characters. See docs/third-party-facts.md § `::warning::` and `::error::` annotations cap
  at 10 per type per step and 4096 characters each. A run emits at most ~5 warnings and 1 error
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
- Tests pin the escaping and line format
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
- Integration tests in `main_test.go` capture the log and pin one warning per case, and none for
  a missing artifact
- Update `run.spec.md`
- Done also means a live check on the implementation branch: temporarily remove `actions: read`
  from the `reminder` job's `permissions` in `pr-reminder.yml`, dispatch with `run-mode=post` and
  `build-first=true`. The run page shows the state load warning and the run stays green. Restore
  the permission before merge

## Consequences

### Positive

- A lost feature, such as marking stale on a 403, shows on the run page instead of passing
  silently
- A red run says why on the run page

### Negative

None

### Caveats

- Unverified: whether the annotation UI shows an escaped newline as a line break
- If a workspace's message edit window applies to bot edits and is shorter than the time between
  posts, every `post` warns about the not-editable previous message. Unverified whether such
  windows apply to bots: see docs/third-party-facts.md § `chat.update` errors: `message_not_found`,
  `cant_update_message`, and `edit_window_closed` from the workspace's edit settings

### Neutral

- All log output moves from stderr to stdout. Nothing in the repo reads the streams separately
