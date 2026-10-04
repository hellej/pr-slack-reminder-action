# Notification Text Says What's Asked

date: 2026-10-04
status: draft

## Requirements

- The notification text names how many open PRs wait on each next action, e.g. `2 PRs to review, 1 to merge, 1 waiting for author 👀`
  - Today it reads `3 open PRs are waiting for attention 👀`, so a reader has to open the message to see whether anything is theirs
- Counts come in the order review, merge, author. A zero count is left out
- Serves AGENTS.md § Purpose: with 0 to 8 open PRs, a member who gets the notification sees from it whether to open the message now
- Lands on a feature branch, through a PR
- Non-goals:
  - The message body, the canvas
  - The no-open-PRs text, `Nothing waiting for review 🎉`
  - The mark-as-stale edit's text

## Target Shape

- `Content.SummaryText` is the message's top-level `text`, which Slack shows in notifications when the message carries blocks. See docs/third-party-facts.md § A message with `blocks` shows its top-level `text` only in notifications
- `getSummaryText` takes the three next-action counts instead of the open PR total
  - Parts: `N to review`, `N to merge`, `N waiting for author`, joined with `, `, then ` 👀`
  - The first part names the noun: `1 PR` or `N PRs`, by its own count
  - Counts are PRs per bucket, before `RowsCollapsingPRsFromAuthors`, so collapsed bot PRs count each
- No action input or permission changes

Sketches, with 2 PRs waiting for review, 1 ready to merge, 1 waiting for author:

- Posted: `2 PRs to review, 1 to merge, 1 waiting for author 👀`
- Edited by `update` after the merge: `2 PRs to review, 1 waiting for author 👀`
- Only waiting for author: `1 PR waiting for author 👀`
- 1 to merge, 3 waiting for author: `1 PR to merge, 3 waiting for author 👀`
- No open PR: `Nothing waiting for review 🎉`, as today
- Marked stale: keeps the text it was last written with, as today (`lastWrittenMessage.SummaryText` in `markPreviousMessageStale`)

## Breaking Change

- Non-breaking: minor, a user-visible enhancement with no input or config change
- State: `LastWrittenMessage.SummaryText` keeps its shape. The first post after upgrading marks the previous message stale with its old wording, which is that message's own text

## Summary

1. Name what each open PR count is waiting on in the summary text

## Steps

### 1. Name What Each Open PR Count Is Waiting On in the Summary Text

- `internal/messagecontent/`: `getSummaryText` takes the ready to merge, waiting for author and waiting for review counts from `GetContent`'s buckets
  - The `.state.json` goldens and `main_test.go` expectations are the test: they already hold all three buckets, review and merge alone, and review alone
  - `TestGetContentSummaryAndNoOpenPRsText`: reword the existing cases, and add two: review and author but no merge, so an empty middle part is left out; merge and author but no review, so the noun moves to whichever part comes first
- `messagecontent.spec.md` § Behaviour: rewrite the `SummaryText` bullet
- `cmd/pr-slack-reminder/main_test.go`: ~20 `expectedSummary` and saved-state expectations change to the new wording
- Re-record the ~11 `.state.json` snapshots holding `summaryText` with `make update-test-snapshots`, reading the diff first
- `internal/messagebuilder/` tests: reword the `SummaryText` fixture to the new wording, ~2 places
- No live check: the path from `SummaryText` to Slack's `text` is unchanged. The PR's E2E runs post and update with the new text

## Consequences

### Positive

- A reader sees from the notification whether a PR waits on them

### Negative

- None

### Caveats

- The message mentions only authors with a mapped Slack user. A reviewer with no PR in the list is not mentioned, so whether the text reaches them rests on their own channel notification settings
- The text names next actions, not people: a PR's author still opens the message to see that `1 waiting for author` is theirs

### Neutral

- None
