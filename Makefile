GO_BUILD=go build -ldflags="-s -w"
MAIN_GO=./cmd/pr-slack-reminder/main.go

run:
	env \
	'INPUT_GITHUB-TOKEN=$(INPUT_GITHUB_TOKEN)' \
	'GITHUB_REPOSITORY=$(GITHUB_REPOSITORY)' \
	'INPUT_SLACK-BOT-TOKEN=$(INPUT_SLACK_BOT_TOKEN)' \
	'INPUT_SLACK-CHANNEL-NAME=$(INPUT_SLACK_CHANNEL_NAME)' \
	go run $(MAIN_GO)

build-darwin-amd64:
	env GOOS=darwin GOARCH=amd64 $(GO_BUILD) -o main-darwin-amd64 $(MAIN_GO)

build-darwin-arm64:
	env GOOS=darwin GOARCH=arm64 $(GO_BUILD) -o main-darwin-arm64 $(MAIN_GO)

build-linux-amd64:
	env GOOS=linux GOARCH=amd64 $(GO_BUILD) -o main-linux-amd64 $(MAIN_GO)

build-linux-arm64:
	env GOOS=linux GOARCH=arm64 $(GO_BUILD) -o main-linux-arm64 $(MAIN_GO)

build-windows-amd64:
	env GOOS=windows GOARCH=amd64 $(GO_BUILD) -o main-windows-amd64 $(MAIN_GO)

build-windows-arm64:
	env GOOS=windows GOARCH=arm64 $(GO_BUILD) -o main-windows-arm64 $(MAIN_GO)

build-all: 
	$(MAKE) build-linux-amd64
	$(MAKE) build-linux-arm64
	$(MAKE) build-windows-amd64
	$(MAKE) build-windows-arm64
