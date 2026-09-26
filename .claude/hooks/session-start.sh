#!/usr/bin/env bash
# Web sessions start from a fresh clone, so the pre-commit hook is opted into for them.
# Local clones keep `make install-hooks` as their own opt-in.
[ "${CLAUDE_CODE_REMOTE:-}" = "true" ] || exit 0
cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null && make --silent install-hooks >/dev/null 2>&1
exit 0
