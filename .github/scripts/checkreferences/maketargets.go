package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// A rule line: targets, then a colon that doesn't start `:=`. Recipes start with a tab, special targets with a dot.
	makefileRule   = regexp.MustCompile(`^([A-Za-z0-9_-][A-Za-z0-9_. -]*?)[ \t]*:([^=]|$)`)
	makeTargetName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
)

func loadMakeTargets(repoRoot string) (map[string]bool, error) {
	makefile, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		return nil, fmt.Errorf("reading Makefile: %w", err)
	}
	isTargetByName := map[string]bool{}
	for _, target := range makefileTargets(string(makefile)) {
		isTargetByName[target] = true
	}
	return isTargetByName, nil
}

func makefileTargets(makefile string) []string {
	var targets []string
	for _, line := range strings.Split(makefile, "\n") {
		if match := makefileRule.FindStringSubmatch(line); match != nil {
			targets = append(targets, strings.Fields(match[1])...)
		}
	}
	return targets
}

func brokenMakeTargets(paths []string, isTargetByName map[string]bool) ([]string, error) {
	var broken []string
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		for _, mention := range missingMakeTargets(string(content), commentStyleOf(path), isTargetByName) {
			broken = append(broken, fmt.Sprintf("%s:%d: `make %s`: no such Makefile target", path, mention.line, mention.name))
		}
	}
	return broken, nil
}

// Reads backticked `make <target>...` commands, skipping flags and VAR=value arguments.
func missingMakeTargets(content string, style commentStyle, isTargetByName map[string]bool) []nameMention {
	var missing []nameMention
	for i, line := range proseLines(content, style) {
		for _, span := range codeSpans(line) {
			words := strings.Fields(span.content)
			if len(words) == 0 || words[0] != "make" {
				continue
			}
			for _, word := range words[1:] {
				if makeTargetName.MatchString(word) && !isTargetByName[word] {
					missing = append(missing, nameMention{line: i + 1, name: word, kind: "make target"})
				}
			}
		}
	}
	return missing
}
