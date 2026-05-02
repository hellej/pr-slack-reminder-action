---
name: release
description: "Create a new release of pr-slack-reminder-action. Use when: releasing, bumping version, creating a release tag, publishing a new version, making a patch release, minor release, or major release. Guides through semver selection, CI checks, and the full release pipeline."
argument-hint: "Describe what changed to help determine the semver bump level"
---

# Release pr-slack-reminder-action

## When to Use

- Publishing a new version of the action
- Creating a release after merging features or fixes
- User asks to "release", "bump version", "tag a release", or "publish"

## Procedure

### 1. Determine Semver Bump

Skip this step if the user already specified the bump level (e.g., "make a patch release").

| Bump | When |
|------|------|
| **patch** | Bug fixes, documentation, dependency updates, internal refactors with no behavior change |
| **minor** | New inputs/features, new optional functionality, non-breaking enhancements |
| **major** | Breaking changes: renamed/removed inputs, changed default behavior, incompatible config format |

Review commits since the last tag to help decide:

```bash
git fetch --tags origin
LATEST_TAG=$(git ls-remote --tags origin | awk '{print $2}' | grep -o 'refs/tags/v[0-9]*\.[0-9]*\.[0-9]*$' | sed 's_refs/tags/v__g' | sort -V | tail -n 1 | awk '{print "v"$1}')
git --no-pager log "$LATEST_TAG..HEAD" --oneline
```

### 2. Run the Release

Run the release workflow via the Make target. This is an **interactive script** that prompts for input multiple times — run it in an interactive terminal session.

```bash
make release-workflow
```

This runs `trigger-release-workflow.sh`, which:
1. Verifies local `main` is in sync with `origin/main`
2. Shows the latest tag and commits since then
3. Prompts for semver bump level (patch/minor/major) — if already determined in step 1 or by the user, provide it directly without re-asking
4. Asks whether to commit the built binary to the repository — answer based on whether the compiled binary would change. Check which files changed since the last tag:
   ```bash
   git diff --name-only "$(git ls-remote --tags origin | awk '{print $2}' | grep -o 'refs/tags/v[0-9]*\.[0-9]*\.[0-9]*$' | sed 's_refs/tags/v__g' | sort -V | tail -n 1 | awk '{print "v"$1}')"..HEAD
   ```
   - **Yes** if any `.go` files, `go.mod`, `go.sum`, `Makefile`, or `invoke-binary.js` changed (including `go.mod`/`go.sum`-only changes like Go version bumps, which affect the compiled binary)
   - **No** if only docs, GitHub Actions workflows, or other non-code files changed
5. Triggers the `release.yml` GitHub Actions workflow

The workflow handles everything remotely: unit tests → build → e2e tests → `make release-tag` → `make draft-release` → Slack notification. Do **not** run build or test locally — the workflow does this.

### 3. Wait for Workflow Completion

The full pipeline takes several minutes. The draft release will **not** appear on GitHub until all steps pass.

The trigger script prints the workflow run URL which contains the run ID. Use `gh run watch` to wait for completion:

```bash
gh run watch <run-id> --exit-status 2>&1 | tail -20
```

- If the run **succeeds**, proceed to step 4.
- If it **failed**, show the user the failure and offer to open the logs:
  ```bash
  GH_PAGER="" gh run view <run-id> --log-failed
  ```
  Do not proceed with publishing.

### 4. Publish the Release

Once the workflow completes successfully:

1. Get the tag of the draft release:
   ```bash
   GH_PAGER="" gh release list --limit=1 --json tagName,isDraft --jq '.[0].tagName'
   ```
2. Open it in the browser so the user can review the release notes:
   ```bash
   gh release view <tag> --web
   ```
3. Ask the user whether to publish the release now
4. If yes:
   ```bash
   gh release edit <tag> --draft=false
   ```
