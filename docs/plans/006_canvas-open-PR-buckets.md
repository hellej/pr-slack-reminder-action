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

1. At least one approval, no unresolved review threads, not conflicting → Ready to merge
2. Any approval (which check 1 has already turned down), any non-approving review, or any
   unresolved review thread → Waiting for author
3. Otherwise → Waiting for review

A thread counts as unresolved only when its last comment is not the PR author's: a reply hands
it back to the reviewer, and an author's own note on their diff never blocks. See
[Why the last commenter decides](#why-the-last-commenter-decides).

Conflicts demote rather than promote. A conflicting PR cannot be merged, so it never reaches
Ready to merge, but an unreviewed one falls to check 3 and stays in the review queue: a conflict
is usually small, and reviewing around it is not wasted. See [Why conflicts only
demote](#why-conflicts-only-demote).

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
| `HasUnresolvedReviewThreads` | `reviewThreads(first: 100){ nodes { isResolved comments(last: 1){ nodes { author { login } } } } }` |
| `Conflicting` | `mergeable`, `CONFLICTING` only |
| `HasNonApprovingReview` | the already-selected `reviews.nodes[].state` |

No `action.yml` change, and no new token permission: `README.md` already documents
`pull-requests: read`, which covers both additions. The REST endpoint carrying `mergeable`
needs Pull requests read or Contents read, either one ([REST get a pull
request](https://docs.github.com/en/rest/pulls/pulls?apiVersion=2022-11-28#get-a-pull-request)),
and the REST equivalents of `reviewThreads` sit under Pull requests at read with no alternative
([permissions for fine-grained PATs](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens)).
This repository's own `pr-reminder` workflow grants less than the README documents. Step 1
closes the `pull-requests` half, leaving `issues: read` still missing.

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
  - `reviewThreads(first: 100){ nodes { isResolved comments(last: 1){ nodes { author { login } } } } }`,
    100 being the page maximum, measured as `first: 101` returning `EXCESSIVE_PAGINATION`. The
    thread carries no author of its own, so the last comment supplies it
- Add to `PR`: `HasUnresolvedReviewThreads`, `Conflicting`, `HasNonApprovingReview`
- Derive them in `prWithReviewers`:
  - `HasUnresolvedReviewThreads`: any node with `isResolved` false whose last comment is not
    the PR author's. A thread with no comments, or one whose author is null, counts as blocking
  - `Conflicting`: `mergeable == "CONFLICTING"`
  - `HasNonApprovingReview`: any `COMMENTED` or `CHANGES_REQUESTED` review not by the PR author.
    Naming those two states leaves out `DISMISSED`, which no longer blocks
  - Both author comparisons run through `collaboratorFromAuthorNode` on each side, as
    `deriveReviewers` does. See [Why author comparison needs care](#why-author-comparison-needs-care)
- Extend `logEnrichment` to name each PR's returned thread count and `mergeable` value beside
  the review and comment counts. It runs before `prWithReviewers`, so it logs the raw selection
  rather than the derived flags
- `UNKNOWN` is GitHub still computing mergeability ([MergeableState
  enum](https://docs.github.com/en/graphql/reference/pulls)), read as not conflicting
- `isOutdated` is not selected: an unresolved thread blocks whether or not newer commits moved it
- `reviewDecision` is not used. It is null on an approved PR in a repository without
  required-reviewer rules, which makes it unusable as an approval signal ([community discussion
  24375](https://github.com/orgs/community/discussions/24375))
- Both fragments get the selection so the two fetch paths return the same enrichment. See
  [Why both fetch fragments](#why-both-fetch-fragments)
- Add `pull-requests: read` to the `reminder` job in `.github/workflows/pr-reminder.yml`, which
  grants only `contents: read` and `actions: read` today, below what `README.md` documents as
  required. Done when the next workflow run is green
- Measured at 25 aliases, `rateLimit.cost` stays 1 with both additions, and the request stays
  an order of magnitude under the 500,000 node cap ([rate and node
  limits](https://docs.github.com/en/graphql/overview/rate-limits-and-node-limits-for-the-graphql-api))
- Update `githubclient.spec.md`, including two oddities:
  - A PR with over 100 review threads is judged on the first 100, so an unresolved thread past
    that is missed
  - A PR left without reviewer info by a failed enrichment gets all three flags false

### Step 2: add the bucketing rule

Files: `internal/prparser/`

- Add `PRTurn` with `TurnReadyToMerge`, `TurnWaitingForAuthor`, `TurnWaitingForReview`
- Add `GetPRTurn(pr PR) PRTurn`, implementing the three ordered checks from the target shape
- Read the three new fields off the embedded `*githubclient.PR`, rather than mirroring them
  onto `prparser.PR` the way `Approvers` and `Commenters` are: those carry Slack IDs, these
  don't
- Return a turn, not three slices: each consumer filters for itself, so `messagecontent` can
  call it later with no change here. See [Why a turn per PR](#why-a-turn-per-pr)
- Do not compute a latest-review-per-author state. A reviewer who commented and then approved
  lands in `Approvers`, and check 1 fires before `HasNonApprovingReview` is read
- Table-driven tests over the bucket matrix: one case per check, plus the overlaps where an
  earlier check has to win
- Update `prparser.spec.md`, including the oddity this rule inherits: `githubclient` counts a
  user with any `APPROVED` review as an approver, so a PR approved and then changes-requested by
  the same person, with every thread resolved and no conflict, files as Ready to merge

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

- Re-record the golden files with `make update-test-snapshots`, ~3 new cases covering hidden
  buckets and grouped mode
- Grow the two existing whole-canvas cases, `flat open PRs and WIP PRs` and `all sections
  grouped by repository`, past what their assertions need: ~4 Ready to merge, ~4 Waiting for
  author, ~6 Waiting for review over three repositories. Their golden files exist to be read:
  three `##` headings at a real day's volume, and what grouped mode's `###` count costs
  - Vary approver and commenter counts so the `(✅ a / 💬 b)` segment renders at both widths
  - No new helper or `prOptions` field: these cases fill the bucket fields directly, never
    `GetPRTurn`
  - Read both golden files before Step 4
- `TestPostModeCanvasIsRefreshedWhenNoPRsAreFound` asserts the all-empty rendering, which this
  change leaves identical. It must keep passing untouched
- The snapshot target regenerates but never deletes, so drop any golden file a renamed case
  leaves behind
- Update the canvas example in `README.md` and the ordering paragraph under it, the only place
  a user learns how the canvas is ordered
- Update `canvasbuilder.spec.md`, and `canvascontent.spec.md`, whose **Doesn't Do** says
  "Doesn't filter or re-sort the open section"

### Step 4: verify against the live canvas

- No test covers this: it runs the real GraphQL selection against a real token, which no mock
  can stand in for
- Leave an inline comment on your own PR first, so the run has a thread to report
- Dispatch `pr-reminder` from the feature branch with `run-mode: post` and `build-first: true`.
  Its other triggers run the committed `dist/` binary, which `invoke-binary.js` pins by version,
  so they would go green without ever executing the new selection
- Read the thread count and `mergeable` value off the log line Step 1 adds, not off a bucket: an
  unreviewed conflicting PR renders in `## Waiting for review` whether `mergeable` said
  `CONFLICTING` or `UNKNOWN`
- Done means at least one PR logs a nonzero thread count alongside a `mergeable` value, and
  every open PR renders under the bucket its state predicts
- Your own commented PR keeping its place in `## Waiting for review` is what proves the
  `comments(last: 1)` subselection resolved and matched the author: an empty one counts a
  thread with no comments as blocking, which would move that PR to `## Waiting for author`
- Only `## Waiting for review` is reachable from one account. The other two need a second
  account to approve, since conflicts decide a bucket only once a PR is approved. Step 2's
  tests cover both
- Record the outcome in `docs/third-party-facts.md`, against the existing entry saying no GitHub
  page documents the permission for any GraphQL field. Closing that gap needs a private
  repository under a fine-grained PAT

## Consequences

### Positive

- A quiet day renders shorter: an empty bucket disappears instead of showing a fallback line

### Negative

None.

### Caveats

- Grouped mode multiplies sub-headings: three buckets over two repositories can render six
  `###` headings before `## WIP`. Nothing dedupes a repository across sections
- A PR moving between buckets changes the rendered markdown, so `canvasContentHash` differs and
  the canvas is rewritten. That is a real change worth showing, but it raises write frequency
  against [004](004_reduce-PR-tracker-canvas-writes.md)
- A `Ready to merge` row still names its approvers in the `(✅ a)` segment, repeating the heading
- A reviewer nit the author answered without fixing stops blocking, since the reply is what
  decides rather than the fix
- An unreviewed conflicting PR reads as ordinary in `## Waiting for review`: nothing on the row
  says a rebase is coming
- For one run, `Ready to merge` can name a PR that cannot be merged, when `mergeable` was still
  `UNKNOWN` at fetch time

### Neutral

None.

## Justification

### Why a section type first

Three buckets take `Content` from one flat plus one grouped field to three plus three, leaving
~15 fields, most of them section shapes. `PRSection` names the pairing that already exists, so
the feature step adds three fields instead of six, and every `Content{...}` literal in the tests
reads as sections rather than as a field list.

The rendering concerns stay out of it: `PRSection` carries data only, and `canvasbuilder` keeps
supplying the heading, row renderer and empty text per section.

### Why both fetch fragments

`GetPRs` feeds the reminder message and does not need the new fields today. Giving it the same
selection keeps both fetch paths returning one enrichment shape. The alternative leaves a trap:
adopting buckets for the message later would silently bucket every PR on zero-valued fields,
and nothing would fail.

### Why the last commenter decides

Reading a thread as open-or-closed produces two false positives on a team that approves with
nits: an author annotating their own diff, and a reviewer nit the author already replied to.
Both park a PR under Waiting for author until someone remembers to click Resolve.

The last comment's author separates them, for one nested connection and one comparison.

### Why author comparison needs care

Both new booleans rest on "not the PR author", and two things make that comparison easy to get
wrong:

- `submittedReviews` keeps the author. `deriveReviewers` removes them only later, so a new
  derivation reading `submittedReviews` directly sees the author's own reviews
- `collaboratorFromAuthorNode` appends `[bot]` to a bot login, so a raw node login never equals
  `Author.Login` on a bot's PR. Comparing raw logins makes a bot's own reply block its own PR

It matters because a bare inline diff comment arrives as a `COMMENTED` review, confirmed in this
repo by PR #58 and workflow run 34025576473. Authors comment on their own diffs routinely.

### Why conflicts only demote

Treating a conflict as "waiting for author" pulls the PR out of the review queue, and most
conflicts are a small rebase. The review can happen in parallel, so the cost of holding it back
is a reviewer who never looks rather than a reviewer whose work is wasted.

It still blocks Ready to merge, because a conflicting PR cannot be merged: the bucket would be
claiming an action nobody can take.

### Why a turn per PR

`GetPRTurn` returning one value keeps the rule in one place and leaves bucketing to the caller.
A `BucketPRsByTurn` returning three slices would fix the section set in `prparser`, and
`messagecontent` may well want two buckets or a different order.
