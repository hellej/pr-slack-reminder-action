#!/usr/bin/env bash
set -euo pipefail

# Parse CLI flags for non-interactive mode
SEMVER=""
COMMIT_BINARY=""
AUTO_YES=false

usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --semver <patch|minor|major>  Version bump level"
    echo "  --commit-binary               Commit built binary to repository"
    echo "  --no-commit-binary            Do not commit built binary"
    echo "  --yes                         Auto-confirm proceed prompts"
    echo "  --help                        Show this help message"
    echo ""
    echo "With no options, runs interactively (prompts for all values)."
    echo "Partial options are supported: only provided flags skip their prompts."
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --semver)
            if [[ -z "${2:-}" ]]; then
                echo "Error: --semver requires a value (patch, minor, or major)"
                exit 1
            fi
            SEMVER="$2"
            if [[ "$SEMVER" != "patch" && "$SEMVER" != "minor" && "$SEMVER" != "major" ]]; then
                echo "Error: Invalid semver option '$SEMVER'. Must be 'patch', 'minor', or 'major'"
                exit 1
            fi
            shift 2
            ;;
        --commit-binary)
            COMMIT_BINARY="true"
            shift
            ;;
        --no-commit-binary)
            COMMIT_BINARY="false"
            shift
            ;;
        --yes)
            AUTO_YES=true
            shift
            ;;
        --help)
            usage
            exit 0
            ;;
        *)
            echo "Error: Unknown option '$1'"
            usage
            exit 1
            ;;
    esac
done

if ! command -v gh &> /dev/null; then
    echo "Error: GitHub CLI (gh) is not installed"
    echo "Install it from: https://cli.github.com/"
    exit 1
fi

if ! gh auth status &> /dev/null; then
    echo "Error: Not authenticated with GitHub CLI"
    echo "Run: gh auth login"
    exit 1
fi

REMOTE_URL=$(git remote get-url origin)
if [[ "$REMOTE_URL" =~ github\.com[:/]([^/]+)/([^/.]+)(\.git)?$ ]]; then
    REPO_OWNER="${BASH_REMATCH[1]}"
    REPO_NAME="${BASH_REMATCH[2]}"
    REPO="$REPO_OWNER/$REPO_NAME"
else
    echo "Error: Could not parse GitHub repository from remote URL: $REMOTE_URL"
    exit 1
fi

echo "Repository: $REPO"
echo ""

echo "Fetching latest changes from origin..."
git fetch origin main --quiet
git fetch --tags --prune-tags --force origin --quiet

LOCAL_MAIN=$(git rev-parse main 2>/dev/null || echo "")
REMOTE_MAIN=$(git rev-parse origin/main 2>/dev/null || echo "")

if [[ -z "$LOCAL_MAIN" || -z "$REMOTE_MAIN" ]]; then
    echo "Error: Could not resolve local or remote main branch"
    exit 1
fi

if [[ "$LOCAL_MAIN" != "$REMOTE_MAIN" ]]; then
    echo "Error: Local main branch is not in sync with origin/main"
    echo "Local:  $LOCAL_MAIN"
    echo "Remote: $REMOTE_MAIN"
    echo "Run: git pull origin main"
    exit 1
fi

echo "✓ Local main is in sync with origin/main"
echo ""

get_latest_tag() {
    git ls-remote --tags origin | awk '{print $2}' | grep -o 'refs/tags/v[0-9]*\.[0-9]*\.[0-9]*$' | sed 's_refs/tags/v__g' | sort -V | tail -n 1 | awk '{print "v"$1}'
}

LATEST_TAG=$(get_latest_tag)

if [[ -z "$LATEST_TAG" ]]; then
    echo "Error: No valid semver tags found"
    exit 1
fi

TAG_DATE=$(git log -1 --format=%ai "$LATEST_TAG" 2>/dev/null || echo "unknown")

echo "Latest release: $LATEST_TAG ($TAG_DATE)"
echo ""

COMMIT_COUNT=$(git rev-list --count "$LATEST_TAG..origin/main" 2>/dev/null || echo "0")

if [[ "$COMMIT_COUNT" -eq 0 ]]; then
    echo "⚠️  No commits to release since $LATEST_TAG"
    echo ""
    read -p "Proceed anyway? (y/n): " PROCEED
    if [[ "$PROCEED" != "y" ]]; then
        echo "Cancelled."
        exit 0
    fi
else
    echo "Commits since $LATEST_TAG ($COMMIT_COUNT commits):"
    echo ""
    git --no-pager log "$LATEST_TAG..origin/main" --format="%ai %h %s (%an)"
    echo ""
    if [[ "$AUTO_YES" == true ]]; then
        echo "Proceeding (--yes)"
    else
        read -p "Proceed with release? (y/n): " PROCEED
        if [[ "$PROCEED" != "y" ]]; then
            echo "Cancelled."
            exit 0
        fi
    fi
fi

if [[ -z "$SEMVER" ]]; then
    echo ""
    read -p "Select version bump (patch/minor/major) or 'exit' to cancel: " SEMVER

    if [[ -z "$SEMVER" || "$SEMVER" == "exit" ]]; then
        echo "Cancelled."
        exit 0
    fi

    if [[ "$SEMVER" != "patch" && "$SEMVER" != "minor" && "$SEMVER" != "major" ]]; then
        echo "Error: Invalid semver option. Must be 'patch', 'minor', or 'major'"
        exit 1
    fi
else
    echo ""
    echo "Version bump: $SEMVER (from --semver flag)"
fi

if [[ -z "$COMMIT_BINARY" ]]; then
    echo ""
    read -p "Commit the built binary to repository (for release)? (y/n): " COMMIT_BINARY_CHOICE

    if [[ "$COMMIT_BINARY_CHOICE" == "y" ]]; then
        COMMIT_BINARY="true"
    else
        COMMIT_BINARY="false"
    fi
else
    echo "Commit binary: $COMMIT_BINARY (from flag)"
fi

echo ""
echo "Triggering release workflow with semver=$SEMVER, commit-binary=$COMMIT_BINARY..."

gh workflow run release.yml -f semver="$SEMVER" -f commit-binary="$COMMIT_BINARY" --repo "$REPO"

if [[ $? -ne 0 ]]; then
    echo "Error: Failed to trigger workflow"
    exit 1
fi

echo "Waiting for workflow to start..."
sleep 2

RUN_URL=$(gh run list --workflow=release.yml --limit=1 --json url --jq '.[0].url' --repo "$REPO" 2>/dev/null || echo "")

echo ""
if [[ -n "$RUN_URL" ]]; then
    echo "✅ Release workflow triggered successfully!"
    echo "🚀 Workflow URL: $RUN_URL"
else
    echo "✅ Release workflow triggered successfully!"
    echo "🚀 View workflow runs at: https://github.com/$REPO/actions/workflows/release.yml"
fi
