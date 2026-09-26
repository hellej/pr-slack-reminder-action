package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type repo struct {
	root string
	// Every file git lists, tracked or untracked but not ignored, plus every directory holding one.
	isListedByPath map[string]bool
}

// Claude Code's own agent types, which live outside .agents/.
var isBuiltInByAgentName = map[string]bool{"Explore": true, "general-purpose": true, "Plan": true}

func loadRepo(root string) (repo, error) {
	listFiles := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard")
	listFiles.Dir = root
	output, err := listFiles.Output()
	if err != nil {
		return repo{}, fmt.Errorf("listing repository files: %w", err)
	}
	return newRepo(root, strings.Split(strings.TrimSpace(string(output)), "\n")), nil
}

func newRepo(root string, listedFiles []string) repo {
	isListedByPath := map[string]bool{}
	for _, file := range listedFiles {
		for path := filepath.ToSlash(file); path != "." && path != ""; path = filepath.Dir(path) {
			isListedByPath[path] = true
		}
	}
	return repo{root: root, isListedByPath: isListedByPath}
}

func (r repo) isListed(path string) bool {
	return r.isListedByPath[path]
}

func skillFile(name string) string {
	return ".agents/skills/" + name + "/SKILL.md"
}

func agentFile(name string) string {
	return ".agents/agents/" + name + ".md"
}
