#!/usr/bin/env bash
# PreToolUse hook on Bash (see .claude/settings.json).
# Blocks a `git commit` without a `-- <paths>` pathspec, per AGENTS.md § Git: the index is
# shared with other sessions, so a bare commit takes whatever they staged.
# Matches inside compound commands too, so it doesn't use the `if` filter.

# Flattened, since grep matches per line and a multi-line commit message puts `--` on a later one
command=$(jq -r '.tool_input.command // empty' | tr '\n' ' ')

commit_onwards=$(echo "$command" | grep -oE 'git( -C [^ ]+)? commit.*' | head -n1)
if [ -z "$commit_onwards" ]; then
  exit 0
fi
if echo "$commit_onwards" | grep -qE '[[:space:]]--([[:space:]]|$)'; then
  exit 0
fi

reason="Commit with an explicit pathspec: git commit -m \"...\" -- <paths>. A bare git commit takes everything staged, including other sessions' work (AGENTS.md § Git)."
jq -n --arg reason "$reason" '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason: $reason}}'
exit 0
