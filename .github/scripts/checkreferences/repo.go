package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type repo struct {
	root string
	// Every file git lists, tracked or untracked but not ignored, plus every directory holding one.
	isListedByPath map[string]bool
	// Blob URLs into this repository start with blobURLRoot, such as "https://github.com/o/r/blob/"
	blobURLRoot                string
	knownBranchBlobURLPrefixes []string
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
	r := newRepo(root, strings.Split(strings.TrimSpace(string(output)), "\n"))
	r.blobURLRoot = blobURLRoot(gitOutput(root, "remote", "get-url", "origin"))
	r.knownBranchBlobURLPrefixes = knownBranchBlobURLPrefixes(r.blobURLRoot, gitOutput(root, "branch", "--show-current"))
	return r, nil
}

// Empty when git fails, such as with no remote: blob URLs are then left unchecked.
func gitOutput(root string, args ...string) string {
	command := exec.Command("git", args...)
	command.Dir = root
	output, _ := command.Output()
	return strings.TrimSpace(string(output))
}

var githubRemote = regexp.MustCompile(`^(?:git@github\.com:|ssh://git@github\.com/|https://(?:[^@/]+@)?github\.com/)([^/]+/[^/]+?)(?:\.git)?$`)

// Empty for a remote not on GitHub.
func blobURLRoot(remoteURL string) string {
	match := githubRemote.FindStringSubmatch(remoteURL)
	if match == nil {
		return ""
	}
	return "https://github.com/" + match[1] + "/blob/"
}

// A branch name may hold slashes, so a blob URL's path is found by its known branch, not by position.
func knownBranchBlobURLPrefixes(root, currentBranch string) []string {
	if root == "" {
		return nil
	}
	branches := []string{"main"}
	if currentBranch != "" && currentBranch != "main" {
		branches = append(branches, currentBranch)
	}
	return utilities.Map(branches, func(branch string) string {
		return root + branch + "/"
	})
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
