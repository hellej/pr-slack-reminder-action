---
name: plan
description: "Force the mandatory pre-plan read sequence and the required plan structure (requirements, target shape, breaking-change call, ordered steps) before drafting or finalizing an implementation plan. Use when: entering plan mode, designing an implementation approach, or the user runs /plan."
argument-hint: "Optional: what you're about to plan"
---

# Mandatory Pre-Plan Read Sequence

Before drafting an implementation plan, read:

- **Package Specs** — every touched package's `<package>.spec.md` in full, plus any related package's spec needed to understand how the change fits
- **Code** — only the parts still unclear after reading the specs
- **Third-party APIs and libraries** — verify any method, capability, documented behavior, or required permission/scope the plan relies on (`github.com/google/go-github`, `github.com/slack-go/slack`, GitHub token permissions, Slack OAuth scopes, etc.) against the library's source or the provider's official docs — check the local module cache, a vendor dir, or a local checkout (ask the user for its path, or search near the repo). Link the confirming doc page at the point in the plan that depends on it. Never assume or guess. Read [`docs/third-party-facts.md`](../../../docs/third-party-facts.md) first: it may already name the source. Cite that source, not the file
- **Unverified third-party claims** — a claim research came back marked unverified cannot carry a step. Verify it, or design so nothing depends on it, before drafting

## Mandatory Plan Steps

1. Create the plan file in `.local/plans/` (gitignored), unless the user gives another path, e.g. `docs/plans/` (committed)
   - Name: `NNN_title-with-hyphens.md` — 3-digit sequence number, underscore, the H1 title lowercased with hyphens for spaces (acronyms like `PR` stay uppercase), e.g. `001_PR-tracker-canvas.md`
   - Right after the H1, add: `date: YYYY-MM-DD` then `status: draft` (or `ready`); bump `date` on substantial revisions or status changes
2. Consider a pre-refactor step: for bigger features, restructuring existing code first — often renaming things so the new feature/concept lands as an explicit, self-evident diff — can make the real change smaller, safer, and more explicit. Propose it as a separate step before the main implementation when it earns its keep. Optional; skip for small changes. Assess test coverage of the touched code and close any gap found — the regression net for both the refactor and the feature work built on top of it
3. Draft the plan, following the Structure and Style sections below
4. Iterate with the user: surface every open decision and ask instead of guessing, revise, repeat until nothing ambiguous or optional is left. Planning isn't done after one pass — treat each round of feedback as new input, not a rubber stamp

## Style

- Apply the [writing skill](../writing/SKILL.md)'s language style: plain words, short bullets, no filler, concise
- A plan instructs, it doesn't argue for itself
- Justify a choice in one sentence, only where the other choice is reasonable. If it needs more,
  it goes in Justification and the step links to it
- Cite by symbol, not line: `isActiveEnoughForCanvas`, not `canvascontent.go:92-98`. Lines drift
- Steps are bullets, one claim each, sub-bullets for the detail under it. State a constraint once
- Fixing a claim doesn't earn a paragraph about the fix

## Structure

1. Requirements/goals/non-goals — a short bullet list, or a reference to another document that already states them, incl. motivation for the change (what problem this solves and for whom), if not obvious from the requirements
2. The target shape: the resulting architecture/feature, if not already fully covered by the requirements. Always call out changes to action inputs (`action.yml`) here, and any new/changed required permissions (GitHub token permissions, third-party OAuth scopes) here too
3. Whether the change is breaking or non-breaking, per the [release skill](../release/SKILL.md)'s semver table (patch/minor/major)
4. A short summary listing the steps
5. The full steps, each naming the files/packages it touches, in that same order — the order they're written IS the implementation order, never a separate order/sequence table. Refactor steps (if any) are numbered `R1`, `R2`, ...; real implementation steps restart at `1`
   - Order by dependency. A step calling an external API the repo hasn't used goes early, before the code built on it
   - Reordering steps means renumbering the headings and remapping every `Step N` reference. References to another plan's steps stay as they are
   - Don't plan tests as their own step — writing tests is a natural, inherent part of implementing each step (see the [coding skill](../coding/SKILL.md)'s TDD steps) — unless the feature is complex enough to need its own test-suite shape/refactor planned up front
   - If a step isn't verified by tests (tooling, CI config, docs, live-API checks), state inline what verifying it done means
6. Consequences, after the steps, only if there's something worth saying — subsections **Positive**, **Negative**, **Caveats**, **Neutral**, each a short bullet list; include only the subsections that actually apply
   - **Negative** is for effects that leave the repo worse off than not implementing the plan at all
   - **Caveats** is for the costs of a change that is still worth making: a limit it doesn't lift, a rough edge it leaves, a thing it makes harder. Don't put these under **Negative**
7. Justification, last, only for a choice whose reasoning needs more than one sentence. One `###` per choice; the step links to that heading and states the decision in one line
   - It exists so a settled choice survives review rounds without re-litigation, and so steps stay instructions. A reviewer treats a linked heading as settled unless the reasoning itself is wrong
   - Not a decision log, and not a home for reasoning nobody questioned. A choice nobody would make differently needs no entry
8. No other top-level sections — fold anything else into whichever of 1-7 it belongs to, stated as a plain fact, not narrated as a decision

## Definition of Done

- No open questions, "do this or that" branches, or unresolved alternatives remain
- Discarded options are omitted or mentioned in one line at most, never elaborated on
- Only set `status: ready` when the user explicitly says so; leave `status: draft` otherwise, even once the above two hold

If a task was given as an argument to this skill, work through the read sequence then draft the plan for it now.
