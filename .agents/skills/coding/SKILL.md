---
name: coding
description: "Force the mandatory AGENTS.md rules (TDD, git, spec sync, code style) into context before starting a code change (implementation). Use when: about to add, change or fix code in this repo, or the user runs /coding."
argument-hint: "Optional: what you're about to implement, change or fix"
---

# Mandatory Pre-Coding Read Sequence

Before writing or editing any `.go` file, re-read these [AGENTS.md](../../../AGENTS.md) sections in full. Each read is mandatory, and their rules are too:

- **Package Specs**, then the touched package's `<package>.spec.md`
- **Git**
- **Code Style**
- **Testing**

## Mandatory Implementation Steps

Implement only what was asked.

1. Write a failing test for the change, per AGENTS.md § Testing (TDD, and the snapshot exception)
2. Implement the minimal code to make it pass
3. Run `make test`; refactor if needed
4. Update the package's `.spec.md` in the same change if behaviour changed, Oddities included, per AGENTS.md § Package Specs (use [spec-writer skill](../spec-writer/SKILL.md))
5. For each comment you added, name the fact it states, per AGENTS.md § Code Style's comment rule. If there is none, rename or refactor the code and delete the comment
6. Last, re-read the prose you wrote: `git diff -- '*.md'` and the remaining comments. Apply AGENTS.md § Output Style and see if anything can be clearer or cut

## Implementing a Plan

- Small adjustments to the plan are fine if you find a better way, but do not change its intent or scope without explicit approval
- When the code deviates from a plan step, update that step's own text in the plan file, not only your report. A stale file list, call-site count or line number counts
  - Rewrite the step to describe what you built, keeping the reasoning: a plan reads as one piece written at once, never as text plus a note contradicting it
  - Keep each edit inside the step it describes
