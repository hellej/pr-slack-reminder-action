# Reduce PR tracker canvas writes

date: 2026-09-05
status: draft

## Requirements

Slack's canvas client mis-merges full-document replaces that land while someone has the canvas
open, duplicating headings and PR rows. Client-side only: a reload shows the real content.
Measured 2026-09-05, see the "Rapid `canvases.edit` replaces duplicate headings and rows in an open
canvas" entry in [docs/third-party-facts.md](../third-party-facts.md).

Nothing can stop Slack mis-merging, so the goal is to make the writes that trigger it rare.

Goals:

- Stop two runs writing the canvas back to back
- Stop rewriting the canvas when the markdown would be the same
- Tell README readers a duplicated canvas is a rendering artifact

Non-goals:

- No new inputs, and no way to switch either behaviour off
- No retry or read-back verification. Slack has no method that returns a canvas's markdown
- No change to the reminder message, or to what the canvas renders

## Target shape

`action.yml`, token permissions and Slack scopes are untouched.

`State` gains `CanvasContentHash`. A run that renders matching markdown skips
`ReplaceCanvasContent`.

The hash comes from an unexported `canvasContentHash` in `cmd/pr-slack-reminder/canvas.go`: copy
the built `Content`, zero `GeneratedAt` on the copy, render through `canvasbuilder.BuildMarkdown`,
SHA-256 the result. Two constraints on it:

- Zero `GeneratedAt` on a copy of the built `Content`, never by re-running
  `canvascontent.GetContent` with a zeroed value. There it is also the "now" that
  `isActiveEnoughForCanvas` prunes drafts against, so a zero keeps every dead draft
- Row text still moves with the wall clock, since `prparser` reads `time.Since` directly. The hash
  therefore moves as PRs age. That is fine: the target is runs seconds apart, which render
  identical text

State gets one writer. `runPostMode` writes it today before the canvas refresh produces a hash, so
both modes now hand their state to `Run`, which writes it once, after the refresh.

Update runs start uploading a state artifact. Today they upload nothing, because
`FetchLatestArtifactByName` decodes the zip without ever writing `stateFilePath`. Without this the
hash would only refresh on the daily `post` run.

The schedule gets its own concurrency group so nothing can cancel it. Everything else shares one
and serializes.

The footer now says when the canvas was last written, not when the action last ran. Wording
unchanged.

**State file compatibility.** `FetchLatestArtifactByName` decodes with a plain `json.Decoder`;
`DisallowUnknownFields` is only used for the `filters` input. An old artifact decodes an empty
hash, which reads as "write the canvas"; an older action version ignores the unknown key.
`CurrentSchemaVersion` stays 1, since nothing reads it on load.

## Breaking change

Minor. No input or config change, and the canvas renders the same content. The two observable
changes, a footer that stops moving and a state artifact from update runs, break no configuration.

## Steps

- **R1** Hand `post` mode's state back to `Run`
- **1** Add a concurrency group to the reminder workflow
- **2** Hand `update` mode's state back to `Run`, and save it
- **3** Skip the canvas write when the markdown would be unchanged
- **4** Update the README's canvas section

## Step R1: Hand `post` mode's state back to `Run`

Touches: `cmd/pr-slack-reminder/run.go`, `internal/state/state.go`, `internal/state/state.spec.md`,
`cmd/pr-slack-reminder/main_test.go`.

- Split `SavePostState` into `state.NewPostState(parsedPRs, messageInfo) State` plus the existing
  `state.Save`. `NewPostState` is the only place that stamps `SchemaVersion` and `CreatedAt`
- Move the `failed to save state: %w` wrap and the `Saved state to %s with %d PRs` log to `Run`
- `runPostMode` becomes `(*githubclient.OpenPRsResult, *state.State, error)`, returning a nil state
  where it writes none today: the open-PR fetch failure, the early return with no PRs and no
  `no-prs-message`, and a failed `SendMessage`
- `Run` writes a non-nil state after the canvas refresh, joining any error into its existing
  `errors.Join`

Three accepted deltas: `sentMessageHandler` now runs before the state write; state lands after the
canvas refresh, so a run cancelled inside the refresh loses it; a write error joins instead of
short-circuiting.

Tests: existing post-mode state assertions still hold, and a failed send still writes no state.

## Step 1: Add a concurrency group to the reminder workflow

Touches: `.github/workflows/pr-reminder.yml`.

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.event_name == 'schedule' && 'scheduled' || 'other' }}
  cancel-in-progress: false
```

A queued run cancels a pending one in the same group, so a shared group would let a `push` run
cancel the pending scheduled run, silently costing the day's reminder and state artifact. Hence the
split.

Keying on the event, not the run mode, puts a `workflow_dispatch` with `run-mode: post` in the
shared group. Deliberate: a manual run is visible and re-runnable, and sharing serializes it
against update runs. `cancel-in-progress: false` because `true` would kill an in-flight post run
mid-send.

No test. Done means two runs triggered seconds apart show one `in_progress` and one `queued`.

## Step 2: Hand `update` mode's state back to `Run`, and save it

Touches: `cmd/pr-slack-reminder/run.go`, `internal/state/state.spec.md`,
`cmd/pr-slack-reminder/main_test.go`. Nothing in `state.go`.

- `runUpdateMode` becomes `(*state.State, error)`, returning the loaded state on every path that
  has one, including both early returns: no PRs in the loaded state, and all PRs gone so the
  message was deleted. Nil when `state.Load` itself failed
- Write it back as loaded: `SlackMessage`, `PullRequests`, `CreatedAt` and `SchemaVersion`
  unchanged, only the hash set by Step 3. Refreshing the PR list would change which PRs update mode
  tracks

Saving on the delete path leaves state pointing at a deleted message, which is what already happens
today via the pre-delete artifact.

Tests:

- Saved `SlackMessage` and `PullRequests` match what was loaded. Use a fixture where the loaded and
  fetched sets differ, e.g. load {1, 2} with PR 2 dropped by `ignored-authors`. `TestScenariosUpdateMode`'s
  current fixtures use the same set for both, so they cannot catch an implementation saving this
  run's PRs
- A run that deletes the message, and a run whose state has no PRs, both still save
- A run whose state load fails saves nothing

## Step 3: Skip the canvas write when the markdown would be unchanged

Touches: `internal/state/state.go`, `internal/state/state.spec.md`,
`cmd/pr-slack-reminder/canvas.go`, `cmd/pr-slack-reminder/run.go`,
`cmd/pr-slack-reminder/canvas_internal_test.go` (new), `cmd/pr-slack-reminder/canvas_test.go`.

- Add `CanvasContentHash` to `state.State`, JSON key `canvasContentHash`
- Add `canvasContentHash`, per Target shape
- `refreshPRTrackerCanvas` takes the previous hash and returns what is now on the canvas: skip and
  return the previous hash on a match; write and return the new one otherwise. Return the previous
  hash on a failed write, and on the early return when its own `findOpenPRs` fails, so a write that
  never reached Slack is not recorded as applied
- `Run` passes the loaded hash and saves the returned one. With the canvas disabled it carries the
  loaded hash through

Post runs load no state, so they always write, re-seeding the hash after the artifact expires.

Tests on `canvasContentHash`:

- Differing only in `GeneratedAt` hashes the same; differing in a PR row hashes differently
- Differing only in `MergedPRsUnavailable`, both with an empty merged list, hashes differently.
  That flag only feeds the empty-section text, so with rows present it changes no markdown
- The passed-in `Content` is unmutated and still renders its real footer afterwards

Tests through `Run`, seeding the hash from a first run's saved state file rather than a literal
digest:

- Seeded hash matches: `ReplacedCanvas.Called` is false, saved state keeps the hash
- Seeded hash differs, and seeded state has no hash: both write
- `ReplaceCanvasError` set: saved state keeps the seeded hash
- Post mode: writes, and saves the hash

Keep fixture ages off a `durationText` rounding boundary, as the current ones are, so both runs in
a test render identical row text.

## Step 4: Update the README's canvas section

Touches: `README.md`.

Correct two statements this falsifies:

- The canvas intro, "The canvas is rewritten on every run of the action, in both `post` and
  `update` mode"
- The first `### Good to know` bullet, "Every run replaces all of its content, so anything typed
  there by hand is lost". Hand-typed content now survives until the next write

Add to `### Good to know`:

- A duplicated canvas is a rendering artifact in the Slack client, not lost data. Reload it
- `_Updated <ts>_` says when the canvas was last written, not when the action last ran

Add Step 1's concurrency block to the "Advanced setup with update-mode enabled" example. It is the
only example with more than the schedule trigger, so it is the only one that can overlap.

No tests. Done means both corrections, both bullets, and the concurrency group in that one example.

## Consequences

### Positive

- Runs seconds apart no longer write twice, which is the case that corrupted the canvas
- A repository whose PRs are all over a day old writes about once a day instead of every run
- The state artifact becomes a record of what is on the canvas

### Negative

- The footer no longer proves the action ran. A stale canvas and a broken action look alike
- A hand-edited canvas keeps those edits until the next write

### Caveats

- One state artifact per run instead of per day, until `retention-days: 1` expires them, since an
  upload always creates a new artifact
  ([upload-artifact README](https://github.com/actions/upload-artifact#readme)). `Load` takes the
  newest, so this is noise, not incorrectness
- The skip weakens as PRs get younger. Under an hour old, row text changes every minute
- The scheduled run can still overlap an update run, since it sits in its own group. Once a day at
  most, accepted against a cancellable scheduled run

### Neutral

- Neither change prevents the corruption, they only make it rare
