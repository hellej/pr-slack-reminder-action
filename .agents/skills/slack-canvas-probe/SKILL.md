---
name: slack-canvas-probe
description: "Replace the dev PR tracker canvas's content with hand-written markdown to see how a planned canvas layout renders, before any Go code exists. Use when: probing a canvas heading, row format, or footer live, or checking whether Slack renders a markdown construct inside a canvas."
argument-hint: "Optional: what canvas layout to probe, e.g. 'new WIP row format'"
---

# Probe a PR tracker canvas layout live

## Purpose

Slack documents no rendering rules for canvas markdown, and a snapshot test proves only that the
markdown string is what `internal/canvasbuilder` built. Writing the markdown by hand into the real
canvas answers what a planned canvas layout actually looks like, before the code that would build
it exists.

## When to Use

- A plan proposes a change to the canvas's headings, row format, or footer and someone has to see it
- Comparing two row formats or heading levels in the canvas specifically (for message layout, use
  [slack-message-probe](../slack-message-probe/SKILL.md) instead)
- Checking whether Slack renders a markdown construct at all inside a canvas

## Procedure

1. Read the plan's target shape, if there is one.
2. Copy the row format from `internal/canvasbuilder/testdata/*.md`, or from the current
   [README's PR Tracker Canvas example](../../../README.md) if it's already up to date.
3. Write the whole canvas body to `.local/canvas-payloads/<NNN>_<name>_canvas.md`. `NNN` is the
   next unused 3-digit number in the folder, so files keep sorting in creation order. `.local` is
   gitignored and survives the session, so the next round edits the file instead of rebuilding it.
4. Replace the dev canvas's content with it:

   ```
   .agents/skills/slack-canvas-probe/replace.sh .local/canvas-payloads/<NNN>_<name>_canvas.md
   ```

   The script prints the raw `canvases.edit` response. There is no post/edit/delete split like
   slack-message-probe's messages: every call rewrites the whole canvas, so re-run it after each edit.
5. Report that it's done and let the user look. Checking the rendered canvas in Slack is the
   user's half, per the verification split.
6. Iterate: edit the markdown file, run the script again.

## Canvas

The dev canvas is on `pr-reminders-test`, the same dev channel `slack-message-probe` posts messages to.
Its ID (`F0BMEPVR1DL`) is hardcoded in `replace.sh`, found via the "Run with canvas link" step of
`.github/actions/e2e-tests/action.yml` (`slack-channel-name-1`, which defaults to
`pr-reminders-test`). That same canvas gets rewritten by every `build`/`e2e` CI run, so a probe
left in it doesn't survive long. Screenshot before the next CI run lands.

Never point this at the team's real canvas: a canvas write there replaces the whole thing, wiping
whatever the team was using it for.

## Fixture Data

Use plausible PR titles, authors and repositories, and keep one row per case the layout has to
handle: an old PR with the `🚨` marker, a WIP row, a merged row with reviewers. A layout that only
ever gets the easy row proves nothing.

## What the Response Tells You

- `ok: false` names the error in the `error` field. `canvas_not_found` means the hardcoded ID is
  stale (the canvas was recreated); `missing_scope` means the bot token lacks `canvases:write`
- A canvas that shows duplicated headings or rows after a write is a Slack client rendering
  artifact, not a bad write. Reload the canvas (Cmd/Ctrl + R) to see its real content

## Afterwards

- Record what the probe settled in [docs/third-party-facts.md](../../../docs/third-party-facts.md),
  citing the probe: what rendered, what Slack rejected, what it silently dropped
- Leave the markdown file in `.local/canvas-payloads/` for the next round
