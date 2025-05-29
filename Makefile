MAIN_GO=./cmd/pr-slack-reminder/main.go

run:
	env \
	'INPUT_GITHUB-TOKEN=$(INPUT_GITHUB_TOKEN)' \
	'GITHUB_REPOSITORY=$(GITHUB_REPOSITORY)' \
	'INPUT_SLACK-BOT-TOKEN=${INPUT_SLACK_BOT_TOKEN}' \
	'INPUT_SLACK-CHANNEL-NAME=${INPUT_SLACK_CHANNEL_NAME}' \
	go run $(MAIN_GO)

build-darwin-arm64:
	env GOOS=darwin GOARCH=arm64 go build -o main-darwin-arm64 $(MAIN_GO)

build-linux-amd64:
	env GOOS=linux GOARCH=amd64 go build -o main-linux-amd64 $(MAIN_GO)

build-linux-arm64:
	env GOOS=linux GOARCH=arm64 go build -o main-linux-arm64 $(MAIN_GO)

build-all: build-darwin-arm64 build-linux-amd64 build-linux-arm64
