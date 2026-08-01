---
name: plan
description: "Force the mandatory pre-plan read sequence and the required plan structure (requirements, target shape, breaking-change call, ordered steps) before drafting or finalizing an implementation plan. Use when: entering plan mode, designing an implementation approach, or the user runs /plan."
argument-hint: "Optional: what you're about to plan"
---

# Mandatory Pre-Plan Read Sequence

Before drafting an implementation plan, read:

- **Package Specs** — every touched package's `<package>.spec.md` in full, plus any related package's spec needed to understand how the change fits
- **Code** — only the parts still unclear after reading the specs
- **Third-party APIs and libraries** — verify any method, capability, documented behavior, or required permission/scope the plan relies on (`github.com/google/go-github`, `github.com/slack-go/slack`, GitHub token permissions, Slack OAuth scopes, etc.) against the library's source or the provider's official docs — check the local module cache, a vendor dir, or a local checkout (ask the user for its path, or search near the repo). Link the confirming doc page at the point in the plan that depends on it. Never assume or guess

## Mandatory Plan Steps

1. Create the plan file in `.local/plans/` (gitignored, untracked), unless the user says otherwise
2. Consider a pre-refactor step: for bigger features, restructuring existing code first — often renaming things so the new feature/concept lands as an explicit, self-evident diff — can make the real change smaller, safer, and more explicit. Propose it as a separate step before the main implementation when it earns its keep. Optional; skip for small changes. Assess test coverage of the touched code and close any gap found — the regression net for both the refactor and the feature work built on top of it
3. Draft the plan, following the Structure and Style sections below
4. Iterate with the user: surface every open decision and ask instead of guessing, revise, repeat until nothing ambiguous or optional is left. Planning isn't done after one pass — treat each round of feedback as new input, not a rubber stamp

## Style

- Apply the [writing skill](../writing/SKILL.md)'s language style: plain words, short bullets, no filler, concise

## Structure

1. Requirements/goals/non-goals — a short bullet list, or a reference to another document that already states them
2. The target shape: the resulting architecture/feature, if not already fully covered by the requirements. Always call out changes to action inputs (`action.yml`) here, and any new/changed required permissions (GitHub token permissions, third-party OAuth scopes) here too
3. Whether the change is breaking or non-breaking, per the [release skill](../release/SKILL.md)'s semver table (patch/minor/major)
4. A short summary listing the steps
5. The full steps, each naming the files/packages it touches, in that same order — the order they're written IS the implementation order, never a separate order/sequence table. Refactor steps (if any) are numbered `R1`, `R2`, ...; real implementation steps restart at `1`
   - Don't plan tests as their own step — writing tests is a natural, inherent part of implementing each step (see the [coding skill](../coding/SKILL.md)'s TDD steps) — unless the feature is complex enough to need its own test-suite shape/refactor planned up front
6. No other top-level sections — fold anything else (e.g. a "decisions made while planning" log) into whichever of 1-5 it belongs to, stated as a plain fact, not narrated as a decision

## Definition of Done

- No open questions, "do this or that" branches, or unresolved alternatives remain
- Discarded options are omitted or mentioned in one line at most, never elaborated on

If a task was given as an argument to this skill, work through the read sequence then draft the plan for it now.
