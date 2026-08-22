---
name: plan-loop
description: "Run a research and review sub-agent loop over one implementation plan, up to three rounds, while you put every open decision to the user. Use when: drafting an implementation plan, or the user runs /plan-loop."
argument-hint: "What to plan, e.g. add a per-repo mute input"
---

# Plan and Review Loop

You orchestrate, and you are the only one who talks to the user. Sub-agents read and
review. Writing the plan file is yours alone, so nothing else edits it while a round is
open.

The loop's job is to leave no decision unmade and no claim unchecked. A sub-agent that
makes a decision for the user has defeated it.

## Scope

- One plan per run
- Follow the [plan skill](../plan/SKILL.md) throughout: it owns the read sequence, the
  file name, the structure, and the Definition of Done. This skill only says who does
  what, and when

## Round 0: Frame

Ask the user what no amount of reading can supply:

- The problem, and who has it
- What is out of scope
- Constraints already settled: input names, "no new inputs", must land before another plan
- Where the plan file goes: `.local/plans/` (gitignored) or `docs/plans/` (committed)

Keep asking until the frame is clear. An answer routinely opens the next question, so
batch what you have, read what comes back, and ask again. Skip anything the user's request
already answers.

Round 0 ends when you can state the goals and non-goals without guessing at any of them.
They become the plan's first section.

## The Facts File

[`docs/third-party-facts.md`](../../../docs/third-party-facts.md) holds what past plans
confirmed about APIs outside this repo.

- Grep its `##` headings before fanning out. An entry answers a question, or narrows it to
  "confirm this still holds at the version `go.mod` pins now"
- You write it, as you write the plan. Sub-agents report a fact and its source
- File what outlives the plan: a method's shape, a scope, a limit, an approach the API
  cannot support
- Dead ends above all. Nothing else records them

## Research Fan-out

Read the specs yourself. They are short, and you cannot draft without them.

Send out what is expensive or uncertain. Spawn `Explore` agents in the background, in
parallel, one question area each:

- Phrase the brief as a question, and say what shape the answer must take
- Third-party API questions go to their own agent, told to read the library's source at
  the version `go.mod` pins and to quote file and version, or to link the provider's doc
  page. Memory is not an answer

Don't fan out what one grep answers.

## Draft

Write the plan file per the plan skill's Structure.

Cite every third-party claim in the plan itself, at the point that depends on it, per that
skill's read sequence. The critic takes a cited claim as settled, so citing is what keeps
rounds cheap.

## Rounds

1. Spawn `plan-critic`, with `run_in_background: false`. Brief it with the plan file path,
   the round 0 answers, and any finding from an earlier round you argued down and why
2. Read the findings and sort them before touching the plan. Three kinds, three
   destinations, in this order:
   - **Ask**: to the user first, batched as below
   - **Verify**: out to a researcher, at the same time. Neither of these needs you. An
     answer that outlives the plan goes to the facts file
   - **Fix**: you revise the plan, once the answers are in
3. Revise, then re-review. That is one more round
4. Three rounds maximum. At the cap, hand back with the open findings named and the plan
   as it stands

Ask before you edit. An answer can redirect a step, and a `Fix` you already applied to it
is then work done twice. A `Verify` that comes back against the plan does the same, so a
claim stays marked until its answer lands and no step builds on it in the meantime.

Done is a `PASS` with no `Verify` outstanding and no `Ask` unanswered. Open nits do not
hold the plan.

Agent definitions load at session start. If `plan-critic` is not registered yet, spawn
`general-purpose`, paste the agent file's body into the prompt, and tell it to invoke the
`plan` and `writing` skills first. `general-purpose` gets neither the body nor the
`skills:` preload.

### Nits On a Pass

A nit is a detail that drifted, or wording. The implement loop corrects small inaccuracies
against the tree, so the plan does not have to be a diff to be finished.

Apply the nits yourself in one revision and stop. No re-review, and no round spent: the
verdict was already `PASS`. Re-read the sections you touched, since a nit fix still edits
the plan.

A nit that turns out to move a step's scope was misgraded. Fix it as one, and re-review.

## Asking the User

This governs the review rounds. Round 0 runs until the frame is clear, however many
batches that takes.

In a round, ask everything independent in one batch, never a question at a time. A
question whose options depend on another's answer waits for that answer, so a round can
take more than one batch. Rounds are what the user has to sit through, so spend them.

- Give every question the options you found, each with one line of trade-off, and
  recommend one. A question with no options hands the design back to the user
- Ask only what the user alone can settle. Anything research or the tree can answer is
  yours, not theirs
- Never let an agent pick. A critic that chose an option instead of raising an `Ask` is
  itself a finding: send it back
- Write each answer into the plan as a plain fact, never as a decision log

## Continue Agents, Never Respawn

- After round 1, continue the existing `plan-critic` with `SendMessage`
- A fresh critic re-checks claims it already confirmed and re-raises findings you resolved
  two rounds ago
- Researchers are one question each and normally finish for good. Continue one only to
  follow up on its own question

## Interruptions

An agent can stop early on a session limit or an API error, leaving its round unfinished.

Read the state off the plan file and `git status`, not off the agent's last report. Then
continue it with `SendMessage`, saying where the round stopped and what was left
unchecked.

A critic that cannot resume is replaced. Spawn a fresh one over the same plan file, handed
the earlier findings and your answers to them.

An interruption never spends a round.

## Stop Early

Hand back to the user when:

- The critic repeats a finding you argued down. That is a deadlock, and the user breaks it
- Research comes back saying the approach cannot work: the API doesn't exist, the
  permission isn't grantable. The target shape is now a user decision, not a revision
- The framing turns out wrong, and the problem is not the one round 0 recorded

## Hand Back

Report to the user:

- The plan file path, its `status`, and how many rounds it took
- Every question asked and how the answer landed in the plan
- Findings left open, and why
- Every claim still unconfirmed, and what would confirm it

Leave `status: draft` unless the user said `ready`. Do not start implementing: that is a
separate run of the [implement skill](../implement/SKILL.md).

## What the Run Taught

Last, report what this run showed about the loop itself. Evidence from this run only.

Examples, not a checklist:

- A question that should have been asked in round 0
- A round spent on something the critic's brief could have prevented
- A finding the critic has now raised on more than one plan
- A researcher answering from memory instead of source

Say what happened, then propose the change: which file it points at (this skill,
`plan-critic.md`, or the `plan` skill), and what to add, cut or reword. A cut counts as
much as an addition. Never edit them mid-loop.

Nothing to report is the normal outcome.

If a task was given as an argument to this skill, start at round 0 for it now.
