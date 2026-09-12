# Canvas open PR buckets

date: 2026-09-12
status: draft

## Goals

- Split the canvas `## Open` section into three sections, bucketed by whose turn it is
- Make a canvas reader able to pick their next action from the heading alone
- Keep the bucketing rule callable from `messagecontent`, so the scheduled reminder message
  can adopt it later without a rewrite
- Land the work on a feature branch and a PR, not a direct push to `main`

## Non-goals

- Bucketing the reminder message. Canvas only
- CI status. A red build does not move a PR
- `## WIP` and `## Merged`. Unchanged
- New action inputs. The split is unconditional for anyone using `pr-tracker-canvas-link`

## Motivation

The canvas lists every open PR under one heading, ordered by age. A reader cannot tell an
approved PR waiting on a merge click from one nobody has opened yet, so every row costs the
same attention and the cheap wins stay buried.

## Target shape

Three `##` sections replace `## Open`, in descending closeness to main:

```
## Ready to merge
## Waiting for author
## Waiting for review
## WIP
## Merged
```

A PR lands in the first bucket that matches:

1. Conflicting → Waiting for author
2. At least one approval, no unresolved review threads → Ready to merge
3. Any non-approving review, or any unresolved review thread → Waiting for author
4. Otherwise → Waiting for review

Conflicts outrank "nobody reviewed yet": a conflicted PR never sits in the review queue, since
reviewing it is wasted until the author rebases.

Rendering rules:

- A bucket with no PRs renders nothing, no heading and no fallback line
- All three empty renders one `## Open` heading with `_No open PRs_`, keeping today's
  zero-state signal
- Repository sub-headings stay at `###`, so `group-by-repository` is untouched. Each bucket
  groups independently
- Rows keep `renderOpenPRRow` unchanged, including the `(✅ a / 💬 b)` reviewer segments
- The footer cap note keeps its current wording

New data per PR, all from the existing enrichment request:

| Field | Source |
|---|---|
| `HasUnresolvedReviewThreads` | `reviewThreads(first: 100){ nodes { isResolved } }` |
| `Conflicting` | `mergeable`, `CONFLICTING` only |
| `HasNonApprovingReview` | the already-selected `reviews.nodes[].state` |

No `action.yml` change, and no new token permission for the action itself: `README.md` already
documents `pull-requests: read` for listing and fetching PRs and reviews, and the REST
equivalents of `reviews`, `comments` and `reviewThreads` all sit under Pull requests at read
([permissions for fine-grained PATs](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens)).
This repository's own `pr-reminder` workflow grants less than that today, which Step 1 corrects.

## Breaking change

**Minor.** No input is renamed, removed or given a new default, and every existing workflow
keeps running unchanged. The canvas layout changes for anyone who set
`pr-tracker-canvas-link`, which is an enhancement to an optional feature rather than an
incompatible config.

## Steps

- **R1** — introduce `canvascontent.PRSection`, converting the existing three sections
- **1** — fetch thread and conflict state in `githubclient`
- **2** — add the bucketing rule to `prparser`
- **3** — bucket and render the three sections
- **4** — verify against the live canvas

### R1: introduce `canvascontent.PRSection`

Files: `internal/canvascontent/`, `internal/canvasbuilder/`, `cmd/pr-slack-reminder/canvas_internal_test.go`

- Add `PRSection` holding the flat and grouped shapes of one section:
  - `PRs []prparser.PR` and `Groups []prparser.RepositoryPRs`, the latter named to match
    `canvasbuilder.section`
  - Only one is ever populated, as today
- Replace the six paired fields on `Content` with `Open`, `WIP` and `Merged` of that type
- `canvasbuilder.BuildMarkdown` reads `content.Open.PRs` and `content.Open.Groups` in place of
  the paired fields; `section` keeps its current fields
- No behaviour change. The `testdata/` golden files must come out byte-identical, which is the
  step's regression check
- Update `canvascontent.spec.md` for the struct shape

See [Why a section type first](#why-a-section-type-first).

### Step 1: fetch thread and conflict state

Files: `internal/apiclients/githubclient/`, `.github/workflows/pr-reminder.yml`

- Add to `enrichedPullRequestSelection` and to `fullPullRequestSelection`:
  - `mergeable`
  - `reviewThreads(first: 100){ nodes { isResolved } }`, 100 being the page maximum
    ([GraphQL pulls reference](https://docs.github.com/en/graphql/reference/pulls))
- Add to `PR`: `HasUnresolvedReviewThreads`, `Conflicting`, `HasNonApprovingReview`
- Derive them in `prWithReviewers`:
  - `HasUnresolvedReviewThreads`: any node with `isResolved` false
  - `Conflicting`: `mergeable == "CONFLICTING"`. `UNKNOWN` and `MERGEABLE` both give false
  - `HasNonApprovingReview`: any submitted review whose state is not `APPROVED`, reusing the
    existing `submittedReviews` filter, which already drops `PENDING` and bot authors
- `UNKNOWN` means GitHub is still computing mergeability ([MergeableState
  enum](https://docs.github.com/en/graphql/reference/pulls)). Reading it as not conflicting
  drops a PR into the review queue rather than parking it under its author, and the next run
  corrects it. No polling
- `isOutdated` is not selected: an unresolved thread blocks whether or not newer commits moved it
- `reviewDecision` is not used. It is null on an approved PR in a repository without
  required-reviewer rules, which makes it unusable as an approval signal ([community discussion
  24375](https://github.com/orgs/community/discussions/24375))
- Both fragments get the selection so the two fetch paths return the same enrichment. See
  [Why both fetch fragments](#why-both-fetch-fragments)
- Add `pull-requests: read` to the `reminder` job in `.github/workflows/pr-reminder.yml`, which
  grants only `contents: read` and `actions: read` today, below what `README.md` documents as
  required. Step 4's run is what verifies it
- Cost stays 1 per batch, taking a 25-PR enrichment request from 5,000 to 7,500 requested nodes
  against a 500,000 cap ([rate and node
  limits](https://docs.github.com/en/graphql/overview/rate-limits-and-node-limits-for-the-graphql-api))
- Update `githubclient.spec.md`, including two oddities:
  - A PR with over 100 review threads is judged on the first 100, so an unresolved thread past
    that is missed
  - A PR left without reviewer info by a failed enrichment gets all three flags false

### Step 2: add the bucketing rule

Files: `internal/prparser/`

- Add `PRTurn` with `TurnReadyToMerge`, `TurnWaitingForAuthor`, `TurnWaitingForReview`
- Add `GetPRTurn(pr PR) PRTurn`, implementing the four ordered checks from the target shape
- Read the three new fields off the embedded `*githubclient.PR`, rather than mirroring them
  onto `prparser.PR` the way `Approvers` and `Commenters` are: those carry Slack IDs, these
  don't
- Return a turn, not three slices: each consumer filters for itself, so `messagecontent` can
  call it later with no change here. See [Why a turn per PR](#why-a-turn-per-pr)
- Do not compute a latest-review-per-author state. A reviewer who commented and then approved
  lands in `Approvers`, and check 2 fires before `HasNonApprovingReview` is read
- Table-driven tests over the bucket matrix: one case per check, plus the overlaps where an
  earlier check has to win
- Update `prparser.spec.md`, including the oddity this rule inherits: `githubclient` counts a
  user with any `APPROVED` review as an approver, so a PR approved and then changes-requested by
  the same person, with every thread resolved, files as Ready to merge

### Step 3: bucket and render the three sections

Files: `internal/canvascontent/`, `internal/canvasbuilder/`, `cmd/pr-slack-reminder/`, `README.md`

Bucketing and rendering land together: replacing `Content.Open` breaks
`canvasbuilder.BuildMarkdown` and `canvas_internal_test.go`, which both read it.

In `canvascontent`:

- Replace the `Open` section with `ReadyToMerge`, `WaitingForAuthor` and `WaitingForReview`,
  filled via `utilities.Filter` on `prparser.GetPRTurn`
- Bucket after the existing oldest-first sort, so each bucket stays oldest first
- Group each bucket independently with `GroupPRsByRepositoriesInGivenOrder` when
  `GroupByRepository` is on, as the single `Open` section does today
- Keep `OpenPRsCapped` as one flag over all three buckets: it reports what the fetch capped,
  and the fetch does not know about buckets
- Extend the existing count log line to name the three bucket sizes
- Drafts and merged PRs are untouched: they never reach `GetPRTurn`

In `canvasbuilder`:

- Add `hideWhenEmpty bool` to `section`; `renderSectionBlocks` returns nil for an empty section
  carrying it, which `BuildMarkdown`'s existing `append` drops
  - Return nil, never `[]string{""}`, or `strings.Join` leaves a blank block
- Add the three headings and replace the single open `section` literal with three, all keeping
  `renderOpenPRRow` and all setting `hideWhenEmpty`
- Render `## Open` with `_No open PRs_` only when all three buckets are empty
- Correct the `renderSection` doc comment, which currently states an empty section keeps its
  heading

Tests and docs:

- Re-record the golden files with `make update-test-snapshots`, adding cases for a partly
  hidden set, a fully hidden set, and all three grouped by repository
- The zero-state assertions in `canvascontent` and `cmd/pr-slack-reminder` encode the old
  single section, so they change meaning rather than only being re-recorded
- The snapshot target regenerates but never deletes, so drop any golden file a renamed case
  leaves behind
- Update the canvas example in `README.md`, plus `canvascontent.spec.md` and
  `canvasbuilder.spec.md`

### Step 4: verify against the live canvas

- No test covers this: it runs the real GraphQL selection against a real token, which no mock
  can stand in for
- Run the `pr-reminder` workflow against this repository, with Step 1's `pull-requests: read`
  in place
- Done means a PR carrying an unresolved review thread renders under `## Waiting for author`,
  and one approved with every thread resolved renders under `## Ready to merge`
- This repository is public, so `GITHUB_TOKEN` reads its PR data whatever the `permissions:`
  block lists. The run confirms the selection and the bucketing, not the permission
- Record the outcome in `docs/third-party-facts.md`, against the existing entry saying no GitHub
  page documents the permission for any GraphQL field. Closing that gap needs a private
  repository under a fine-grained PAT

## Consequences

### Positive

- A reader picks a bucket by what they can do, instead of reading every row
- `GetPRTurn` is the whole rule in one function, so the reminder message adopts it by filtering

### Negative

None.

### Caveats

- Grouped mode multiplies sub-headings: three buckets over two repositories can render six
  `###` headings before `## WIP`. Nothing dedupes a repository across sections
- A PR moving between buckets changes the rendered markdown, so `canvasContentHash` differs and
  the canvas is rewritten. That is a real change worth showing, but it raises write frequency
  against [004](004_reduce-PR-tracker-canvas-writes.md)
- A `Ready to merge` row still names its approvers in the `(✅ a)` segment, repeating the heading
- Buckets are only as good as the team resolving threads. A team that never clicks Resolve gets
  approved PRs in `Ready to merge` with nits outstanding

### Neutral

- The fetch gains one nested connection and no rate-limit cost

## Justification

### Why a section type first

Three buckets take `Content` from one flat plus one grouped field to three plus three, leaving
15 fields of which 10 are section shapes. `PRSection` names the pairing that already exists, so
the feature step adds three fields instead of six, and every `Content{...}` literal in the tests
reads as sections rather than as a field list.

The rendering concerns stay out of it: `PRSection` carries data only, and `canvasbuilder` keeps
supplying the heading, row renderer and empty text per section.

### Why both fetch fragments

`GetPRs` feeds the reminder message and does not need the new fields today. Giving it the same
selection keeps both fetch paths returning one enrichment shape. The alternative leaves a trap:
adopting buckets for the message later would silently bucket every PR on zero-valued fields,
and nothing would fail.

### Why a turn per PR

`GetPRTurn` returning one value keeps the rule in one place and leaves bucketing to the caller.
A `BucketPRsByTurn` returning three slices would fix the section set in `prparser`, and
`messagecontent` may well want two buckets or a different order.
