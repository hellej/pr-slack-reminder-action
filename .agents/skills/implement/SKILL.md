---
name: implement
description: "Run an implementer and reviewer sub-agent loop over one plan step, up to four rounds, until the reviewer returns PASS. Use when: implementing a step from docs/plans/, or the user runs /implement."
argument-hint: "The plan file and step, e.g. docs/plans/001_GraphQL-migration.md step 3"
---

# Implement and Review Loop

You orchestrate. You do not write the code or review it. Verifying is yours: the
acceptance criteria before you hand back, and any check an interrupted round left
unfinished. Verifying re-runs a check someone already specified. Reviewing forms new
findings.

## Scope

- Check what of the plan is already implemented, and continue from there
- One plan step or phase (a group of related steps) per run
- Confirm the planned scope of the run with the user before starting
- `git status` before round 1. Name any unrelated work to both agents as off-limits: do
  not stage it, revert it, or fold it into the step

## Acceptance Criteria

Before round 1, read the step and pull out what must hold when it is done. Pass it to
both agents, and check it yourself before handing back.

A step often ends in an `Acceptance:` line, but states criteria in passing too, so read
the whole step:

- A regression net the step must leave untouched, snapshots and golden files above all
- Wording the step says is preserved verbatim
- What the step must **not** do, usually a later step's work
- Test cases the step lists are a minimum. A guard they don't cover needs one too

Name the trap alongside an "unchanged" criterion. An unchanged snapshot means diagnose
the diff, never re-record it.

Work out where the regression net is blind before round 1 and give it to both agents:
the implementer tests those spots, the reviewer mutates them.

If the step states no criteria, say so and carry on. Do not invent them.

Check the step's claims about the tree yourself: paths, call sites, what is already
landed, and whether wording the step quotes verbatim still matches the tree. Tell the
implementer where the plan has gone stale.

## Rounds

1. Spawn the `implementer` agent. Brief it with:
   - The plan file path, the step, the step text, and the acceptance criteria
   - Which sections outside the step to read, by heading. Cost models, error handling and
     timeouts usually sit outside the step that depends on them
   - The contracts it consumes from steps already landed, by file and symbol
   - Findings carried forward from the previous step's review
2. When it reports, spawn the `reviewer` agent. Give it the same step, the implementer's
   report, and the blind spots above. Tell it to check the report's claims against the
   tree. A mock renders whatever the code asks it for, so a wrong query shape or page
   size passes it
3. Read the verdict:
   - `PASS`: clear any open nits as below, then stop
   - `CHANGES NEEDED`: send the findings to the implementer, then ask the reviewer to
     re-review. Give it the implementer's answers, including any finding it rejected and
     why, otherwise nothing surfaces a deadlock. That is one more round
4. Four rounds maximum. At the cap, hand back with the open findings named and the code
   as it stands

Spawn both agents with `run_in_background: false`.

Agent definitions load at session start. If `implementer` or `reviewer` is not registered
yet, spawn `general-purpose`, paste the agent file's body into the prompt, and tell it to
invoke the `coding` skill first. `general-purpose` gets neither the body nor the `skills:`
preload, so the TDD, spec-sync and mutation rules arrive no other way.

### Nits On a Pass

Send the nits to the implementer in one batch. `nit` is a severity, not a size: a rename
worth doing can touch many files.

The size of the fix decides what follows, not whether to make it.

- A line or two: stop when the implementer reports, with no re-review. Check `make test`
  and the acceptance criteria yourself, since a nit fix still edits the code
- Larger: the reviewer reviews it. That does not spend a round, since the verdict was
  already `PASS`. Never land a real change that no reviewer sees. If that review returns
  `CHANGES NEEDED`, hand back instead of starting another cycle

## Continue Agents, Never Respawn

- After round 1, always continue the existing `implementer` and `reviewer` with
  `SendMessage`
- A fresh spawn starts cold. The implementer re-derives the step and often redoes work.
  The reviewer re-raises findings it already got fixed
- One exception, an agent that cannot resume at all. See Interruptions

## Interruptions

An agent can stop early on a session limit or an API error, leaving its round unfinished
and its last report stale.

Continue it with `SendMessage`, per above. First read the state off the tree rather than
off the report:

- `git status` and `git diff`, for what landed
- `make test`, and the step's acceptance criteria

A half-done change often compiles and passes, so a green suite is one input, not the
round's verdict.

Then tell the agent where the round stopped: what it had already fixed, and what was left
unverified.

An implementer that cannot resume is replaced. Spawn a fresh one over the same diff,
telling it which findings landed and which are left. Writing the code is not yours to take
over, not even for a nit.

Finishing its verification is yours. Re-run it and say so in the hand-back. Never report
`PASS` on a verification nobody finished.

A reviewer that cannot resume is replaced the same way, handing the fresh one any earlier
findings and the implementer's answers to them.

Check the tree before you do. A reviewer that stopped mid-mutation never reached the
restore step, so a file it mutated in place is still mutated.

An interruption never spends a round.

## Stop Early

Stop and hand back to the user when:

- The implementer rejects a finding and the reviewer repeats it. That is a deadlock, and
  a further round will not break it
- The step turns out to be wrong, blocked, or bigger than the plan says
- Tests fail for a reason outside the step

These are about the work being wrong or blocked. An agent that stops while the work is
still sound is an interruption, above.

## Hand Back

Report to the user:

- The verdict and how many rounds it took
- Each acceptance criterion, and whether it holds. Check them yourself, do not relay
  either agent's word for it
- What changed, by file
- Each deviation from the plan, and how the step now reads
- Nits, cleared or left, and why any was left
- Any finding the implementer rejected, and any the reviewer labelled `unverified`

Leave the changes uncommitted, unless the user asked you to commit between chained steps.

## What the Run Taught

Last, two lists. Evidence from this run only.

- Both lists empty is the normal outcome. Most agent slop has no fix
- Report a lesson only if it cost this run something, or you have seen it on more than
  one step

**Lessons.** What this run showed about the loop. One line each, three at most. Examples:

- A round spent on something the brief could have prevented
- A finding the reviewer has now raised on more than one step
- An agent asking you for context it should have been briefed with
- Work an agent redid after a handover
- A rule either agent ignored, or that made the work worse

**Proposed changes.** Only where a file edit would have prevented a lesson above.

- Read the file first. These rules are mostly already written, and a duplicate proposal
  is worse than none
- Name the file: this skill, `implementer.md`, or `reviewer.md`
- Say what to add, cut or reword. A cut counts as much as an addition
- Small is fine: one sentence made explicit or less confusing is a real fix
- A lesson with no proposed change is a complete entry
- Never edit these three files mid-loop
