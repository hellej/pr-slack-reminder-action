---
name: spec-writer
description: "Write or update <package>.spec.md files documenting an internal Go package's behaviour, non-goals, and oddities. Use when: writing a package spec, documenting a directory's responsibilities, creating/updating a spec file, or asked to explain what a package does and doesn't do."
argument-hint: "Optional: directory path(s) to write specs for. Defaults to any internal/ package missing a spec file."
---

# Package Spec Files

## Purpose

A spec file in a package directory lets an agent learn that package's responsibilities and gaps by reading one short file, instead of re-reading and re-reasoning about the source every time.

## When to Use

- A Go package directory (one containing `.go` files directly — not an umbrella folder that only holds subpackages) has no spec file
- A package's code changed and its spec file no longer matches
- User asks to document, spec out, or explain what a package does

## Format

One spec file per Go package directory, at its root, named `<dirname>.spec.md` (e.g. `internal/state/state.spec.md`, `internal/apiclients/githubclient/githubclient.spec.md`) — an umbrella directory with no `.go` files of its own (like `internal/apiclients/`) gets no spec file; each of its subpackages gets its own. Exactly these headings, in order, all as bullet points (no prose paragraphs):

```markdown
# <package name>

## Behaviour

- ...

## Doesn't Do

- ...

## Oddities

- ...
```

Omit a heading's body and the heading itself if it has no genuine content — never write "None" or "N/A".

### Behaviour
- What the package is responsible for and does, as observable through its exported functions/types — its business logic and contract, not its implementation
- One bullet per capability, not per line of code
- Name exported functions/types where it helps lookup; don't name unexported helpers
- Skip mechanism (which concurrency primitive, which internal helper, how a value is threaded through private functions) unless that mechanism is itself part of the contract (e.g. a documented concurrency limit callers may rely on)

### Doesn't Do
- Non-obvious non-goals: unhandled cases, missing validation, things a reader might assume it does but doesn't — framed at the same public-interface level as Behaviour
- Skip anything already obvious from the Behaviour section stated negatively

### Oddities
- Surprising behaviour, implicit coupling between packages, non-obvious ordering/mutation, footguns — anything a caller of the public interface could be bitten by
- This is the one section where implementation detail belongs, if that detail explains a surprising externally-visible effect
- Known rough edges *as they exist today* — not suggested fixes or TODOs
- Sibling `_test.go` files' table-driven test case names (`grep -n 'name:'` or `t.Run(`) are a fast way to surface edge cases worth a bullet here, without reading the whole test file

## Rules

- Follow AGENTS.md output style: plain words, short bullets, no filler, no duplication
- Describe current state only — never proposed changes, TODOs, or "should" statements
- Business logic over implementation: describe what the package's public interface does for a caller, not internal data flow, private helper names, or which library/pattern it's built with. Implementation detail is fair game only in Oddities, and only when it's needed to explain a surprising externally-visible effect
- Don't restate what's already in the root `AGENTS.md` (pipeline order, architecture) — link there (`[AGENTS.md](../../AGENTS.md)`) instead
- Don't restate a concern owned by another package's spec — reference that package by path instead of duplicating
- Any link to `AGENTS.md` or another spec file must be a relative path (e.g. `[internal/state](../../state/state.spec.md)`) — not absolute or bare — so it stays clickable when the file is viewed on GitHub
- Base every bullet on the actual source in the directory, not on the package name or prior assumptions

## Maintenance

Update a package's spec file in the same change that changes its behaviour — don't let it drift out of sync with the code.
