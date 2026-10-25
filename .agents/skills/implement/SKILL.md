---
name: implement
description: "Run an implementer and reviewer sub-agent loop over a whole plan, up to four rounds, until the reviewing agents return PASS. Use when: implementing a plan file or a task, or the user runs /implement."
argument-hint: "The plan file, e.g. .local/plans/001_GraphQL-migration.md, or a task with no plan"
---

# Implement and Review Loop

You orchestrate. You do not write the code or review it. Verifying is yours: the
acceptance criteria before you hand back, and any check an interrupted round left
unfinished. Verifying re-runs a check someone already specified. Reviewing forms new
findings.

## Scope

- Check what of the plan is already implemented, and continue from there
- One whole plan per run. Its steps are the suggested implementation order, not review gates
- Review mid-way only at a step the plan marks `(checkpoint)`. That review skips the whole-diff checks: dead code, the comment list, spec and plan consistency
- The final review covers the whole diff against the base branch, with every check
  - A follow-up run on a branch whose earlier commits were already reviewed too: the base is
    the branch's base, such as `main`, never `HEAD`. The new change can make earlier prose false
- Confirm the planned scope of the run with the user before starting
- Branch per AGENTS.md § Git, a mandatory read, unless the plan says otherwise. If it is silent and the user
  has not said, ask it with that same confirmation, never once the work is under way
- `git status` before round 1. Name any unrelated work to every agent as off-limits: do
  not stage it, revert it, or fold it into the change
- Fetch the base branch before round 1 and note how far behind the working branch is. If
  behind, bring the base in first, so the plan's claims and the review run against current code

## No Plan File

Write one. The rest of this skill reads a plan.

- Draft the task to `.local/implement-<slug>.md`, already gitignored
- Give it what this skill reads: brief motivation, what to change, the acceptance criteria, what it must not do, and the test cases
- Verify its claims against the tree before showing it, per § Acceptance Criteria below
- Confirm it with the user before round 1
- If drafting turns up a design decision, stop and hand back to the [plan skill](../plan/SKILL.md)

## Acceptance Criteria

Before round 1, read the plan and pull out what must hold when it is done. Pass it to
the implementer and the reviewer, and check it yourself before handing back.

A step often ends in an `Acceptance:` line, but steps state criteria in passing too, so read
the whole plan:

- A regression net the plan must leave untouched, snapshots and golden files above all
- Wording the plan says is preserved verbatim
- What the plan must **not** do: its non-goals
- Test cases the plan lists are a minimum. A guard they don't cover needs one too

Name the trap alongside an "unchanged" criterion. An unchanged snapshot means diagnose
the diff, never re-record it.

Work out where the regression net is blind before round 1 and give it to the implementer and the reviewer:
the implementer tests those spots, the reviewer mutates them.

One criterion holds on every run, stated or not: prose describing behaviour the change
alters stays true, specs and AGENTS.md above all.

If the plan states no other criteria, say so and carry on. Do not invent them.

A criterion a checkpoint cannot satisfy on its own is a briefing bug, not a finding: a lint
target an uncalled function fails by construction. Check each is reachable before passing it on.

Check the plan's claims about the tree yourself: paths, call sites, what is already
landed, and whether wording the plan quotes verbatim still matches the tree. Tell the
implementer where the plan has gone stale.

## Rounds

1. Spawn the `implementer` agent. Brief it with:
   - The plan file path, the checkpoints, and the acceptance criteria
   - What is already landed, by file and symbol
   - Findings carried forward from a checkpoint review
2. At the final review only: when the implementer reports, spawn `prose-clarity-checker` and
   give it the base. Send its medium findings to the implementer and wait for the fixes.
   Its nits wait for § Nits On a Pass. This does not spend a round
3. Spawn the reviewing agents in parallel, in one message:
   - At a checkpoint, the `reviewer` only
   - At the final review, the `reviewer`, a new `prose-truth-checker`, and the
     `prose-clarity-checker` from step 2, continued with `SendMessage`
   - Brief the `reviewer` with the plan, the implementer's report, and the blind spots
     above. Tell it to check the report's claims against the tree. A mock renders
     whatever the code asks it for, so a wrong query shape or page size passes it
   - Brief `prose-truth-checker` with the base, the behaviours the change alters, and the
     plan files to sweep. Brief `prose-clarity-checker` with the base again
4. Merge the reports into one verdict. It is `CHANGES NEEDED` when any report holds a high
   or medium Fix, or a Document finding, else `PASS`. Every report's nits go to § Nits On a
   Pass. Each claim `prose-truth-checker` marks `unverifiable` goes to the hand-back
   - `PASS`: clear any open nits as below, then stop
   - `CHANGES NEEDED`: send every high and medium Fix and Document finding to the
     implementer in one batch. Nits wait for § Nits On a Pass. Then ask each agent of
     step 3 to re-review. Give each agent the implementer's answers, including any finding
     the implementer rejected and why. Otherwise nothing surfaces a deadlock. That is one
     more round
5. Four rounds maximum. At the cap, hand back with the open findings named and the code
   as it stands

The prose agents are `prose-truth-checker` and `prose-clarity-checker`. Spawn every agent
with `run_in_background: false`.

Agent definitions load at session start. If an agent this section names is not registered
yet, spawn `general-purpose`, paste the agent file's body into the prompt, and tell it to
invoke each skill its `skills:` line lists first. `general-purpose` gets neither the body
nor the `skills:` preload, so the TDD, spec-sync, mutation and writing rules arrive no other
way.

### Nits On a Pass

Send the nits to the implementer in one batch. `nit` is a severity, not a size: a rename
worth doing can touch many files.

On a wording nit, name what is wrong and add no wording of your own. Replacement phrasing
you write outranks the style rules the implementer already carries, so a nit answered
literally can breach them. A prose agent's sketches are the exception: forward every one
unchanged, with its label. The implementer chooses among them, and applies
AGENTS.md § Output Style to the sentence it writes.

The size of the fix decides what follows, not whether to make it.

- A line or two: stop when the implementer reports, with no re-review. Check `make test`
  and the acceptance criteria yourself, since a nit fix still edits the code
- Larger: the reviewer reviews the fix, and so do the prose agents when the fix changes
  prose. That does not spend a round, since the verdict was
  already `PASS`. Never land a real change that no reviewer sees. If that review returns
  `CHANGES NEEDED`, hand back instead of starting another cycle

## Continue Agents, Never Respawn

- After round 1, always continue the existing `implementer`, `reviewer` and prose agents
  with `SendMessage`
- A fresh spawn starts cold. The implementer re-derives the plan and often redoes work.
  The reviewer re-raises findings it already got fixed
- One exception, an agent that cannot resume at all. See § Interruptions

## Interruptions

An agent can stop early on a session limit or an API error, leaving its round unfinished
and its last report stale.

Continue it with `SendMessage`, per above. First read the state off the tree rather than
off the report:

- `git status` and `git diff`, for what landed
- `make test`, and the acceptance criteria

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

Before ending a turn with an agent still running, schedule a check-in about an hour out with
`send_later`, where available. A session limit otherwise stalls the run until the user writes.
Cancel it once the run is done.

## Stop Early

Stop and hand back to the user when:

- The implementer rejects a finding and the agent that raised it repeats it. That is a deadlock, and
  a further round will not break it
- The plan turns out to be wrong or blocked
- Tests fail for a reason outside the plan

These are about the work being wrong or blocked. An agent that stops while the work is
still sound is an interruption, above.

## Hand Back

Report to the user:

- The verdict and how many rounds it took
- Each acceptance criterion, and whether it holds. Check them yourself, do not relay
  any agent's word for it
- What changed, by file
- Each deviation from the plan, and how the plan now reads
- Nits, cleared or left, and why any was left
- Any finding the implementer rejected, any the reviewer labelled `unverified`, and any claim `prose-truth-checker` marked
  `unverifiable`
- The reviewer's Outside this change items, for the user to fold into this change or queue
  as a separate task

Leave the changes uncommitted, unless the user asked for commits.

## What the Run Taught

Last, two lists. Evidence from this run only.

- Both lists empty is the normal outcome. Most agent slop has no fix
- Report a lesson only if it cost this run something, or you have seen it more than
  once

**Lessons.** What this run showed about the loop. One line each, three at most. Examples:

- A round spent on something the brief could have prevented
- A finding a reviewing agent has raised more than once
- An agent asking you for context it should have been briefed with
- Work an agent redid after a handover
- A rule any agent ignored, or that made the work worse

**Proposed changes.** Only where a file edit would have prevented a lesson above.

- Read the file first. These rules are mostly already written, and a duplicate proposal
  is worse than none
- Name the file: this skill, `implementer.md`, or `reviewer.md`
- Say what to add, cut or reword. A cut counts as much as an addition
- Small is fine: one sentence made explicit or less confusing is a real fix
- A lesson with no proposed change is a complete entry
- Never edit these three files mid-loop
