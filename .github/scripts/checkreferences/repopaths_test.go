package main

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestMissingRepoPaths(t *testing.T) {
	r := newRepo("", []string{"internal/state/state.go", "AGENTS.md", ".github/scripts/check-style.sh", "docs/plans/001_x.md"})
	markdown := "See `internal/state/`, `./internal/state`, `.github/scripts/check-style.sh` and `docs/plans/`\n" +
		"Moved: `internal/gone/`, `.github/scripts/old.sh`\n" +
		"Not repo paths: `owner/name`, `/search/issues`, `internal/<pkg>/`, `internal/**/*.go`, `./...`, " +
		"`slack-go@v0.29.0/chat.go`, `https://x/y`, `.local/plans/`, `go test ./internal/state`\n" +
		"```\n`internal/gone/`\n```\n" +
		"`` `internal/gone/` is an example `` and ``internal/lost/``"

	got := missingRepoPaths(markdown, markdownProse, r)

	want := []repoPathMention{{line: 2, writtenPath: "internal/gone/"}, {line: 2, writtenPath: ".github/scripts/old.sh"}, {line: 7, writtenPath: "internal/lost/"}}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMissingRepoPathsInCodeComments(t *testing.T) {
	r := newRepo("", []string{"internal/state/state.go"})
	goSource := "// Reads `internal/state/` and `internal/gone/`\nx := \"`internal/lost/`\""
	shellScript := "# Runs `internal/gone.sh`\necho `internal/lost.sh`"

	gotGo := missingRepoPaths(goSource, slashComments, r)
	gotShell := missingRepoPaths(shellScript, hashComments, r)

	if want := []repoPathMention{{line: 1, writtenPath: "internal/gone/"}}; !slices.Equal(gotGo, want) {
		t.Errorf("Go: got %v, want %v", gotGo, want)
	}
	if want := []repoPathMention{{line: 1, writtenPath: "internal/gone.sh"}}; !slices.Equal(gotShell, want) {
		t.Errorf("shell: got %v, want %v", gotShell, want)
	}
}

func TestBrokenRepoPathsSkipsTheFactsFile(t *testing.T) {
	root := t.TempDir()
	agentsFile := filepath.Join(root, "AGENTS.md")
	writeFile(t, agentsFile, "`internal/gone/`")
	writeFile(t, filepath.Join(root, "docs", "third-party-facts.md"), "Claude Code reads `.claude/CLAUDE.md`")
	writeFile(t, filepath.Join(root, ".claude", "settings.json"), "{}")
	writeFile(t, filepath.Join(root, "internal", "x.go"), "package internal")

	got, err := brokenRepoPaths([]string{agentsFile, filepath.Join(root, "docs", "third-party-facts.md")}, repoOfFilesUnder(t, root))

	if err != nil {
		t.Fatal(err)
	}
	want := []string{agentsFile + ":1: `internal/gone/`: not in the repository"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
