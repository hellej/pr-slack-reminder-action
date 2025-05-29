GO_BUILD=go build -ldflags="-s -w"
MAIN_GO=./cmd/pr-slack-reminder/main.go
VERSION := $(shell git rev-parse --short=10 HEAD)

test:
	go test ./...

run:
	env \
	'INPUT_GITHUB-TOKEN=$(INPUT_GITHUB_TOKEN)' \
	'GITHUB_REPOSITORY=$(GITHUB_REPOSITORY)' \
	'INPUT_SLACK-BOT-TOKEN=$(INPUT_SLACK_BOT_TOKEN)' \
	'INPUT_SLACK-CHANNEL-NAME=$(INPUT_SLACK_CHANNEL_NAME)' \
	'INPUT_GITHUB-USER-SLACK-USER-ID-MAPPING=$(INPUT_GITHUB_USER_SLACK_USER_ID_MAPPING)' \
	go run $(MAIN_GO)

build-darwin-amd64:
	env GOOS=darwin GOARCH=amd64 $(GO_BUILD) -o dist/main-darwin-amd64-$(VERSION) $(MAIN_GO)

build-darwin-arm64:
	env GOOS=darwin GOARCH=arm64 $(GO_BUILD) -o dist/main-darwin-arm64-$(VERSION) $(MAIN_GO)

build-linux-amd64:
	env GOOS=linux GOARCH=amd64 $(GO_BUILD) -o dist/main-linux-amd64-$(VERSION) $(MAIN_GO)

build-linux-arm64:
	env GOOS=linux GOARCH=arm64 $(GO_BUILD) -o dist/main-linux-arm64-$(VERSION) $(MAIN_GO)

build-windows-amd64:
	env GOOS=windows GOARCH=amd64 $(GO_BUILD) -o dist/main-windows-amd64-$(VERSION) $(MAIN_GO)

build-windows-arm64:
	env GOOS=windows GOARCH=arm64 $(GO_BUILD) -o dist/main-windows-arm64-$(VERSION) $(MAIN_GO)

update-invoke-binary-targets:
	@echo "Updating invoke binary targets..."
	@sed -i '' "s|^const VERSION = '.*'|const VERSION = '$(VERSION)'|" invoke-binary.js

build-all: 
	$(MAKE) build-linux-amd64
	$(MAKE) build-linux-arm64
	$(MAKE) build-windows-amd64
	$(MAKE) build-windows-arm64
	$(MAKE) update-invoke-binary-targets
