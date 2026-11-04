#!/usr/bin/env bash
# Runs a command, retrying it on failure: 3 attempts, 10 s then 30 s apart.
# Only for commands safe to repeat, such as a git push: GitHub sometimes rejects one with a server error.
# Usage: ./retry.sh <command> [args...]
set -u

if [[ $# -eq 0 ]]; then
    echo "Usage: $0 <command> [args...]"
    exit 1
fi

DELAYS_SECONDS=(10 30)
MAX_ATTEMPTS=$((${#DELAYS_SECONDS[@]} + 1))

for ((attempt = 1; attempt <= MAX_ATTEMPTS; attempt++)); do
    "$@" && exit 0
    exit_code=$?
    if ((attempt == MAX_ATTEMPTS)); then
        echo "Failed after $MAX_ATTEMPTS attempts: $*" >&2
        exit "$exit_code"
    fi
    delay=${DELAYS_SECONDS[attempt - 1]}
    echo "Attempt $attempt of $MAX_ATTEMPTS failed (exit $exit_code), retrying in $delay s: $*" >&2
    sleep "$delay"
done
