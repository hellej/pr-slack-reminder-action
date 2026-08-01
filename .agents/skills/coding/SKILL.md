---
name: coding
description: "Force the mandatory AGENTS.md rules (TDD, git, spec sync, code style) into context before starting a code change (implementation). Use when: about to add, change or fix code in this repo, or the user runs /coding."
argument-hint: "Optional: what you're about to implement, change or fix"
---

# Mandatory Pre-Coding Read Sequence

Before writing or editing any `.go` file, re-read these [AGENTS.md](../../../AGENTS.md) sections in full:

- **Package Specs** — the touched package's `<package>.spec.md` itself
- **Git** — never amend or force push
- **Code Style**
- **Testing** — TDD is mandatory, no exceptions for small changes

## Mandatory Implementation Steps

1. Write a failing test for the change; run it and confirm it fails for the expected reason
2. Implement the minimal code to make it pass
3. Run `make test`; refactor if needed
4. Update the package's `.spec.md` in the same change if behaviour changed

If a task was given as an argument to this skill, work through the read sequence then the implementation steps for it now.
