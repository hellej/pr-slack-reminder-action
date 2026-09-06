# Canvas workflow triggers

date: 2026-09-06
status: draft

## Goals

- Correct the one trigger list the README shows, inside the `update` mode example: it was
  chosen for that mode's message and misses events that change an updated message
- Serve the canvas from that same list, so a canvas user has no second list to keep in sync
- Run the list in this repository's own workflow, where `types: [closed,
  ready_for_review]` has been suppressing the `opened` the default set gives

## Non-goals

- No input, permission or scope changes
- No `repository_dispatch` setup for the other monitored repositories
- No `github.event.issue.pull_request` conditional: comments on issues keep starting runs
- No fork guidance, and no `synchronize` trigger
- No `pull_request_review_comment` trigger: a lone inline diff comment submits an implicit
  review, so `pull_request_review` already catches it (PR #58, run 34025576473)

## Target shape

`#### 3. Update Mode Enabled` holds the one trigger list, serving both the updated message and
the canvas. `.github/workflows/pr-reminder.yml` takes the same list. `## PR Tracker Canvas`
gains nothing, so there is no second `on:` block to drift.

```yaml
on:
  schedule:
    - cron: "0 9 * * MON-FRI"
  push:
    branches: [main]
  pull_request:
    types: [opened, reopened, closed, ready_for_review, converted_to_draft]
  pull_request_review:
    types: [submitted, dismissed]
  issue_comment:
    types: [created, deleted]
```

- `opened` serves the canvas only: an updated message re-renders the state-tracked PRs only, so
  a PR opened after the post run can never join it
- Naming any `types:` replaces the `pull_request` default of `opened`, `synchronize`,
  `reopened` ([events that trigger workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows))

No action input changes, no new GitHub token permissions, no new Slack scopes.

## Breaking change

Non-breaking, **patch**: documentation plus this repository's own workflow. No shipped
behaviour changes.

## Steps

1. Put the trigger list in `#### 3. Update Mode Enabled` of `README.md`
2. Put the same list in `.github/workflows/pr-reminder.yml`

### 1. Put the trigger list in `#### 3. Update Mode Enabled`

Touches: `README.md`.

- Replace `types: [closed, ready_for_review]` with `types: [opened, reopened, closed,
  ready_for_review, converted_to_draft]`, and add `dismissed` to `pull_request_review`
- Narrow the bare `issue_comment:` to `types: [created, deleted]`
- Keep `push: branches: [main]` and the cron comment, with no README explanation of the push.
  See [Keeping the push trigger](#keeping-the-push-trigger)
- The four types besides `opened` are two pairs: `converted_to_draft` hides a tracked PR, since
  `GetPRs` filters drafts out through `getPRFilterFunc`, and `ready_for_review` brings it back;
  `closed` restyles the row, struck through per `IsClosedButNotMerged` or suffixed 🚀 per
  `IsMerged`, and `reopened` clears the strike
- `dismissed` changes the review emojis the same way `submitted` does
- Leave `## PR Tracker Canvas` untouched, intro, `### Setup` and `### Good to know` alike, "Every
  run rewrites it" included: it is what a reader sees, and the unchanged-content skip is an
  implementation detail

No tests. Done means the example's list matches Step 2's.

### 2. Put the same list in `.github/workflows/pr-reminder.yml`

Touches: `.github/workflows/pr-reminder.yml`.

- Keep `workflow_dispatch` with its `run-mode` and `build-first` inputs, the `concurrency` block
  and the existing `schedule`
- Its bare `issue_comment:` narrows to `types: [created, deleted]`, which drops `edited` only

No tests, and no `actionlint` in this environment. Done means the file still parses as YAML, and
a PR opened in this repository starts a run, which the current list never did.

## Consequences

### Positive

- One list serves the updated message and the canvas
- `opened` starts working in this repository

### Caveats

- More runs. The unchanged-content skip keeps most of them from writing to the canvas, but a row
  reads its age off the wall clock, so a run whose rows have aged past a rounding boundary still
  writes
- More failed runs. Every added trigger runs `update` mode, and `state.Load` fails when no state
  artifact exists, which is the case after a `post` run that found no PRs and had no
  `no-prs-message` to send. The canvas still refreshes on such a run, so the cost is a red run,
  not a stale canvas
- Label and title filters are unserved. `labeled`, `unlabeled` and `edited` are left out for run
  volume, so a PR that a label or a retitle moves in or out of the filters waits for the schedule

## Justification

### Keeping the push trigger

A merge fires `pull_request: closed` and `push` on main together, and both land in the same
concurrency group with `cancel-in-progress: false`, so the second waits for the first. That gap
is what makes the pair useful: GitHub's PR search index lags a merge by seconds (polled with
`gh api graphql` against `NixOS/nixpkgs` and `ClickHouse/ClickHouse`: absent 3 seconds after
`mergedAt`, present at 10), so the first run can miss the merge that triggered it, and the
second finds it.
