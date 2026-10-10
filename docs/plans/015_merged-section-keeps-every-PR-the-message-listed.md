# Merged Section Keeps Every PR the Message Listed

date: 2026-10-10
status: draft

## Requirements

- Once a PR has been listed in an open section of the channel message, the merged section shows it after it merges, for the rest of that message's life
  - Today an update run's listed PRs are not saved. A PR opened after the post reaches the merged section only through the merged search, capped at the 6 newest merges across all repositories, so a 7th merge after the post drops the oldest
  - Today a failed merged search drops those PRs for that run as well
- Non-goals:
  - The PR tracker canvas: its merged section keeps its own caps
  - A run where the merged search and the tracked PR fetch both fail: it edits without the rows those fetches would have given, as today. The next successful run restores them
  - A PR never listed in an open section, such as one that opened and merged between two update runs, or one merged before the post: it stays subject to the merged section's caps, so its row can still drop out later the same day
- Serves AGENTS.md § Purpose: the merged section is the team's record of what landed since the post. On a day merging most of a weekly Dependabot batch, it no longer loses PRs the team saw in the message
- Lands on a feature branch, through a PR

## Target Shape

- **Tracked PRs**: every PR the message has listed in an open section since it was posted. Saved in state under the `trackedPRs` key
  - Post mode saves the PRs it listed, as today
  - Update mode, after a successful edit, adds the PRs it listed to the loaded tracked PRs and saves the result. Loaded refs keep their order, newly listed refs append in the open fetch's order, no duplicates
  - A deleted or kept message, or a failed edit, saves the loaded state unchanged, as today
- A tracked PR that has merged is already shown however long ago it merged, and is re-fetched by `GetPRs` when neither the open nor the merged fetch resolves it (`resolveTrackedPRs`). Neither changes
- No change to action inputs or permissions
- No message layout change: a row that would have dropped stays, rendered as any merged row

### GitHub API Budget

- No query changes
- Each update run re-fetches tracked PRs that neither the open nor the merged fetch resolves, about 1 point each (AGENTS.md § GitHub API Budget)
  - New entries there: listed PRs that later merged outside the merged search's 6, closed unmerged, got snoozed, or turned draft while the canvas is off
  - In the reference deployment, 0 on most runs

## Breaking Change

- Non-breaking, patch: a fix with no input change
- State keeps its schema version. The first run after upgrading reads a state without `trackedPRs` through the `pullRequests` fallback, and an update run adds what it lists
- A downgraded version finds no `pullRequests` key in a state this version saved, so it shows no tracked merged rows until the next post

## Summary

- R1: rename the state's PR set to tracked PRs, Go field and JSON key, reading the old key as a fallback
- 1: update mode saves the PRs its edit listed as tracked

## Steps

### R1: Rename the State's PR Set to Tracked PRs

- `internal/state`: rename `State.PullRequests` to `TrackedPRs`, JSON key `trackedPRs`
  - ~12 references across `state`, `cmd/pr-slack-reminder` and their tests
  - `State.UnmarshalJSON` reads `pullRequests` when `trackedPRs` is absent or empty, the same way as the `createdAt` and `slackMessage` fallbacks
  - Test it in `TestStateDecodesTheLegacyMessageKeysWhenTheNewOnesAreAbsent`, renamed to cover legacy keys in general
- State snapshots in `cmd/pr-slack-reminder/testdata/snapshots/` change by the key only. Read the diff, then re-record
- `state.spec.md`:
  - The intro and § Behaviour name the field tracked PRs, no longer the post run's PR set
  - § Oddities: add `pullRequests` to the legacy key fallback bullet

### 1: Update Mode Saves the PRs It Listed

- `internal/state`: add `WithTrackedPRsAdded(loadedState, prViews)`, returning a copy with the views' refs added per § Target Shape
- `cmd/pr-slack-reminder/run.go` `runUpdateMode`: after a successful edit, apply it to the state it saves, with the open PR views the message was built from
- Tests:
  - The update-mode snapshot "every section under load" lists a PR missing from its loaded state, so its `.state.json` is the test. Read the diff, then re-record
  - Extend the "message edit fails" case in `TestUpdateModeStateSavingOnEarlyReturns` with an open PR missing from state, asserting the loaded tracked PRs save unchanged
  - Reword the doc comment of `TestUpdateModeSavesTheLoadedState`: the saved state is the loaded one plus the listed PRs
- Specs:
  - `state.spec.md` § Behaviour: a bullet for `WithTrackedPRsAdded`, next to `WithLastWrittenMessage`
  - `state.spec.md` § Oddities: the bullet on what an update run rewrites becomes `LastWrittenMessage`, the canvas hash and the tracked PRs. `MessageRef` and `MessagePostedAt` stay the post run's
  - `run.spec.md` § Behaviour: update mode's saved state adds the edit's listed PRs
  - `messagecontent.spec.md`: `trackedPRs` are the PRs the message has listed in an open section since it was posted, as re-fetched
- `README.md` [update mode](#3-update-mode-enabled): PRs that merged since the original message move to the merged section, including ones opened after it

## Consequences

### Positive

- On a run whose tracked PR fetch succeeds, the merged section shows every PR the message listed that has since merged, whatever the number of merges since the post
- A failed merged search no longer drops listed PRs: the tracked PR fetch covers them

### Negative

- None

### Caveats

- One more legacy key fallback in `State.UnmarshalJSON`, to remove in a later release with the others

### Neutral

- The plan "Trust Only Non-fork State Artifacts and Attest Release Binaries" also edits `state.Load`, `state.spec.md` and `run.spec.md`. Whichever lands second resolves the merge conflicts
