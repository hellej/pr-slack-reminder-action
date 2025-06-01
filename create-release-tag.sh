#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 [patch|minor|major]"
    exit 1
fi

INCREMENT_TYPE="$1"
if [[ "$INCREMENT_TYPE" != "patch" && "$INCREMENT_TYPE" != "minor" && "$INCREMENT_TYPE" != "major" ]]; then
    echo "Error: Argument must be one of: patch, minor, major"
    exit 1
fi

get_latest_tag() {
    git ls-remote --tags origin | awk '{print $2}' | grep -o 'refs/tags/v[0-9]*\.[0-9]*\.[0-9]*$' | sed 's_refs/tags/v__g' | sort -V | tail -n 1 | awk '{print "v"$1}'
}

LATEST_TAG=$(get_latest_tag)
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

if [[ ! "$LATEST_TAG" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: No valid tags found"
    exit 1
fi

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

if [[ ! "$NEW_VERSION" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: New version is invalid: $NEW_VERSION (not vX.X.X)"
    exit 1
fi

echo "New version: $NEW_VERSION"
git tag -a "$NEW_VERSION" -m "${NEW_VERSION#v}"
git push --tags

# Below we also move the major version tag to the new version as suggested in
# https://github.com/actions/toolkit/blob/master/docs/action-versioning.md#recommendations

MAJOR_VERSION=$(echo "$NEW_VERSION" | grep -o '^v[0-9]\+')
echo "Major version: $MAJOR_VERSION"

if [[ ! "$MAJOR_VERSION" =~ ^v[0-9]+$ ]]; then
    echo "Error: New major version tag does not start with 'v' followed by a number"
    exit 1
fi

if [[ "$MAJOR_VERSION" == "v0" ]]; then
    TAG="v1-beta"
    git tag -f -a $TAG -m "${NEW_VERSION#v}"
    git push origin $TAG --force
    echo "Pushed tag: $TAG"
else
    git tag -f -a $MAJOR_VERSION -m "${NEW_VERSION#v}"
    git push origin $MAJOR_VERSION --force
    echo "Pushed tag: $MAJOR_VERSION"
fi
