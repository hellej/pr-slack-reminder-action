# Message sections by next action

date: 2026-09-19
status: draft

## Goals

- The channel message groups PRs into the same next-action sections as the PR tracker canvas:
  ready to merge, waiting for author, waiting for review
- The message shows what is open right now, in both run modes, not only the PRs it was posted
  with
- The message gains a Merged section, so the team sees what landed without opening the canvas
- The Merged section never drops a PR the message was posted with, even past the cap
- An update run re-reads the PRs it was posted with only when neither fetch resolves them
- The message carries a footer saying it is live and when it was last written, in each reader's
  own timezone

Serves § **Purpose**: the reminder says who owes what and keeps pace with the day, instead of
freezing at 09:00 and making every reader parse every row. Sized for § **Reference Deployment**:
0 to 8 open PRs over three sections, plus a handful of merged rows.

### Non-goals

- No WIP (draft) section in the message. Drafts stay the canvas's job: a draft asks nothing
  of the channel
- No new action input
- No heading above the message. The four bold section headings say what each list holds
- No change to which PRs the filters and `/snooze` comments keep out of the open sections
- No change to the canvas, beyond its merged rows naming reviewers like every other row

## Target shape

```
*✅ Ready to merge*
  • title   2 days ago   by author (✅ reviewer)

*💬 Waiting for author*
  • title   🚨 5 days old   by author (💬 reviewer)

*👀 Waiting for review*
  • title   4 hours ago   by author

*🚀 Recently merged*
  • title   merged 3 hours ago   by author (✅ reviewer)

_Live, updated 15:12_
```

When grouped by repository, each section carries a repository sub-heading, linking to that
repository's pulls page as it does today, one level under the section heading, not bolded like
the heading above it.

An empty section renders nothing, Merged included. See
[§ Why the message hides empty sections while the canvas keeps them](#why-the-message-hides-empty-sections-while-the-canvas-keeps-them).

### What each section lists

Both run modes read the same two fetches: the open PRs as they are right now, and the recently
merged ones. Update mode adds a third read when the first two leave a state-artifact PR
unresolved: only that residue fetch can reach a PR either one missed. See
[§ Why merged PRs come from two places instead of one fetch](#why-merged-prs-come-from-two-places-instead-of-one-fetch).

- The three open sections list every currently open, non-draft PR the filters allow, bucketed by
  `prview.PR.GetNextAction()`. In update mode that is a live list, so a PR opened after the post
  appears in the message, and a PR that closed since disappears from it
- The Merged section lists every state-artifact PR that has since merged, however long ago, plus
  the newest 3 PRs from the recently-merged fetch that aren't already state-artifact PRs
- Closed-but-not-merged PRs reach no section

The state artifact keeps holding the PRs the post was made with, written once by the `post` run.
Update mode never rewrites that list, so "was open when the message was posted" stays the set it
names. The merged PRs a post picks off the fetch are not recorded there.

### Action inputs

- `pr-list-heading` is removed, along with its `<pr_count>` placeholder, and nothing replaces it:
  the message opens with its first section heading. A count at the top means nothing once the
  rows split across four sections, and each section heading already names what its rows are
- `no-prs-message` keeps its name and changes where it renders: a line above the sections
  whenever the message has no open PRs, rather than the whole message body
- No new required GitHub token permission or Slack scope. The recently-merged fetch is the
  GraphQL `search` the canvas already makes, and it now runs with the canvas disabled too

## Breaking or non-breaking

**Major**, released as **v3.0.0** off v2.3.0. What a v2 user sees change:

- The message is one bulleted list under one heading; it becomes four next-action sections, and
  merged PRs move out of those rows into a section of their own
- An `update` run listed only the PRs the message was posted with; it now lists what is open,
  including PRs opened since
- `no-prs-message` renders as a line above the sections instead of as the whole message
- When grouped by repository, repositories were ordered alphabetically before; each section now
  orders them by its own PR order, led by the repository holding its first PR
- The repository sub-heading was bold before; it no longer is, so it doesn't compete with the
  bold section heading above it
- `pr-list-heading` is removed, and no heading takes its place
- A snoozed PR that then merged was hidden from the message; it now appears in the Merged section.
  A snooze still hides an open PR
- Canvas merged rows name their approvers and commenters, where they named only the author
- The message ends in a footer line naming when it was last written

`pr-list-heading` is the only input removed. A v2 workflow that still sets it runs on v3, but the
runner warns on every run: `##[warning]Unexpected input 'pr-list-heading', valid inputs are [...]`
([actions/runner#514](https://github.com/actions/runner/issues/514)). So the release notes have to
tell users to delete the input, not just that it stopped working.

The release itself is a separate run of the [release skill](../../.agents/skills/release/SKILL.md),
not a step here. The work lands on a feature branch and a PR, so the live Slack checks and the
README screenshots run off that branch.

## Steps

- **R1** — hoist both PR fetches and the run timestamp into `Run`
- **1** — remove the `pr-list-heading` input
- **2** — `messagecontent` builds four sections
- **3** — `messagebuilder` renders the four sections and the live footer
- **4** — both run modes render the live open PRs and the merged ones, and the send/keep/delete
  rules follow the sections
- **5** — merged PRs carry their reviewers, on both surfaces
- **6** — update mode re-fetches only the state PRs the two fetches leave unresolved
- **7** — delete what the change left unreachable

## R1. Hoist both PR fetches and the run timestamp into `Run`

Touches `cmd/pr-slack-reminder` (`run.go`, `canvas.go`), `testhelpers/mockgithubclient`.

- `Run` performs, before the run-mode switch:
  - `generatedAt := time.Now().UTC()`, today the first line of `refreshPRTrackerCanvas`
  - `findOpenPRs` with `IncludeDrafts: cfg.CanvasEnabled()`, today post mode's own call
  - `findRecentlyMergedPRs`, moved here from `canvas.go`
- `runPostMode` takes the open PRs instead of fetching them, and stops returning them
- `refreshPRTrackerCanvas` takes `openPRs`, `mergedPRs`, `mergedPRsErr` and `generatedAt` as
  parameters instead of producing any of them. Its `openPRs == nil` fetch branch goes away: no
  caller leaves them unfetched any more. It keeps returning `mergedPRsErr`, so a failed merged
  fetch still fails the run only when the canvas is enabled
- Neither function fetches any more, so both drop their `githubClient` parameter, and
  `findRecentlyMergedPRs` moves to `run.go` beside `findOpenPRs`
- Rewrite `refreshPRTrackerCanvas`'s doc comment: it no longer fetches the merged PRs, owns the
  run's "now", or has a nil-`openPRs` contract
- Rewrite `Run`'s block comment too. "Their errors are collected instead of short-circuited"
  would otherwise sit above code that short-circuits: the message path and the canvas refresh stay
  independent attempts, but they now share one fetch as a precondition
- Three behaviour changes fall out, all intended:
  - A canvas-disabled run makes one GraphQL `search` for merged PRs it does not make today, whose
    failure is only logged
  - A canvas-disabled `update` run makes an open-PR fetch where it makes none today: `findOpenPRs`
    has one caller, the `openPRs == nil` branch of `refreshPRTrackerCanvas`, which `Run` gates on
    `cfg.CanvasEnabled()`. Update runs are event-driven, so this is a `FindOpenPRs` across every
    configured repository on every PR, review, comment and push event. Step 4 is what consumes it
  - A failed open-PR fetch now fails the message and the canvas together. Today the canvas
    retries the same query seconds later, a retry that serves no real scenario
- A failed open-PR fetch returns from `Run` before the run-mode switch, so no message is sent,
  edited or deleted, no canvas is written and no state is saved. That is the shape `runPostMode`
  and `state.Load` failures already take, and it is what keeps a fetch outage from deleting a
  message that update mode cannot rebuild
- Update mode passes the open PRs straight to the canvas here, and step 4 is what makes its
  message read them
- No message output changes. `canvas_test.go` and the snapshot tests are the regression net,
  except for three of its tests that pin behaviour this step reverses: the post-mode fetch
  failure no longer reports a second, canvas-scoped failure; the update-mode fetch failure no
  longer updates the message, which turns that test into the failed-fetch case over both canvas
  settings; and the seeded-hash test loses its fetch-failure row, since such a run now saves no
  state at all
- `mockgithubclient` gains a `FetchRecording`, counting the open and merged fetches a run made.
  With the canvas off, those fetches reach no output yet, so the request is all a test can assert
  on

## 1. Remove the `pr-list-heading` input

Touches `action.yml`, `internal/config`, `internal/canvascontent`, `internal/messagecontent`,
`cmd/pr-slack-reminder`, `testhelpers`, `Makefile`, `.envrc-example`, `README.md`.

- Delete the `pr-list-heading` block from `action.yml`, the `InputPRListHeading` constant and the
  `ContentInputs.PRListHeading` field, and `Config.validateHeadingOptions` with its call
- `messagecontent.GetContent` reads the field through `formatListHeading`, and `main_test.go`
  asserts the validation error this step deletes and substitutes `<pr_count>` in its expected
  message. Nothing compiles between the field's removal and step 2's rewrite, so steps 1 and 2
  land as one commit
- `go run .github/scripts/check_inputs.go` is what proves `action.yml` and the config constants
  stay in sync after the removal: it checks both directions, so a leftover constant fails it
- Drop the field from `testhelpers.TestConfig` and `SetTestEnvironment` too, and from the test
  configs that set it
- Drop the `INPUT_PR-LIST-HEADING` line from the `make run` env list in the `Makefile`, and the
  `INPUT_PR_LIST_HEADING` export from `.envrc-example`, which is what a contributor copies
- Edit `canvascontent.spec.md`'s "Doesn't read `PRListHeading` or `NoPRsMessage`" line: it names a
  field and a `<pr_count>` substitution that no longer exist. The canvas's behaviour is unchanged
- Reword the `group-by-repository` description in `action.yml` and `README.md`: it no longer makes
  `pr-list-heading` ignored
- README: drop the `pr-list-heading` input-table row and its use in the usage examples, drop it
  from the canvas § **Good to know** bullet listing the inputs the canvas ignores, and reword the
  § **Tips** "Customize messages" line, which `no-prs-message` alone now carries
- Update `config.spec.md`

## 2. `messagecontent` builds four sections

Touches `internal/messagecontent`.

- `GetContent(openPRs, trackedPRs, recentlyMergedPRs, generatedAt, contentInputs)` replaces
  `GetContent(openPRs, contentInputs)`
  - `openPRs` are the non-draft PRs of the run's open fetch, already draft-filtered by the caller
    as post mode filters them today. This package does no state filtering on them: everything in
    the list is open
  - `trackedPRs` are the state artifact's PRs as re-fetched, in whatever state they are now. Only
    the merged ones are read, so a caller resolving some of them elsewhere may pass fewer. Empty
    in post mode
- `Content` loses `PRs`, `PRsGroupedByRepository` and `PRListHeading`, and gains:
  - `ReadyToMerge`, `WaitingForAuthor`, `WaitingForReview`, `Merged`, each a `PRSection`
  - `NoOpenPRsText`, the configured `no-prs-message`, set only when no open PR is listed
  - `GeneratedAt`, the run timestamp R1 hoists, passed through for the footer. `canvascontent`
    carries the same field for the same reason
  - No heading field: the message has no heading, and `formatListHeading` goes with the
    placeholder it substituted
  - `GroupedByRepository` stays
- `PRSection` is new, mirroring `canvascontent.PRSection`: a flat `PRs` list, or `Groups` as
  `[]PRsOfRepository` when grouping is on, never both. `PRsOfRepository` keeps its link label and
  link, built as today from `models.Repository.GetPullsURL`, and loses `HeadingPrefix` (step 3)
- Every section groups through `prview.GroupPRsByRepositoriesInGivenOrder`, the canvas's function,
  over the already-sorted list. Today's `GroupPRsByRepositories` orders repositories
  alphabetically, which throws away the sort each section just made: the repository holding the
  oldest PR, or the newest merge, leads its section instead
  - Point `canvasbuilder_test.go`'s ~3 fixture uses at the in-given-order function too. That
    leaves `GroupPRsByRepositories` with no caller outside its own test, for step 7
  - One canvas markdown golden changes with them, the two-repository grouped one, whose sections
    swap order. Test data only: `canvascontent` already groups in given order, and the fixture was
    the odd one out. `make update-test-snapshots` re-records
    `internal/canvasbuilder/testdata/` too
- Open sections: `openPRs` sorted oldest to newest as today, then filtered into the three sections
  by `prview.PR.GetNextAction()`, mirroring `canvascontent`'s `includePRsWhoseNextActionIs`. Each
  section keeps that order
- Merged section: every `prview.PR.IsMerged` PR in `trackedPRs`, plus the newest
  `MaxUntrackedMergedPRs` entries of `recentlyMergedPRs` not already in that first set, the whole
  result sorted newest merge first by `prview.SortPRsNewestFirst` over `GetMergedAt`, as
  `canvascontent` sorts its own merged list
  - That sort runs on `recentlyMergedPRs` before the cap too, so the message's own cap of 3 does
    not rest on another package's ordering guarantee
  - `MaxUntrackedMergedPRs = 3` is new in this package, with a comment saying the merged PRs the
    message already tracks are never counted against it
  - The membership test is a `map[models.PullRequestRef]bool` over
    `models.PullRequestRef{Repository: pr.Repository, Number: pr.GetNumber()}`, built here rather
    than through `state.PRToPullRequestRef`: the content layer has no reason to import the
    persistence package
- Closed-but-not-merged PRs in `trackedPRs` reach no section, so they leave the message at the
  next update
- `SummaryText`, the Slack fallback text, is `"N open PRs are waiting for attention 👀"` as today
  when an open PR is listed, and the fixed `"Nothing waiting for review 🎉"` otherwise. It is
  never empty, so it cannot gate whether a message is sent
- `HasPRs()` reports whether any of the four sections has a PR
- Log the count each section ended up with, as `canvascontent` does
- Update `messagecontent.spec.md`

## 3. `messagebuilder` renders the four sections and the live footer

Touches `internal/messagebuilder`, `cmd/pr-slack-reminder`, `testhelpers/mockslackclient`,
`testhelpers/mockgithubclient`.

- The message renders no heading block. Its first block is its first non-empty section
- Each non-empty section renders as one `slack.NewRichTextBlock` holding, in order: a bold heading
  section, then its rows. When grouped by repository, the rows are an unbold repository
  sub-heading section and a bullet list per repository, all inside that same block
  - One block per section, instead of today's block per repository, keeps the block count
    independent of repository count: a full four-section message is 8 blocks whatever it lists,
    footer included
  - A `rich_text` block's `elements` is an array mixing `rich_text_section` and `rich_text_list`
    freely, with no documented cap on element count
    ([rich text block](https://docs.slack.dev/reference/block-kit/blocks/rich-text-block);
    `slack-go@v0.29.0/block_rich_text.go`). How Slack spaces them is only checkable live: verify with
    `gh workflow run pr-reminder.yml --ref <branch> -f run-mode=post -f build-first=true`
- The section heading strings are this package's: `✅ Ready to merge`, `💬 Waiting for author`,
  `👀 Waiting for review`, `🚀 Recently merged`. The first three match the canvas wording; the
  merged one does not, since the message caps that section at 3 untracked PRs. None of the four
  are derived from `PRNextAction`: it is an identifier, not display text
- `HeadingPrefix` ("Open PRs in ") is dropped from the repository sub-heading: the section heading
  above already says what the rows are
- The row renderer loses its two state branches: the trailing `🚀` for `pr.IsMerged()` and
  `Strike: pr.IsClosedButNotMerged()` on the title link. Its rows are always open now, so neither
  can fire, and `make check-dead-code` checks function reachability under `./cmd/...`, not
  branches. With a second renderer beside it, it is renamed `buildOpenPRBulletPoint`
  - That leaves `prview.PR.IsClosedButNotMerged` with no caller, for step 7
  - `messagebuilder.spec.md`'s row line loses "struck through if closed-but-not-merged" and
    "a rocket marker if merged" with them
- Merged rows get their own renderer, `buildMergedPRBulletPoint`: linked title, then
  `prview.PR.GetMergedText()` in italics, then the author, then the reviewer segments
  `GetReviewersTextSegments` returns. No age text, no `🚨` old-PR marker and no trailing `🚀`: the
  section heading carries the rocket. An unknown merge time only drops that segment
  - Readers need the reviewer segments to see who got the PR landed, and the row already carries
    them today. A PR with no reviewers renders none
- `Content.NoOpenPRsText` renders as a plain rich-text line above the sections, when set, with no
  spacing block under it. `addNoPRsBlock` and `BuildMessage`'s `!content.HasPRs()` early return go
  away: a message with no section renders that line and the footer
- Both run modes' send/keep/delete test moves off `content.SummaryText` onto
  `content.NoOpenPRsText` here rather than in step 4, since step 2 makes `SummaryText` never
  empty and the old test would stop firing the moment it lands
- `mockslackclient`'s block reader is rewritten with the layout: a section block now holds its
  heading and its lists together, where it used to read a heading block and a list block
- `mockgithubclient` selects `mergedAt` on the `GetPRs` path, which the real query already
  selects and no rendering read before this step
- One spacing block between rendered sections. That replaces today's between-repository spacing
  block
- The footer is the message's last block, always rendered: a `context` block holding one `mrkdwn`
  element, `_Live, updated <!date^<unix seconds>^{time}|<HH:MM> UTC>_`, built with
  `slack.NewContextBlock("", slack.NewTextBlockObject("mrkdwn", footerText, false, false))`
  (`slack-go@v0.29.0/block_context.go`). The unix seconds and the fallback both come from
  `Content.GeneratedAt`
  - `<!date^…>` renders the time in the reader's own timezone, and the pipe fallback is what shows
    if the client cannot process it
    ([formatting message text](https://docs.slack.dev/messaging/formatting-message-text))
  - `{time}` renders 12-hour or 24-hour by each reader's own Slack client setting. No token forces
    one (same page), so the fallback carries UTC
  - A `context` block renders smaller and greyer than a `rich_text` line, so the footer doesn't
    read as a fifth section. Verified live against the dev channel, wrapped in `_` for italics,
    with no divider above it
  - The no-PRs message carries it too
- `BuildMessage` returns `content.SummaryText` as the fallback text, unchanged
- The 50-block cap and its log line stay as a safety net, now over the content blocks alone: the
  footer is appended after the cap, so it keeps the last slot whatever was dropped. Rewrite
  `maximumBlocksInSlackMessage`'s comment: its 16-repositories × 3-blocks derivation describes a
  layout this step removes
- `assertSentBlocksMatchSnapshot` replaces the footer's unix seconds and UTC fallback with fixed
  placeholders before it compares or records, since `Run` stamps the real clock and a snapshot
  recorded a second ago would otherwise fail. The rest of the snapshot already survives a moving
  clock: `main_test.go`'s `var now = time.Now()` sets the fixture ages, so the rendered "6 hours
  ago" stays put
- Update `messagebuilder.spec.md`, and re-record the snapshots with `make update-test-snapshots`.
  Expect every file in `cmd/pr-slack-reminder/testdata/snapshots/` to change
- Two `githubclient.spec.md` oddities describe the marker and the strike-through this step deletes:
  "`MergedAt` is filled on the `GetPRs` path too, where nothing reads it yet. The reminder
  message's merged marker comes from `Merged`" and "`GetPRs` renders any state other than `CLOSED`
  or `MERGED` as open, so an unexpected or missing state doesn't strike the PR through in the
  reminder". Both need rewriting here, where the rendering changes

## 4. Both run modes render the live open PRs and the merged ones

Touches `cmd/pr-slack-reminder/run.go`, `action.yml`, `README.md`, `docs/examples/`.

- Post mode passes R1's open PRs with drafts filtered out, no tracked PRs, and R1's merged PRs to
  `messagecontent.GetContent`. The draft filter and the view building move into
  `buildNonDraftPRViews`, which both modes call
- Both modes pass R1's `generatedAt` to it, so a post stamps the footer with the time it posted and
  an update with the time it edited
- Update mode keeps its `GetPRs` call on `loadedState.PullRequests`, now for one purpose: those are
  the tracked PRs the Merged section reads. It moves behind `getTrackedPRs`, and its open sections
  come from R1's fetch, draft-filtered by the same predicate post mode uses
  - Drop the `len(loadedState.PullRequests) == 0` early return. A message posted with no PRs still
    has live ones to show. `getTrackedPRs` skips the call on an empty ref slice rather than making
    it: `getPRsByRef` chunks the refs, so such a call sends no request but still logs a fetch
  - An empty-state run with nothing open and no `no-prs-message` therefore deletes the message,
    where it used to exit without touching it
- Both modes send or edit whenever `content.HasPRs() || content.NoOpenPRsText != ""`. `SummaryText`
  is no longer part of that test: step 2 always sets it. **Landed in step 3**, which is where
  `SummaryText` stopped being empty and the old test stopped firing
- Update mode deletes the message only when that test fails: no open PR, no merged PR, and no
  `no-prs-message`. Today the same test is over the re-fetched state PRs alone, merged and closed
  ones included, so the delete fires when that fetch comes back empty after filters and snooze
  - `runUpdateMode` takes `mergedPRsErr` alongside the merged PRs, as `refreshPRTrackerCanvas`
    already does, and skips the delete while it is non-nil. Otherwise a failed merged search would
    read as "nothing merged" and remove a message the run cannot rebuild, which
    [§ Why a failed tracked-PR fetch degrades the message instead of failing the run](#why-a-failed-tracked-pr-fetch-degrades-the-message-instead-of-failing-the-run)
    rules out. Such a run still edits the message whenever the send/keep test passes
  - Rewrite the comment above the merged fetch in `Run`: the merged PRs no longer cost the canvas
    alone, and the error now has a second reader
- Cover in `main_test.go`, one update run each:
  - Live open PRs that are not the state's
  - An empty state with a PR open
  - An empty state with nothing open, which deletes
  - A state PR merged an hour past `githubclient.RecentlyMergedWindow`. The merged fetch returns
    it and then drops it as out of window, so only the state path can surface it
  - A failed merged search with nothing else to show, which keeps the message
- The update-mode fixtures gain the live open fetch and the merged search
  - `TestUpdateModeCanvasShowsCurrentlyOpenPRs` and
    `TestUpdateModeFetchesOpenAndMergedPRsWithTheCanvasDisabled` already carry both, and both pin
    the state-only message this step replaces, so their message assertions turn around
  - The two delete cases get the live fetch too. Without it they delete because nothing was
    fetched, not because the filter or the draft rule dropped anything
- `no-prs-message` is documented as the whole message in two places, and is now a line above the
  sections: rewrite its `action.yml` description ("Message to send when there are no open PRs")
  and its README input-table row ("Message when no PRs are found (if not set, no empty message
  gets sent)"). A message with merged rows and no open PR is now sent whether or not the input is
  set, which the README row denies
- README § **3. Update Mode Enabled** carries two sentences this plan reverses: "PRs that were
  merged since the original message are shown with 🚀 emoji suffix" (step 3 moves them to a Merged
  section and drops the suffix) and "the updated message will not contain new PRs published since
  the original message" (it now does). Rewrite both, and README's opening line under the H1, which
  describes the message as "a list of PRs with (optional) highlighting for the old ones"
- README § **Example Output** gains a line under the screenshot for the footer: it says when the
  message was last written, and its clock is the reader's own, 12-hour or 24-hour by their Slack
  setting. That is the section showing the message's layout
- README pins `hellej/pr-slack-reminder-action@v1` in 4 usage examples, already stale at v2. Bump
  them to `@v3`
- Both README screenshots show the old layout: `docs/examples/example_1.png` under § **Example
  Output** and `docs/examples/example_2.png` under § **3. Update Mode Enabled**. Re-take both:
  run `gh workflow run pr-reminder.yml --ref <branch> -f run-mode=post -f build-first=true`, then
  the same with `-f run-mode=update`, against the dev channel, and screenshot each message. Done
  means both images show the four sections and the footer

## 5. Merged PRs carry their reviewers, on both surfaces

Touches `internal/apiclients/githubclient`, `internal/canvasbuilder`, `README.md`.

- `FindRecentlyMergedPRs` runs its capped results through `enrichPRsWithReviewInfo`, so a merged
  row names its approvers and commenters like every other row. `mergedPR` stays as the unenriched
  mapping
  - The search already produces the `[]PRResult` enrichment takes, so this reuses the second
    phase `FindOpenPRs` already runs, just on a second set of PRs
  - `MaxMergedPRsToFetch` (6) is far under `enrichBatchSize` (25), so this is one more GraphQL
    request on a run that fetches merged PRs, never more
- A failed enrichment falls back to `mergedPR` over the search results and logs it, rather than
  failing the fetch. That is today's merged row, and failing instead would let a reviewer query
  take down a canvas-enabled run. Per-PR failures already degrade on their own
  (`failsOnlyOnePullRequest`), so only repository-, query- or transport-scoped errors reach it
- Merged PRs are not snooze-excluded: a snooze suppresses a request for attention, and a merged row
  asks for nothing. `excludeSnoozedPRs` comes off the `GetPRs` path with them, so the same PR
  cannot show or hide depending on which fetch resolved it. After step 4 that path serves the
  Merged section alone, so nothing else sees the change
- Update `githubclient.spec.md` and `FindRecentlyMergedPRs`'s doc comment, which promises no
  reviewers and no snooze. The spec lines that go stale: "Merged PRs are never enriched", the
  snooze exclusion lines, "`FindRecentlyMergedPRs` searches in one request and stops there" with
  the batching sentence beside it, the § **Doesn't Do** line scoping per-PR degradation to
  `FindOpenPRs`, and "any error fails the whole merged fetch", which this step splits into the
  search and the enrichment
- `canvasbuilder.renderMergedPRRow` ends in `renderReviewers(pr.Approvers, pr.Commenters)`, the
  call `renderOpenPRRow` already makes, so a canvas merged row reads like the message's merged row
  - `canvasbuilder.spec.md`'s merged row line says "Never reviewers: the section answers what
    landed, not who reviewed it". That decision is reversed: with the data fetched, who got a PR
    landed is worth the same line on both surfaces
  - Re-record the canvas goldens with `make update-test-snapshots`. Every
    `internal/canvasbuilder/testdata/` file holding a merged row with reviewers changes
  - README § **PR Tracker Canvas** shows a rendered canvas whose merged rows come from the same
    fixtures. Add the reviewer segment there so the sample keeps matching what the code renders

## 6. Update mode re-fetches only the state PRs the two fetches leave unresolved

Touches `cmd/pr-slack-reminder/run.go`, `testhelpers/mockgithubclient`.

- `GetPRs` takes the state refs neither of R1's fetches returned, instead of all of them
  - A ref the open fetch returned is open right now, and an open tracked PR reaches no section.
    The open sections already render it from that same fetch
  - A ref the merged fetch returned is merged, and step 5 gives it the same reviewers `GetPRs`
    would, so the Merged section can take it straight from there
  - The residue is every other state ref: merged outside the 7-day window or past
    `MaxMergedPRsToFetch`, closed, filtered out, turned into a draft, or past the open fetch's cap.
    Absence from a fetch is read as "unresolved", never as "gone", so a capped or filtered fetch
    costs a read rather than a wrong row
  - An empty residue skips the call. That is the common case: a tracked PR stays resolved by the
    open fetch until it merges and by the merged fetch for the week after, and it subsumes step 4's
    empty-state case
  - The refs are compared as `models.PullRequestRef` values built off both fetches
- The tracked PRs handed to `messagecontent` are the merged fetch's matching entries plus whatever
  the residue call returned. A tracked PR resolved off that fetch has to arrive as tracked, or
  step 2's cap of 3 counts it as untracked and can drop the very row the Merged section promises
  to keep
- A failed residue fetch no longer stops the update. Log it, and build the content from the open
  PRs and the merged fetch alone. See
  [§ Why a failed tracked-PR fetch degrades the message instead of failing the run](#why-a-failed-tracked-pr-fetch-degrades-the-message-instead-of-failing-the-run)
  - Such a run never deletes the message, whatever step 4's send/keep/delete test says. Deleting
    needs proof that nothing is left to show, and a failed fetch is not proof. The run edits the
    message when that test passes, and leaves it standing when it does not
  - The state save is unchanged: update mode never rewrites the tracked PR list
- Cover in `main_test.go`: an update run whose state PRs are all still open and one where a state
  PR merged inside the window, both asserting no `GetPRs` request goes out; one where a state PR
  merged outside it, asserting the residue carries it into the Merged section with its reviewers;
  one whose residue fetch fails, asserting the message is edited and not deleted
  - `mockgithubclient` gains a recording of the refs each `GetPRs` request asked for, so a test
    can assert the call was skipped or narrowed. It already parses them off the query as
    `postedPullRequestRefs`, but `MakeMockGitHubClientGetter` builds the transport inside the
    closure and hands the test only a `githubclient.Client`, so the recording hangs off
    `MockGitHubClientOptions` as a pointer. `ErrByPRNumber` already fails the call, since a
    PR-level error fails the whole `GetPRs` query

## 7. Delete what the change left unreachable

Touches whatever `deadcode` names, and those packages' specs.

- Run `make check-dead-code` and delete what it reports, together with the tests that only existed
  to call it
- Steps 1 to 6 drop call sites without dropping everything they called. `GroupPRsByRepositories`
  and `prview.PR.IsClosedButNotMerged` are the two already known; the check is what finds the rest,
  which is cheaper than predicting them here
- `deadcode` walks from `./cmd/...`, so it reports a function its own test is the only caller of.
  That is a deletion, not something to keep alive
- Update each touched package's spec in the same commit. `prview.spec.md`'s "`IsMerged` and
  `IsClosedButNotMerged` expose PR state for display styling" line covers a predicate this plan
  leaves with one caller and no styling role, so it needs rewriting either way
- Verifying it done: `make check-dead-code` and `make test` both pass

## Consequences

### Positive

- One mental model for both surfaces: the same sections, in the same order, with the same wording
- The message tracks the day. A PR opened at 11:00 is in the 09:00 reminder by the next event-driven
  update, instead of waiting for tomorrow's post
- A PR the message asked the team to review is shown merged in that same message, so the reminder
  closes its own loop
- The three open sections split 0 to 8 rows into short lists, each naming who owes what
- The footer dates the message, so a reader scrolling back knows whether they are looking at this
  morning's list or a version of it from ten minutes ago
- One block per section makes the 50-block cap unreachable in practice, where grouped mode spends
  3 blocks per repository today
- `runPostMode` and `runUpdateMode` converge on one content call over the same fetches, leaving the
  state artifact responsible for the Slack message reference and the posted PR refs only
- Against steps 1-4, an update run whose state PRs are all still open, or merged within the week,
  drops from three reads to two, and a GitHub blip on the third no longer costs the whole update

### Negative

- None

### Caveats

- With the canvas off, a run gains PR reads: a post goes from one to two, an update from one to
  two or three. Update runs are event-driven, so every PR, review, comment and push event pays it
- With the canvas on, a run gains none. It already makes all three reads the message now shares:
  update mode hands the canvas no open PRs, so `refreshPRTrackerCanvas` fetches the open and
  merged lists itself, beside `GetPRs` on the state refs
- A failed merged fetch is logged only when the canvas is off, so such a run shows no untracked
  merged PRs and stays green
- Enriching the merged PRs adds one GraphQL request to every run that fetches them, the canvas's
  runs included
- A failed residue fetch drops the tracked merges the merged fetch does not reach, those outside
  the week, filtered, or past its cap. The next update brings them back
- A merged PR a post picked up from the untracked fetch, not the state artifact, can drop out
  of the message later the same day, once newer merges push it past the fetch's 6 newest. Only
  state-artifact PRs are held for good
- A team that set `pr-list-heading` loses its wording, and two instances posting to one channel
  lose their only distinguishing label. That case falls outside § **Reference Deployment** (one
  team, one channel), and this plan replaces `pr-list-heading` with nothing
- Putting a whole section in one block leaves per-block text length as the only remaining size
  limit, and `messagebuilder` checks no such limit today
- "Live" overpromises in two cases: yesterday's message keeps the footer once the next scheduled
  post supersedes it, and a workflow running `post` alone never edits the message it labels live.
  Both accepted: the timestamp next to "Live" is what a reader actually reads. The action sees
  its own `run-mode`, not whether update runs are configured, and § **Non-goals** rules out
  adding an input to declare that

### Neutral

- `no-prs-message` keeps its name while changing from the whole message body to one line in it
- The Merged section's cap of 3 untracked PRs is independent of the canvas's 6: both read the same
  fetch, and the canvas keeps showing more
- A 09:00 message and a 14:00 update of it can list different PRs. That is the point, but it means
  the channel's scrollback no longer records what was open at 09:00
- The footer's clock is 12-hour or 24-hour by each reader's own Slack setting, so two members can
  read the same message differently
- The message footer shows a local time where the canvas footer shows UTC. Canvas markdown renders
  `<!date^…>` as raw text (probed live against the dev channel canvas), so only the message can
  show a reader their own time

## Justification

### Why the message hides empty sections while the canvas keeps them

The canvas is a standing document: opening it at `## 🔧 WIP` would read as a broken render, so
every section it can show has a fixed place. The message is an interrupt that has to earn itself,
and a `_No merged PRs_` line in a channel is a row that asks for nothing. Hiding costs the reader
nothing, because the heading order is stable in the sections that do render.

### Why merged PRs come from two places instead of one fetch

The recently-merged fetch answers "what landed lately", capped and windowed for a standing view.
The message needs a second, narrower answer: "what happened to the PRs this message listed". Only
the state artifact knows that set, and a PR merged eight days ago falls outside the fetch's window
entirely. Taking the union, with the cap applied only to the fetch's half, keeps both answers
whole.

### Why a failed tracked-PR fetch degrades the message instead of failing the run

Update runs are event-driven, so a failing one leaves the message showing the last state it could
render, until the next event. Failing the run over the residue fetch trades a whole refresh, open
sections included, for the few merged rows that fetch carries. Rendering without them keeps the
message current and costs one run's worth of merged rows.

What makes it safe is that the degraded run cannot delete: without the residue, "no open PR and no
merged PR" is a fetch outage as much as an empty day, and only one of the two should remove a
message the run cannot rebuild.
