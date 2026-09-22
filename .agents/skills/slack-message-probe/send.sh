#!/usr/bin/env bash
# Post, edit or delete a hand-written Block Kit payload in the dev Slack channel.
#
# Usage:
#   .agents/skills/slack-message-probe/send.sh .local/message-payloads/<NNN>_<name>_payload.json          # post
#   .agents/skills/slack-message-probe/send.sh .local/message-payloads/<NNN>_<name>_payload.json edit     # edit the last post
#   .agents/skills/slack-message-probe/send.sh .local/message-payloads/<NNN>_<name>_payload.json delete   # delete it
#
# The payload is the chat.postMessage body: channel, text, blocks. A post records the channel ID
# and ts in <payload>.sent, which edit and delete read: chat.update rejects a channel name.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$repo_root"
source .envrc

PAYLOAD="$1" ACTION="${2:-post}" TOKEN="$INPUT_SLACK_BOT_TOKEN" python3 - <<'PY'
import json, os, urllib.request

payload_path = os.environ["PAYLOAD"]
action = os.environ["ACTION"]
sent_path = payload_path + ".sent"

body = json.load(open(payload_path))
method = {"post": "chat.postMessage", "edit": "chat.update", "delete": "chat.delete"}[action]

if action != "post":
    sent = json.load(open(sent_path))
    body["channel"], body["ts"] = sent["channel"], sent["ts"]
if action == "delete":
    body = {"channel": body["channel"], "ts": body["ts"]}

request = urllib.request.Request(
    "https://slack.com/api/" + method,
    data=json.dumps(body).encode(),
    headers={
        "Authorization": "Bearer " + os.environ["TOKEN"],
        "Content-Type": "application/json; charset=utf-8",
    },
)
response = json.load(urllib.request.urlopen(request))

if response.get("ok") and action == "post":
    json.dump({"channel": response["channel"], "ts": response["ts"]}, open(sent_path, "w"))
if response.get("ok") and action == "delete":
    os.remove(sent_path)

print(json.dumps({k: response.get(k) for k in ("ok", "ts", "channel", "error", "response_metadata") if k in response}, indent=2))
PY
