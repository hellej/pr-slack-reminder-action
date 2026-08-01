#!/usr/bin/env bash
# PreToolUse hook, fires only on `git commit *` (see .claude/settings.json).
# Nudges (non-blocking) when staged internal/**/*.go changes have no staged
# <dirname>.spec.md update, per .agents/skills/spec-writer/SKILL.md.

staged=$(git diff --cached --name-only --diff-filter=ACMR)

go_dirs=$(echo "$staged" | grep -E '^internal/.*\.go$' | grep -v '_test\.go$' | xargs -r -n1 dirname | sort -u)

missing=""
for d in $go_dirs; do
  if ! echo "$staged" | grep -qE "^${d}/[^/]+\.spec\.md$"; then
    missing="$missing $d"
  fi
done

if [ -n "$missing" ]; then
  msg="Staged Go changes in:$missing have no staged spec update. If behaviour changed, update that package's <dirname>.spec.md before committing (.agents/skills/spec-writer/SKILL.md). Skip for pure refactors."
  jq -n --arg msg "$msg" '{systemMessage: $msg, hookSpecificOutput: {hookEventName: "PreToolUse", additionalContext: $msg}}'
fi

exit 0
