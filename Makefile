TEST=go test -race ./...
GO_BUILD=go build -ldflags="-s -w"
MAIN_GO=./cmd/pr-slack-reminder
COMMIT_HASH := $(shell git rev-parse --short=10 HEAD)
SNAPSHOT_PACKAGES=./cmd/pr-slack-reminder ./internal/canvasbuilder
SNAPSHOT_DIRS=cmd/pr-slack-reminder/testdata internal/canvasbuilder/testdata
SEMVER =


test:
	$(TEST)

check-fmt:
	@set -e; \
	unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needs to run on:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

check-vet:
	go vet ./...

check-dead-code:
	@set -e; \
	findings=$$(go run golang.org/x/tools/cmd/deadcode@latest ./cmd/...); \
	if [ -n "$$findings" ]; then \
		echo "unreachable functions:"; \
		echo "$$findings"; \
		exit 1; \
	fi

check-vulnerabilities:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

lint: check-fmt check-vet check-dead-code check-vulnerabilities

install-hooks:
	git config core.hooksPath githooks

clean-test-cache:
	go clean -testcache
	go clean -cache
	@echo "Cleared Go test and build caches"

update-test-snapshots:
	go test $(SNAPSHOT_PACKAGES) -count=1 -update-snapshots
	@git add -N $(SNAPSHOT_DIRS) && git diff --stat -- $(SNAPSHOT_DIRS)

test-with-coverage: clean-test-cache
	$(TEST) -coverprofile=coverage.out -covermode=atomic -coverpkg=./cmd/...,./internal/...
	go tool cover -func=coverage.out

publish-code-coverage:
	goveralls -coverprofile=coverage.out -service=github

run:
	env \
	'GITHUB_REPOSITORY=$(GITHUB_REPOSITORY)' \
	'INPUT_GITHUB-TOKEN=$(INPUT_GITHUB_TOKEN)' \
	'INPUT_SLACK-BOT-TOKEN=$(INPUT_SLACK_BOT_TOKEN)' \
	'INPUT_RUN-MODE=$(INPUT_RUN_MODE)' \
	'INPUT_STATE-ARTIFACT-NAME=$(INPUT_STATE_ARTIFACT_NAME)' \
	'INPUT_GITHUB-REPOSITORIES=$(INPUT_GITHUB_REPOSITORIES)' \
	'INPUT_SLACK-CHANNEL-NAME=$(INPUT_SLACK_CHANNEL_NAME)' \
	'INPUT_GITHUB-USER-SLACK-USER-ID-MAPPING=$(INPUT_GITHUB_USER_SLACK_USER_ID_MAPPING)' \
	'INPUT_PR-LIST-HEADING=$(INPUT_PR_LIST_HEADING)' \
	'INPUT_OLD-PR-THRESHOLD-HOURS=$(INPUT_OLD_PR_THRESHOLD_HOURS)' \
	'INPUT_NO-PRS-MESSAGE=$(INPUT_NO_PRS_MESSAGE)' \
	'INPUT_GROUP-BY-REPOSITORY=$(INPUT_GROUP_BY_REPOSITORY)' \
	go run $(MAIN_GO)

build-darwin-amd64:
	env GOOS=darwin GOARCH=amd64 $(GO_BUILD) -o dist/main-darwin-amd64-$(COMMIT_HASH) $(MAIN_GO)

build-darwin-arm64:
	env GOOS=darwin GOARCH=arm64 $(GO_BUILD) -o dist/main-darwin-arm64-$(COMMIT_HASH) $(MAIN_GO)

build-linux-amd64:
	env GOOS=linux GOARCH=amd64 $(GO_BUILD) -o dist/main-linux-amd64-$(COMMIT_HASH) $(MAIN_GO)

build-linux-arm64:
	env GOOS=linux GOARCH=arm64 $(GO_BUILD) -o dist/main-linux-arm64-$(COMMIT_HASH) $(MAIN_GO)

update-invoke-binary-targets:
	@echo "Updating executable versions to $(COMMIT_HASH) in invoke-binary.js"
	@case "$$(uname)" in \
		Darwin) sed -i '' "s|^const VERSION = '.*'|const VERSION = '$(COMMIT_HASH)'|" ./invoke-binary.js ;; \
		*) sed -i "s|^const VERSION = '.*'|const VERSION = '$(COMMIT_HASH)'|" ./invoke-binary.js ;; \
	esac

build:
	rm -rf dist/*
	make build-linux-amd64
	make build-linux-arm64
	make update-invoke-binary-targets

release-tag:
	@if [ -z "$(SEMVER)" ]; then \
		echo "Usage: make release SEMVER=[patch|minor|major]"; \
		exit 1; \
	fi; \
	./create-release-tag.sh $(SEMVER)

draft-release:
	./create-draft-release.sh
