# Reduce PR tracker canvas writes

date: 2026-09-05
status: draft

## Requirements

Slack's canvas client mis-merges full-document replaces that land while someone has the canvas
open, rendering duplicated headings and PR rows. Measured on 2026-09-05 against canvas
`F0BPS4FKCEL`: 15 replaces 1.5s apart, each reshaping the document, produced `## Open` and
`## WIP` twice, one PR row under two sections, empty-state text beside real rows, and the footer
stranded mid-document. See the "Rapid `canvases.edit` replaces duplicate headings and rows in an
open canvas" entry in [docs/third-party-facts.md](../third-party-facts.md) for the measurement.

The corruption is client-side only. A reload re-renders the last payload exactly. Nothing can stop
Slack from mis-merging, so the goal is to make the writes that trigger it rare, and to tell
readers what a duplicated canvas means.

Goals:

- Stop two runs of the workflow from writing the canvas back to back
- Stop rewriting the canvas when the markdown would be the same
- Say in the README that a duplicated canvas is a rendering artifact, and that the canvas is no
  longer rewritten on every run

Non-goals:

- No new action inputs, and no way to switch either behaviour off
- No retry, backoff or read-back verification of a canvas write. Slack documents no method that
  returns a canvas's markdown
- No change to how the reminder message is posted or updated
- No change to what the canvas renders

## Target shape

`action.yml` is untouched: no new, renamed or removed inputs, and no new GitHub token permission
or Slack scope.

`State` carries the hash of the canvas markdown a run put on the canvas. A run that renders
matching markdown skips `ReplaceCanvasContent`.

**What the hash covers.** An unexported `canvasContentHash(content)` in
`cmd/pr-slack-reminder/canvas.go` copies the built `Content`, zeroes `GeneratedAt` on the copy,
renders it through `canvasbuilder.BuildMarkdown`, and returns the SHA-256 hex of that string. It
stays unexported with one caller, and a `package main` internal test file reaches it directly, as
`internal/canvasbuilder/escape_internal_test.go` already does for that package.

Zeroing `GeneratedAt` neutralises the footer, which is rendered at minute resolution. Without it
two runs that straddle a minute boundary render different markdown from identical PR data.

The zeroing happens on a copy of the already-built `Content`, never by re-calling
`canvascontent.GetContent` with a zeroed `GeneratedAt`. `GeneratedAt` is also the "now" that prunes
inactive drafts (`isActiveEnoughForCanvas`, `canvascontent.go:92-98`), so a zero value there puts
the cutoff in year 0 and keeps every dead draft. Copying also keeps the caller's `Content` intact,
so the markdown actually sent still carries the real footer.

**What the hash does not cover.** Row text moves with the wall clock independently of
`GeneratedAt`. `GetPRAgeText`, `IsOldPR` and `IsRecentlyUpdated` in `prparser`, and
`GetActivityText` and `GetMergedText` in `displaytext.go`, all read `time.Since` or `time.Now`
directly. `durationText` renders minutes under an hour, hours under a day, then days, and
`IsRecentlyUpdated` flips a WIP row between a code span and italics at 24h. So the hash moves as
PRs age: every minute while any listed PR is under an hour old, about hourly under a day, then
daily.

That is enough for the problem at hand. Two runs seconds apart render identical row text, so the
second skips, and two runs seconds apart is exactly what corrupts the canvas. The skip is not a
general "only write when PR data changes".

**One state writer.** Today `runPostMode` writes state mid-run, before the canvas refresh, so the
hash does not exist yet at that point. After this, both run modes hand their state back to `Run`,
which writes it once, after the canvas refresh.

**Update runs start uploading a state artifact.** Today they upload nothing:
`FetchLatestArtifactByName` streams the artifact zip to a temp file and decodes the entry from it,
never writing `stateFilePath` (`internal/apiclients/githubclient/fetchartifact.go:70-113`), so the
workflow's `upload-artifact` step finds no file after an update run. Without this the hash would
only be refreshed by the daily `post` run, and every update run after the first content change
would write again.

**State file compatibility, both directions.** State decodes with a plain
`json.NewDecoder(...).Decode(target)` (`fetchartifact.go:99-101`); `DisallowUnknownFields` is used
only for the `filters` input (`internal/config/filters.go:55`).

- A new run reading an artifact written before this change decodes an empty
  `CanvasMarkdownHash`, which reads as "unknown, write the canvas"
- A pinned older version reading an artifact written after it ignores the unknown key
- `CurrentSchemaVersion` stays 1, so version 1 comes to describe two shapes. Deliberate: nothing
  reads the version on load, per [the state spec](../../internal/state/state.spec.md), and a bump
  with no reader is decoration

**The workflow serializes its own runs.** GitHub cancels any pending run in a concurrency group
when a new one queues, so a burst of triggers collapses to the run in progress plus the newest
queued one ([workflow syntax: concurrency](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#concurrency)).
`cancel-in-progress` stays `false`: `true` would kill an in-flight `post` run mid-send and lose
both the reminder and the state artifact.

**The footer changes meaning.** It says when the canvas was last written, not when the action last
ran. Wording stays `_Updated <ts>_`.

## Breaking change

Minor. The action's contract is unchanged: no input renamed, removed or added, no config format
change, and the canvas renders the same content from the same data. The release table's "changed
default behavior" row under **major** is read here as covering the action's contract, not an
internal write that produces the same result. The two behaviour changes a user can observe, a
footer timestamp that stops moving while nothing changes and a state artifact from update runs,
break no existing configuration.

## Steps

- **R1** Hand `post` mode's state back to `Run`
- **1** Add a concurrency group to the reminder workflow
- **2** Hand `update` mode's state back to `Run`, and save it
- **3** Skip the canvas write when the markdown would be unchanged
- **4** Update the README's canvas section

## Step R1: Hand `post` mode's state back to `Run`

Touches: `cmd/pr-slack-reminder/run.go`, `internal/state/state.go`, `internal/state/state.spec.md`,
`cmd/pr-slack-reminder/main_test.go`.

`runPostMode` calls `state.SavePostState` before `Run` reaches the canvas refresh, so it cannot
carry a hash the refresh has not produced yet.

- Split `SavePostState` into a builder plus the existing write: `state.NewPostState(parsedPRs,
  messageInfo) State`, written by `state.Save(filePath, state)`
- `NewPostState` is the one place that stamps `SchemaVersion` and `CreatedAt`, exactly as
  `savePostState` does today. `Save` keeps writing the struct as given and stamps nothing
- The `failed to save state: %w` wrap and the `Saved state to %s with %d PRs` log move to the call
  site in `Run`, so neither is lost
- `runPostMode` becomes `(*githubclient.OpenPRsResult, *state.State, error)` and returns a `nil`
  state on the paths that write none today: the open-PR fetch failure (`run.go:82`), the early
  return with no PRs and no `no-prs-message` (`:95`), and a failed send (`:101`)
- `Run` writes a non-nil returned state after the canvas refresh, and joins any write error into
  the error it already returns from `errors.Join(messageErr, canvasErr)`

Three deltas this accepts, none of which change what a successful run produces:

- `sentMessageHandler` now runs before the state write, not after. Today a state-write failure
  returns before the handler runs (`run.go:103-106`)
- State lands on disk after the canvas refresh instead of before it. A post run cancelled inside
  the refresh loses the state artifact although the message was already sent
- A state-write error now joins the run's other errors instead of short-circuiting

Tests: the existing post-mode state assertions in `main_test.go` still hold, and a post run whose
send fails still writes no state file.

## Step 1: Add a concurrency group to the reminder workflow

Touches: `.github/workflows/pr-reminder.yml`.

Add at the top level:

```yaml
concurrency:
  group: ${{ github.workflow }}
  cancel-in-progress: false
```

`github.workflow` is constant for this workflow, so every run lands in one group regardless of ref
or event. Merging a PR fires `push` and `pull_request closed` seconds apart, and those two runs are
what put two canvas writes back to back.

Nothing in this repo lints workflow YAML, so this step has no test. Done means the next triggered
run still starts and completes, and two runs triggered within seconds of each other show one
`in_progress` and one `queued` in the Actions tab rather than two `in_progress`.

## Step 2: Hand `update` mode's state back to `Run`, and save it

Touches: `cmd/pr-slack-reminder/run.go`, `internal/state/state.spec.md`,
`cmd/pr-slack-reminder/main_test.go`.

Step 3's skip reads the previous hash off the state update mode loaded, so update mode has to hand
that state back first.

- `runUpdateMode` becomes `(*state.State, error)` and hands back the state it loaded on every path
  that has one, including both early returns: no PRs in the loaded state (`run.go:125-128`), and
  all PRs gone so the message was deleted (`:140-150`). It returns `nil` when the load itself
  failed (`:115-124`), which `Run` already skips
- The loaded state is written back as loaded: `SlackMessage`, `PullRequests`, `CreatedAt` and
  `SchemaVersion` unchanged, with only the canvas hash set by Step 3. Update mode tracks the PR set
  the `post` run captured, and refreshing that list would change which PRs it tracks. Keeping
  `CreatedAt` keeps it meaning what the state spec says, the time this PR set and message ref were
  created
- No change to `internal/state/state.go` in this step; `Save` and the struct are as R1 left them

Saving on the delete path keeps state pointing at a message that was just deleted. That is what
already happens: today the next update run loads the same pre-delete artifact, finds no PRs, and
deletes again, which `DeleteMessage` treats as success. Saving here reproduces that rather than
changing it, and it is what lets the canvas hash carry forward on a quiet day.

Tests through `Run` with the existing mocks:

- An update run saves a state file whose `SlackMessage` and `PullRequests` match what was loaded,
  with a fixture where the loaded set and this run's fetched set differ: load PRs {1, 2} with PR 2
  dropped by an `ignored-authors` filter, and assert the saved state still lists both. The existing
  `TestScenariosUpdateMode` fixtures use the same set for `mockState` and `prByNumber`, so an
  implementation saving this run's parsed PRs would pass against those
- An update run that deletes the message still saves a state file
- An update run whose loaded state has no PRs still saves a state file
- An update run whose state load fails writes no state file

## Step 3: Skip the canvas write when the markdown would be unchanged

Touches: `internal/state/state.go`, `internal/state/state.spec.md`,
`cmd/pr-slack-reminder/canvas.go`, `cmd/pr-slack-reminder/run.go`,
`cmd/pr-slack-reminder/canvas_internal_test.go` (new), `cmd/pr-slack-reminder/canvas_test.go`.

- Add `CanvasMarkdownHash string \`json:"canvasMarkdownHash"\`` to `state.State`
- Add the unexported `canvasContentHash` described in Target shape
- `refreshPRTrackerCanvas` takes the previous hash and returns the hash now on the canvas:
  - Hash matches the previous one: skip `ReplaceCanvasContent`, log the skip, return the previous
    hash
  - Hash differs: write, and return the new hash on success
  - Write fails: return the **previous** hash, so a failed write is never recorded as applied.
    Recording it would make the next run skip and leave the canvas stale
  - The function returns early before rendering anything when update mode's own open-PR fetch
    fails (`canvas.go:32-38`). It returns the previous hash there too
- `Run` passes the hash from the loaded state, and writes the returned hash into the state it saves

Post runs carry no loaded state, so they always write. That is the daily scheduled run plus a
manual dispatch, and it re-seeds the hash after the artifact expires.

Tests on `canvasContentHash` in `canvas_internal_test.go`:

- Two `Content` values differing only in `GeneratedAt` hash the same
- Two differing in one PR row hash differently
- Two differing only in `MergedPRsUnavailable`, both with an empty merged list, hash differently.
  That flag only feeds the merged section's empty text, so with merged rows present it changes no
  markdown
- The passed-in `Content` is unchanged after the call, and rendering it afterwards still produces
  its real footer. This is what the copy buys, and it fails if the implementation zeroes
  `GeneratedAt` on the caller's value

Tests through `Run` in `canvas_test.go`, seeding the loaded state's hash from a first run's saved
state file rather than hardcoding a digest:

- Update mode, seeded hash matches: `MockSlackAPI.ReplacedCanvas.Called` is false, and the saved
  state keeps the hash
- Update mode, seeded hash differs: the canvas is written, and the saved state carries the new hash
- Update mode, seeded state has no hash: the canvas is written
- Update mode, `ReplaceCanvasError` set: the saved state keeps the seeded hash, not the new one
- Post mode: the canvas is written, and the saved state carries the hash

Seeding from a first run works because both runs in a test render identical row text. Keep fixture
ages away from a `durationText` rounding boundary, as `canvas_test.go`'s current ones are.

A failed merged-PR fetch renders `_Merged PRs could not be fetched_`, which hashes differently from
a section with rows, so the next successful run writes.

## Step 4: Update the README's canvas section

Touches: `README.md`.

Two existing statements become false and have to change, not just be added to:

- In the PR Tracker Canvas intro, "The canvas is rewritten on every run of the action, in both
  `post` and `update` mode": now rewritten only when the markdown would differ
- The first `### Good to know` bullet, "Every run replaces all of its content, so anything typed
  there by hand is lost": still true when it writes, but hand-typed content now survives until the
  next write

Add to `### Good to know`:

- A canvas showing duplicated headings or PR rows is a rendering artifact in the Slack client, not
  lost data. Reload the canvas and it shows the real content
- `_Updated <ts>_` says when the canvas was last written, not when the action last ran

No tests. Done means both existing statements are corrected and both bullets are in the
`### Good to know` list under [PR Tracker Canvas](../../README.md#-pr-tracker-canvas).

## Consequences

### Positive

- Runs triggered seconds apart no longer write the canvas twice, which is the case that corrupted
  it
- Bursts of triggers collapse to two runs, and those two are serialized
- A repository whose PRs are all more than a day old writes the canvas about once a day instead of
  on every run
- The state artifact becomes a record of what is on the canvas, which a later change could diff
  against

### Negative

- The footer timestamp no longer proves the action ran. A stale-looking canvas and a broken action
  look the same
- Update runs upload a state artifact where they uploaded none, so `Load` starts picking up
  update-run artifacts
- A canvas edited by hand keeps those edits until the next write, instead of losing them on the
  next run
- The skip weakens as PRs get younger. While any listed PR is under an hour old, row text changes
  every minute and almost every run writes

### Neutral

- Neither change prevents the corruption. Slack merges the writes; these only make them rare
- `post` runs always write, since they load no state
