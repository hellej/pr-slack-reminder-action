TEST=go test ./...
GO_BUILD=go build -ldflags="-s -w"
MAIN_GO=./cmd/pr-slack-reminder
COMMIT_HASH := $(shell git rev-parse --short=10 HEAD)
SEMVER =


test:
	$(TEST)

clean-test-cache:
	go clean -testcache
	go clean -cache
	@echo "Cleared Go test and build caches"

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
	'INPUT_MAIN-LIST-HEADING=$(INPUT_MAIN_LIST_HEADING)' \
	'INPUT_OLD-PR-THRESHOLD-HOURS=$(INPUT_OLD_PR_THRESHOLD_HOURS)' \
	'INPUT_NO-PRS-MESSAGE=$(INPUT_NO_PRS_MESSAGE)' \
	'INPUT_PR-LINK-REPO-PREFIXES=$(INPUT_PR_LINK_REPO_PREFIXES)' \
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

build-windows-amd64:
	env GOOS=windows GOARCH=amd64 $(GO_BUILD) -o dist/main-windows-amd64-$(COMMIT_HASH) $(MAIN_GO)

build-windows-arm64:
	env GOOS=windows GOARCH=arm64 $(GO_BUILD) -o dist/main-windows-arm64-$(COMMIT_HASH) $(MAIN_GO)

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

release:
	@if [ -z "$(SEMVER)" ]; then \
		echo "Usage: make release SEMVER=[patch|minor|major]"; \
		exit 1; \
	fi; \
	./create-release-tag.sh $(SEMVER)
