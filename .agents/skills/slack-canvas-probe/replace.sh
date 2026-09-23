#!/usr/bin/env bash
# Replace the whole content of the dev PR tracker canvas with hand-written markdown.
#
# Usage:
#   .agents/skills/slack-canvas-probe/replace.sh .local/canvas-payloads/<NNN>_<name>_canvas.md
#
# canvases.edit has no notion of a draft: every call rewrites the live canvas. There is no
# post/edit/delete split like slack-message-probe's messages, only this one replace call, run again
# each round.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$repo_root"
source .envrc

# pr-reminders-test's canvas (see .github/actions/e2e-tests/action.yml's "Run with canvas link"
# step, slack-channel-name-1). This canvas gets rewritten by every build/e2e CI run too.
CANVAS_ID="F0BMEPVR1DL"

MARKDOWN_PATH="$1" CANVAS_ID="$CANVAS_ID" TOKEN="$INPUT_SLACK_BOT_TOKEN" python3 - <<'PY'
import json, os, urllib.request

markdown = open(os.environ["MARKDOWN_PATH"]).read()

body = json.dumps({
    "canvas_id": os.environ["CANVAS_ID"],
    "changes": [{
        "operation": "replace",
        "document_content": {"type": "markdown", "markdown": markdown},
    }],
}).encode()

request = urllib.request.Request(
    "https://slack.com/api/canvases.edit",
    data=body,
    headers={
        "Authorization": "Bearer " + os.environ["TOKEN"],
        "Content-Type": "application/json; charset=utf-8",
    },
)
response = json.load(urllib.request.urlopen(request))
print(json.dumps(response, indent=2))
PY
