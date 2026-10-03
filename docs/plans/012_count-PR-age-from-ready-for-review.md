# Count PR age from ready for review

date: 2026-10-03
status: implemented

## Requirements

A PR's age, its 🚨 marker and the order of open-PR rows read `createdAt`. A PR that spent a week as a draft is flagged old the moment it is opened for review, though nobody has been asked to review it yet.

- An open PR's age counts from its ready-for-review time, in the message and on the canvas
- The old-PR check (`old-pr-threshold-hours`, 🚨) reads the same time
- Open-PR rows sort oldest first by the same time, so a row's order matches the age beside it
- A PR marked ready, converted back to draft and marked ready again keeps its first ready-for-review time
- Non-goals:
  - A per-section "waiting N days" text. A later plan
  - Any change to WIP rows (they show activity, not age) or merged rows (they show the merge time)
  - The 50-PR fetch cap: it keeps the newest PRs by creation time, as now
- Work lands on a feature branch, then a PR

Purpose: an age that counts draft time cries wolf, and a 🚨 that cries wolf trains the team to ignore it. In the reference deployment, where a draft with recent activity is a real WIP signal, every PR that starts as a draft hits this.

## Target Shape

- **Ready-for-review time**: when the PR was first marked ready for review, or its creation time when it was opened as non-draft
  - `githubclient.PR.FirstReadyForReviewEventAt *time.Time`: the earliest ready-for-review event's time, nil when there is none
  - `githubclient.PR.ReadyForReviewAt()`: that time, else `CreatedAt`. Every reader goes through it, so a PR built without the field, by a failed enrichment or a test fixture, reads its creation time
  - Read off `timelineItems(itemTypes: [READY_FOR_REVIEW_EVENT], first: 1){ nodes { ... on ReadyForReviewEvent { createdAt } } }`, selected in the enrichment fragment and in the `GetPRs` selection. See Justification § Why the enrichment query
  - `first: 1` is the earliest event, measured rather than documented. See docs/third-party-facts.md § `timelineItems` filters `itemTypes` before paging and returns events oldest first, measured only
  - A PR opened as non-draft has no such event. See docs/third-party-facts.md § GitHub records a `ReadyForReviewEvent` on each draft-to-ready switch, never when a PR is opened as non-draft
  - "No event" is read off `nodes` being empty, never `totalCount`, which ignores `itemTypes`
- A PR's **age** is the time since its ready-for-review time. `prview` reads `ReadyForReviewAt()` wherever it read `createdAt` for age, the old-PR check and the open-PR sort
- Display text is unchanged: "N days ago", or "N days old" with 🚨 past the threshold
- `old-pr-threshold-hours` keeps its name and default. Its description in `action.yml` and README says the age counts from ready for review
- No new action input or OAuth scope, and no permission beyond [Required Permissions](../../README.md#required-permissions), which already grants `pull-requests: read` and `issues: read`. The REST timeline endpoint takes either ([timeline](https://docs.github.com/en/rest/issues/timeline?apiVersion=2022-11-28)); the GraphQL field's permission is undocumented, so Step 1's live check settles it
- Query cost is unchanged: the enrichment query logged the same `rateLimit.cost`, about 1 per PR, with and without `timelineItems`. See docs/third-party-facts.md § The GraphQL cost formula overestimated a search, but matches the enrichment query's logged cost of about 1 per PR: read `rateLimit.cost`
- Rendered rows, for a PR created 10 days ago as a draft and marked ready 2 days ago, threshold 96 hours:
  - Before: `Add retries 🚨 10 days old by Alice`
  - After: `Add retries 2 days ago by Alice`

## Breaking Change

Non-breaking, minor: the age and 🚨 in every message and canvas change meaning. State written by the previous version holds no timestamps this reads.

## Summary

1. Fetch the ready-for-review time (checkpoint)
2. Count age from it in `prview`
3. Docs

## Steps

### Step 1: fetch the ready-for-review time (checkpoint)

- `githubclient`: select the `timelineItems` connection in `enrichedPullRequestSelection` and `fullPullRequestSelection`
  - New `pullRequestNode` field for it, plus a `readyForReviewEventNode` type
- `githubclient.PR.FirstReadyForReviewEventAt`, filled in `prWithReviewers`; new `PR.ReadyForReviewAt()` getter
- `logEnrichment` prints the event's time, or that there is none
- `testhelpers/mockgithubclient`: new `FirstReadyForReviewEventAtByPRNumber` option, rendered as the `timelineItems` connection on enriched nodes. No entry renders an empty connection
- `githubclient.spec.md`: the new field and getter, `timelineItems` added to the per-PR page sizes line and to the **Doesn't Do** lists naming per-PR sub-fields
  - The **Oddities** entry on a PR left without reviewer info by a failed enrichment adds that it reads its creation time as its ready-for-review time
- Live check:
  - Open this plan's PR as a draft, mark it ready, then push a commit, since Build doesn't fire on `ready_for_review`. See docs/third-party-facts.md § A `pull_request` `types:` list replaces the default `opened, synchronize, reopened`
  - Build's E2E multi-repository run lists `hellej/pr-slack-reminder-action` under the App token
  - `gh workflow run pr-reminder.yml --ref <branch> -f run-mode=post -f build-first=true` runs under `GITHUB_TOKEN`. Its job grants `pull-requests: read` and `contents: read` on a public repository, so it doesn't isolate either permission
  - Done means both job logs print this PR's ready-for-review event time. Record the result in docs/third-party-facts.md
  - An empty connection in either log stops the implementation and goes back to the user: the failure is silent, the age falling back to creation time. See docs/third-party-facts.md § No GitHub page documents the token permission for any GraphQL field

### Step 2: count age from it in `prview`

- `prview`: `GetPRAgeText`, `isOlderThan` and `SortPRsOldestToNewest` read `ReadyForReviewAt()` instead of `GetCreatedAt()`. ~3 call sites, no new symbol
- Existing fixtures set only `CreatedAt` and keep passing through the getter's fallback. No existing snapshot or canvas golden changes
- Snapshot tests in `cmd/pr-slack-reminder/snapshot_test.go`: new case with a PR created past the threshold and marked ready inside it, plus a second PR whose creation and ready orders disagree. The message snapshot shows no 🚨 and the ready-time order
- `cmd/pr-slack-reminder/canvas_test.go`: two PRs of the same shape on the canvas, asserting the age texts, the 🚨 and the row order. The canvas goldens can't tell the two times apart: `canvasbuilder`'s fixtures set only `CreatedAt`, and `IsOldPR` by hand
- `prview_test.go`: two PRs tied on ready-for-review time but not on creation time still fall to the `UpdatedAt` tie-break, and the old-PR check's zero guard reads the ready-for-review time
- `prview.spec.md`: age and old-PR flag read the ready-for-review time, in **Behaviour**, the **Doesn't Do** future-time entry and the zero-time oddity. New **Behaviour** entry for `SortPRsOldestToNewest`
- `messagecontent.spec.md` and `canvascontent.spec.md`: "oldest" means by ready-for-review time

### Step 3: docs

- `action.yml` and README inputs table: `old-pr-threshold-hours` counts from ready for review, or from creation for a PR never a draft. `go run .github/scripts/check_inputs.go` passes
- README: new `### What the times mean` subsection under `### Example Output`, one bullet per row time:
  - Open PR, "N days ago" / "N days old": since the PR was first marked ready for review, or since it was opened if it was never a draft. A PR moved back to draft and marked ready again keeps its first time
  - WIP PR on the canvas, "updated N ago" / "idle N days": since the PR's last update
  - Merged PR, "merged N ago": since the merge

## Consequences

### Positive

- 🚨 fires only on PRs that have actually waited for review past the threshold
- Row order matches the ages shown

### Negative

None

### Caveats

- A PR whose enrichment failed shows its creation age, so it can be flagged old wrongly for that run
- A PR re-drafted and marked ready again keeps counting from its first ready time, including the time it spent back in draft
- `first: 1` being the earliest event rests on measurement. Should GitHub reorder, a re-drafted PR would count from a later ready time
- Over the 50-PR cap, a PR created long ago as a draft and marked ready recently is among the first dropped, since the cap keeps the newest by creation time
- A test fixture that forgets `FirstReadyForReviewEventAt` silently tests creation time

### Neutral

- One more connection per PR in the enrichment and `GetPRs` queries

## Justification

### Why the enrichment query

- The listing query selects up to 100 PRs per repository, drafts included; the enrichment query covers only the capped set the run renders
- `GetPRs` and the merged enrichment select it too, so every enriched PR carries its event. Nothing reads it there today: update mode's open rows come from the live open fetch
