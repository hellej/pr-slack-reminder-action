package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type repoPathMention struct {
	line        int
	writtenPath string
}

var (
	// Placeholders, globs, Go package patterns, versioned module paths, URLs and commands.
	notARepoPath = regexp.MustCompile(`[\s<>*{}$"'\[\]()@:|=]|\.\.\.`)
)

func brokenRepoPaths(paths []string, r repo) ([]string, error) {
	var broken []string
	for _, path := range paths {
		// It quotes paths from third-party docs, such as Claude Code's .claude/CLAUDE.md.
		if filepath.Base(path) == "third-party-facts.md" {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		for _, mention := range missingRepoPaths(string(content), commentStyleOf(path), r) {
			broken = append(broken, fmt.Sprintf("%s:%d: `%s`: not in the repository", path, mention.line, mention.writtenPath))
		}
	}
	return broken, nil
}

func missingRepoPaths(content string, style commentStyle, r repo) []repoPathMention {
	var missing []repoPathMention
	for i, line := range proseLines(content, style) {
		for _, span := range codeSpans(line) {
			path, isRepoPath := repoPathIn(span.content, r)
			if isRepoPath && !r.isListed(path) {
				missing = append(missing, repoPathMention{line: i + 1, writtenPath: span.content})
			}
		}
	}
	return missing
}

// Only a path whose first part is a top-level entry of the repository: most other slashed names are GitHub repos, API routes or third-party source files.
func repoPathIn(codeSpan string, r repo) (string, bool) {
	if !strings.Contains(codeSpan, "/") || notARepoPath.MatchString(codeSpan) {
		return "", false
	}
	path := strings.TrimSuffix(strings.TrimPrefix(codeSpan, "./"), "/")
	firstPart, _, _ := strings.Cut(path, "/")
	return path, r.isListed(firstPart)
}
