module github.com/hellej/pr-slack-reminder-action/tools

go 1.26.0

tool (
	github.com/mattn/goveralls
	golang.org/x/tools/cmd/deadcode
	golang.org/x/vuln/cmd/govulncheck
)

require (
	github.com/mattn/goveralls v0.0.12 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/telemetry v0.0.0-20260924152758-ed294f943157 // indirect
	golang.org/x/tools v0.51.0 // indirect
	golang.org/x/vuln v1.8.0 // indirect
)
