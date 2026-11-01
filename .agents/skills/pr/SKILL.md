---
name: pr
description: "Open or update a GitHub pull request with a description that follows the repo's PR template. Use when: opening a PR, writing or editing a PR description, or the user runs /pr."
argument-hint: "Optional: draft, or what the PR is for"
---

# Open a Pull Request

- Merge the latest `main` first (AGENTS.md § Git)
- Fill [the PR template](../../../.github/pull_request_template.md). Drop a section that would be empty
- Summarize in a few bullets. Don't copy a plan or spec into the description
- List every `.md` file the PR adds or changes under `## Docs`, each linked on the PR's branch:
  `[019_x.md](https://github.com/<owner>/<repo>/blob/<branch>/<path>)`
- Keep each link under 150 characters. See docs/third-party-facts.md § The GitHub MCP tools wrap a PR description URL over about 150 characters in backticks, breaking the link
- Read the stored description back after writing it
- A linked file renamed or moved later means editing its link
- A plan's draft PR usually gets the implementation pushed onto it later. Then rewrite the description to cover the whole PR: the Summary, every `.md` file such as the specs, and what is still open
