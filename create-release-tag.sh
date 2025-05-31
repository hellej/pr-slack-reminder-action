#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "Usage: $0 [v0|v1|v2|...] [patch|minor|major]"
    exit 1
fi

CURRENT_MAJOR_VERSION="$1"
if [[ ! "$CURRENT_MAJOR_VERSION" =~ ^v[0-9]+$ ]]; then
    echo "Error: First argument must be a version prefix like v0, v1, v2, etc."
    exit 1
fi 

INCREMENT_TYPE="$2"
if [[ "$INCREMENT_TYPE" != "patch" && "$INCREMENT_TYPE" != "minor" && "$INCREMENT_TYPE" != "major" ]]; then
    echo "Error: Argument must be one of: patch, minor, major"
    exit 1
fi

get_latest_tag() {
    git tag --list "$1.*.*" | tail -n 1
}

LATEST_TAG=$(get_latest_tag $CURRENT_MAJOR_VERSION)
echo "Latest tag: $LATEST_TAG"

increment_patch_version() {
    local CURRENT_VERSION="$1"
    local patch=$(cut -d'.' -f3 <<< "$CURRENT_VERSION")
    ((patch++))
    echo "${CURRENT_VERSION%.*}.$patch"
}

increment_minor_version() {
    local CURRENT_VERSION="$1"
    local major=$(cut -d'.' -f1 <<< "$CURRENT_VERSION")
    local minor=$(cut -d'.' -f2 <<< "$CURRENT_VERSION")
    ((minor++))
    echo "$major.$minor.0"
}

increment_major_version() {
    local CURRENT_VERSION="$1"
    local major=$(cut -d'.' -f1 <<< "$CURRENT_VERSION")
    local major_num="${major#v}"
    ((major_num++))
    echo "v${major_num}.0.0"
}

if [[ "$LATEST_TAG" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    case "$INCREMENT_TYPE" in
        patch)
            NEW_VERSION=$(increment_patch_version "$LATEST_TAG")
            ;;
        minor)
            NEW_VERSION=$(increment_minor_version "$LATEST_TAG")
            ;;
        major)
            NEW_VERSION=$(increment_major_version "$LATEST_TAG")
            ;;
    esac
    # git tag "$NEW_VERSION"
    # git push --tags
    echo "New version: $NEW_VERSION"
else
    echo "No valid tags found"
    exit 1
fi
