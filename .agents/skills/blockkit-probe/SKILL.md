---
name: blockkit-probe
description: "Post a hand-written Slack Block Kit payload to the dev channel to see how a planned message renders, before any Go code exists. Use when: probing a message layout, checking block spacing or a heading level live, trying out a planned message shape, or iterating on Block Kit JSON."
argument-hint: "Optional: what layout to probe, e.g. 'plan 007 four sections'"
---

# Probe a Block Kit layout live

## Purpose

Slack documents no spacing, and a snapshot test proves only that the JSON is what the code built.
Posting the payload by hand answers what a plan's target shape actually looks like, before the code
that would build it is written.

## When to Use

- A plan proposes a message layout and someone has to see it
- Comparing two layouts, block counts, or heading levels
- Checking whether Slack renders an element at all

## Procedure

1. Read the plan's target shape, if there is one.
2. Copy the row formats from `cmd/pr-slack-reminder/testdata/snapshots/*.json`. Those are real
   payloads, so link, age, reviewer and author runs come out right without re-deriving them from
   `internal/messagebuilder`.
3. Write the whole `chat.postMessage` body to `.local/<name>_payload.json`: `channel`, `text`,
   `blocks`. `.local` is gitignored and survives the session, so the next round edits the file
   instead of rebuilding it.
4. Post it:

   ```
   .agents/skills/blockkit-probe/send.sh .local/<name>_payload.json
   ```

   The script reads `INPUT_SLACK_BOT_TOKEN` from `.envrc`, prints `ok` and `ts`, and records the
   channel ID and ts in `<payload>.sent`.
5. Report the `ts` and let the user look. Checking the rendered message in Slack is the user's
   half, per the verification split.
6. Iterate: edit the payload, then edit the same message in place:

   ```
   .agents/skills/blockkit-probe/send.sh .local/<name>_payload.json edit
   ```

   `delete` removes it. Both read `<payload>.sent`, so the payload's `channel` can stay a name:
   `chat.update` and `chat.delete` take a channel ID only.

## Channel

`pr-reminders-test` is the dev channel. Never post a probe to the team's real channel: every
message there interrupts the team.

## Fixture Data

Use plausible PR titles, authors and repositories, and keep one row per case the layout has to
handle: an old PR with the `🚨` marker, a PR with approvers and commenters, a bot-authored PR, a
PR in a repository the team does not own. A layout that only ever gets the easy row proves
nothing.

## What the Response Tells You

- `ok: false` with `invalid_blocks` means the payload is malformed. The `response_metadata`
  messages name the block index and path
- Slack rewrites what it accepts: a unicode emoji inside a `text` run comes back as a separate
  `emoji` element. That is display-equivalent, so it is not a reason to change the payload
- Rendering questions (spacing between blocks, how a heading level looks, whether two sections
  read as one list) are answerable only by looking at Slack

## Afterwards

- Record what the probe settled in [docs/third-party-facts.md](../../../docs/third-party-facts.md),
  citing the probe: what rendered, what Slack rejected, what it silently dropped
- Leave the payload in `.local/` for the next round
