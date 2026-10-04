# Collapse bot PRs into one row per author

date: 2026-10-03
status: draft

## Requirements

A dependency bot's weekly batch adds ~5 rows at once, burying the human PRs in the message and on the canvas.

- In every section of the message and the canvas, 2 or more PRs by one author in `collapse-prs-from-authors` collapse into one **collapsed row**: `🤖 dependabot (3): #101 #102 #103`, each number linking to its PR
- New input `collapse-prs-from-authors`: a list of GitHub logins, default `dependabot[bot]` and `renovate[bot]`. Set to `""`, nothing collapses: see docs/third-party-facts.md § An action input's `default:` applies only when `with:` omits the key, so an explicit `""` stays empty
- One collapsed row per author: Dependabot and Renovate PRs never share a row
- When grouped by repository, each repository group gets its own collapsed rows, collapsing at 2 or more PRs by one author in that group
- A lone PR by a listed author keeps its full row
- Numbers carry no repository prefix, even when an ungrouped row spans repositories
- A collapsed row shows no age, author, reviewers or old-PR marker
- Non-goals:
  - Changing any cap, sort or bucket: collapsing happens after them
  - Changing `SummaryText`: it still counts every open PR
- Work lands on branch `collapse-bot-prs`, then a PR

Purpose: the reference deployment has 0 to 8 open PRs, so a 5 PR batch can be most of the message. One row per bot keeps the human PRs readable while still listing every bot PR.

## Target Shape

- New input `collapse-prs-from-authors`, optional, a list read with `inputhelpers.GetInputList` (`;` or newline separated). `action.yml` carries the default; Go applies none, as with `no-prs-message`
  - Logins match exactly, as `authors`, `ignored-authors` and the Slack user mapping do
  - `config` drops empty items, such as the one between `;;`, so no PR with an unknown author collapses
  - Both default logins confirmed: see docs/third-party-facts.md § GitHub App bot logins end in `[bot]`, and GraphQL drops the suffix
- No token permission or OAuth scope change
- `prview` owns the section model, shared by both content packages. Content packages fill it, builders render it as given:
  - `Row`: a sealed interface, implemented by `PR` and `CollapsedRow`. Builders type-switch on it
  - `CollapsedRow{AuthorLogin, PRs}`, with `GetAuthorLabel()`: the login without a trailing `[bot]`, and `GetSearchURL()`: a GitHub PR search for the author's open or merged PRs, per step 5
  - `PRSection{Rows, Groups}` and `RepositoryRows{Repository, Rows}`, replacing `messagecontent.PRSection`, `messagecontent.PRsOfRepository` and `canvascontent.PRSection`. A section fills `Rows` or `Groups`, never both, as today
  - `RowsCollapsingAuthors(prs, collapsePRsFromAuthors) []Row`: every PR not collapsed, in the given order, then one `CollapsedRow` per author with at least `MinPRsToCollapse` (2) PRs, in `collapsePRsFromAuthors` order. Each `CollapsedRow` keeps the given order of its PRs
- `prview.RepositoryPRs` and `GroupPRsByRepositoriesInGivenOrder` stay: content packages group PRs first, then turn each group's PRs into rows
- `messagebuilder` takes the repository name and pulls URL off `RepositoryRows.Repository`, as `canvasbuilder` already does

Message, ungrouped, as posted. An `update` edit adds the update-time footer; a message marked stale re-sends the stored blocks, collapsed rows included:

```
✅ Ready to merge
• Bump golang.org/x/net from 0.30.0 to 0.31.0  2 days ago by dependabot[bot] (✅ bob)
👀 Waiting for review
• Fix login redirect  3 days ago by @alice
• 🤖 dependabot (3): #101 #102 #103
• 🤖 renovate (2): #88 #90
🚀 Recently merged
• Add canvas footer  merged 2 hours ago by bob (✅ alice)
• 🤖 dependabot (2): #98 #97
```

- A collapsed row is one more `rich_text_section` in the section's existing bullet list: a bold link element `🤖 <label> (<count>)` to the search URL, a plain `: ` run, then one bold link element per number, joined by plain `" "` runs. No new block, so the block cap is unchanged

Message, grouped:

```
👀 Waiting for review
app
• Fix login redirect  3 days ago by @alice
• 🤖 dependabot (2): #101 #102

infra
• 🤖 dependabot (3): #7 #8 #9
```

Canvas, ungrouped:

```
## 👀 Waiting for review

- **[Fix login redirect](…)** _3 days ago_ by alice
- **[🤖 dependabot (3)](…)**: **[#101](…)** **[#102](…)** **[#103](…)**
```

Canvas, grouped: as ungrouped, under each `### [repo](…)` sub-heading. A group holding only a collapsed row shows that row alone.

## Breaking Change

Non-breaking: minor. A new optional input, and a layout change with no config change.

- No persisted format changes. Where 2 or more PRs by one default author share a section or group, the first `update` run after upgrading edits the message and the first run rewrites the canvas, since both differ from what the previous version wrote

## Summary

- R1: shared section model in `prview`
- R2: sections hold rows
- 1: `collapse-prs-from-authors` input
- 2: collapsing in `prview` and the content packages
- 3: collapsed rows in both builders
- 4: README
- 5: collapsed row style: linked, bold label with a count, bold numbers

## Steps

### R1: shared section model in `prview`

- Move `PRSection` into `prview`, with `RepositoryRows` for its groups, still holding PRs. Both content packages and both builders use it
  - `messagecontent.PRsOfRepository`'s name and URL strings go: `messagebuilder` reads them off `models.Repository`
  - `canvasbuilder`'s `section` takes a `prview.PRSection` in place of its `prs` and `groups` fields
- No behaviour change: message snapshots and canvas goldens stay as they are
- ~4 packages and their tests, mostly mechanical. Update the specs naming the moved types

### R2: sections hold rows

- Add `prview.Row`, implemented by `PR`. `PRSection` and `RepositoryRows` hold `[]Row`
- Builders type-switch on each row. Only the `PR` case exists yet
- `PRSection.HasRows` replaces the emptiness checks in `messagecontent` and `canvasbuilder`: `PRSection.HasPRs` and `section.isEmpty`
- No behaviour change: snapshots and goldens stay as they are

### 1: `collapse-prs-from-authors` input

- `action.yml`, `config` (`InputCollapsePRsFromAuthors`, `ContentInputs.CollapsePRsFromAuthors`), README inputs table, `testhelpers.TestConfig`
- Done means `go run .github/scripts/check_inputs.go` passes and a `config` test reads both separators and drops an empty item
- Update `config.spec.md`

### 2: collapsing in `prview` and the content packages

- `prview`: `CollapsedRow`, `GetAuthorLabel`, `MinPRsToCollapse` and `RowsCollapsingAuthors` per § Target Shape. A login listed twice in `collapsePRsFromAuthors` still gets one row
  - Package test for the rule: the 1 and 2 boundary, two authors in `collapsePRsFromAuthors` order, PRs of one author interleaved with others, exact login match, a login listed twice
- `messagecontent` and `canvascontent`: `newPRSection` builds each flat section, or each repository group, through `RowsCollapsingAuthors`
  - `canvascontent_test.go` pins the canvas wiring: an open, the WIP and the merged section collapse flat, and an open section collapses per repository group. `testhelpers.DescribeRows` renders rows as strings for these tests and the `prview` one. The message wiring is pinned by step 3's snapshots, which run the whole pipeline
- Update `prview.spec.md`, `messagecontent.spec.md` and `canvascontent.spec.md`

### 3: collapsed rows in both builders

- `messagebuilder` and `canvasbuilder` render the `CollapsedRow` case per § Target Shape. One renderer per builder, shared by every section
- New post mode snapshot scenario in `snapshot_test.go`, ungrouped and grouped:
  - one section with 3 Dependabot PRs, 2 Renovate PRs and a human PR, interleaved in the section's sort order
  - one section with exactly 1 Dependabot PR, which keeps its full row
  - the merged section with 2 Dependabot PRs
  - grouped: one repository with only Dependabot PRs
  - a third scenario: no open PRs, no `no-prs-message`, and only 2 merged Dependabot PRs, so the message is sent for a section holding only a collapsed row
  - the fixtures need `GetTestPROptions.AuthorType` for a `Bot` author and `snapshotScenario.mergedPRsByRepo`
- New canvas goldens, flat and grouped, covering a collapsed row in an open, the merged and the WIP section, and a group holding only a collapsed row
- Update `messagebuilder.spec.md` and `canvasbuilder.spec.md`

Live check, covering steps 1 to 3, checked by the user: the PR's E2E job runs the default input over `hellej/pr-slack-reminder-test-repo-1`, which has ~15 open Dependabot PRs. Done means the grouped and ungrouped dev channels and canvases each show one `🤖 dependabot:` row of ~15 numbers for that repository, and every link opens its PR. No E2E repository has Renovate or merged Dependabot PRs, so those are covered by snapshots only.

### 4: README

- One line under `## PR Tracker Canvas` and one under `### Example Output`: 2 or more PRs by one author in `collapse-prs-from-authors` in a section collapse into one row of linked numbers
- `collapse-prs-from-authors` joins the list of inputs that shape the canvas, under `### Good to know`
- Done means `make check-style` passes

### 5: collapsed row style: linked, bold label with a count, bold numbers

Live feedback on the first version: a collapsed row reads plainer than the bold blue title rows beside it, and it drops the PR titles.

- The label becomes a bold link with the row's PR count: `🤖 dependabot (14)`, then a plain `: `, then the numbers
- Every number link is bold too, so the whole row matches a title row
- The label links to a GitHub PR search listing the row's author's PRs:
  - `is:pr`, then `is:open`, or `is:merged` for a row of merged PRs, then the author qualifier
  - Author qualifier: `author:app/<label>` for a login ending in `[bot]`, else `author:<login>`. Source: [GitHub docs, searching issues and pull requests](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests), `author:app/USERNAME`
  - A row whose PRs are all in one repository: that repository's pulls URL with `?q=<query>`
  - A row spanning repositories: `https://github.com/search?type=pullrequests&q=<query>` with one `repo:<owner>/<name>` per repository, in the row's order
  - Query URL-encoded
  - Repeated `repo:` qualifiers OR together: see docs/third-party-facts.md § Repeated `repo:` qualifiers in a GitHub web search OR together
- The search cannot follow the message's buckets: a row in one open section links to every open PR of that author. Accepted: it reads as "this bot's PRs"
- `prview.CollapsedRow` owns the search URL and the merged/open choice, derived from its PRs. Builders only render
- Message: label link element with bold style, the `: ` and `" "` runs plain, each number a bold link element. Canvas: `- **[🤖 dependabot (14)](<search URL>)**: **[#3](…)** **[#4](…)**`, the label still through `escapeMarkdown`
- Existing collapsed snapshots and goldens change: read the failing diff, confirm it is only this restyle, then re-record. Pre-plan snapshots and goldens stay unchanged
- `prview` package test for the URL: single repository, spanning repositories, merged, a `[bot]` login and a plain login
- New mark-as-stale snapshot case "collapse PRs from authors" in `TestSnapshotsPreviousMessageMarkedStale`, with its `.json` and `.state.json`: it pins the bold links surviving mark-as-stale
- Update `prview.spec.md`, `messagebuilder.spec.md`, `canvasbuilder.spec.md`, the plan's § Target Shape examples, and the README example line

Live check, by the user: the E2E dev channels and canvases show the bold label with its count, and the label opens the bot's open PRs in `hellej/pr-slack-reminder-test-repo-1`.

## Consequences

### Positive

- A bot's batch takes one row per section, so human PRs stay visible
- Content packages decide every row and builders only render, so a further row kind costs one type and one renderer case per builder

### Negative

None

### Caveats

- A collapsed PR loses its age, old-PR marker and reviewers from view. A reviewer opens the PR to see them
- An ungrouped row spanning repositories can show the same number twice. Only the link targets differ
- A large batch makes one long row, ~15 numbers in the test repository
- A bot under a user account, such as a self-hosted Renovate, needs its login added to the input

### Neutral

- Caps still count every collapsed PR, so a collapsed row frees no room for more PRs
- [012_count-PR-age-from-ready-for-review.md](012_count-PR-age-from-ready-for-review.md) also touches `prview`, on the age and sort side. Either can land first

## Justification

### Rows as a sum type in a shared `prview` model

- `prview` provides models and helpers, content packages organize PRs, builders render what they are given. A side field such as `CollapsedPRs` on each section would leave builders deciding where collapsed rows go
- Both content packages already had near-identical section types, so one shared model replaces two
- The refactor steps cost a larger diff than a side field. It was chosen for the end architecture, not for size
