---
name: pr
description: "Open or update a GitHub pull request with a description that follows the repo's PR template. Use when: opening a PR, writing or editing a PR description, or the user runs /pr."
argument-hint: "Optional: draft, or what the PR is for"
---

# Open a Pull Request

1. Merge the latest `main` (AGENTS.md § Git)
2. Write the description to `.local/pr/<branch>.md`, from [the PR template](../../../.github/pull_request_template.md)
3. Run `make check-pr-description FILE=.local/pr/<branch>.md`. Fix and rerun until it prints nothing
4. Create or edit the PR from that file: `gh pr create --body-file <file>`, or `gh pr edit`. With the GitHub MCP tools, pass the file's content, then read the stored description back

## The Description

- Bullets only, no prose. Don't copy a plan or spec into it
- `### The problem: <a few words>`, then 2 to 3 short bullets
- `### The solution: <a few words>`, then 2 to 5 short bullets, with a few short sub bullets where needed. When a few words can't sum it up, the heading is just `### The solution`
- When the PR resolves an issue, start with `Closes #<issue>`. Use `Fixes #<issue>` for a bug
- Under `## Docs`, link every `.md` file the PR adds or changes, on the PR's branch:
  `[019_x.md](https://github.com/<owner>/<repo>/blob/<branch>/<path>)`
- Drop a template section that would be empty
- [checkprdescription.spec.md](../../../.github/scripts/checkprdescription/checkprdescription.spec.md) has the word and link limits

## Later Changes

- A linked file renamed or moved means editing its link
- A plan's draft PR usually gets the implementation pushed onto it later. Then rewrite the description to cover the whole PR: the summary, every `.md` file such as the specs, and what is still open
