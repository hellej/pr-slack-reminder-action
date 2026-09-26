package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type nameMention struct {
	line int
	name string
	kind string
}

var (
	skillsFrontmatter = regexp.MustCompile(`^skills:\s*\[([^\]]*)\]`)
	// One or more backticked names joined by commas, "and" or "or", then `skill` or `agent`: "the `plan` and `writing` skills".
	backtickedNamesBeforeKind = regexp.MustCompile("((?:`[A-Za-z0-9-]+`(?:, and |, or |, | and | or ))*`[A-Za-z0-9-]+`) (skill|agent)")
	backtickedName            = regexp.MustCompile("`([A-Za-z0-9-]+)`")
	skillDefinitionPath       = regexp.MustCompile(`(^|/)\.agents/skills/([^/]+)/SKILL\.md$`)
	agentDefinitionPath       = regexp.MustCompile(`(^|/)\.agents/agents/([^/]+)\.md$`)
)

func brokenSkillAndAgentNames(paths []string, r repo) ([]string, error) {
	var broken []string
	for _, markdownPath := range pathsWithStyle(paths, markdownProse) {
		content, err := os.ReadFile(markdownPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", markdownPath, err)
		}
		for _, mention := range unknownSkillAndAgentNames(string(content), r) {
			broken = append(broken, fmt.Sprintf("%s:%d: `%s` %s: not in .agents/", markdownPath, mention.line, mention.name, mention.kind))
		}
	}
	return broken, nil
}

func unknownSkillAndAgentNames(markdown string, r repo) []nameMention {
	var unknown []nameMention
	for i, line := range linesOutsideCodeBlocks(markdown) {
		for _, mention := range namesOnLine(line, i+1) {
			if !isKnown(mention, r) {
				unknown = append(unknown, mention)
			}
		}
	}
	return unknown
}

func namesOnLine(line string, lineNumber int) []nameMention {
	var mentions []nameMention
	if match := skillsFrontmatter.FindStringSubmatch(line); match != nil {
		for _, name := range strings.Split(match[1], ",") {
			mentions = append(mentions, nameMention{line: lineNumber, name: strings.TrimSpace(name), kind: "skill"})
		}
	}
	for _, match := range backtickedNamesBeforeKind.FindAllStringSubmatch(line, -1) {
		for _, name := range backtickedName.FindAllStringSubmatch(match[1], -1) {
			mentions = append(mentions, nameMention{line: lineNumber, name: name[1], kind: match[2]})
		}
	}
	return mentions
}

func isKnown(mention nameMention, r repo) bool {
	if mention.kind == "skill" {
		return r.isListed(skillFile(mention.name))
	}
	return isBuiltInByAgentName[mention.name] || r.isListed(agentFile(mention.name))
}

func brokenFrontmatterNames(paths []string) ([]string, error) {
	var broken []string
	for _, markdownPath := range pathsWithStyle(paths, markdownProse) {
		content, err := os.ReadFile(markdownPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", markdownPath, err)
		}
		if finding := frontmatterNameMismatch(markdownPath, string(content)); finding != "" {
			broken = append(broken, finding)
		}
	}
	return broken, nil
}

// Other files point to a skill or agent by its path's name, so its `name:` must be the same.
func frontmatterNameMismatch(path, content string) string {
	wantName, isDefinition := definitionName(filepath.ToSlash(path))
	if !isDefinition {
		return ""
	}
	missingName := fmt.Sprintf("%s:1: no `name:` in the frontmatter, want `%s`", path, wantName)
	lines := strings.Split(content, "\n")
	if lines[0] != "---" {
		return missingName
	}
	for i, line := range lines[1:] {
		if line == "---" {
			break
		}
		name, isName := strings.CutPrefix(line, "name:")
		if !isName {
			continue
		}
		if name = strings.TrimSpace(name); name != wantName {
			return fmt.Sprintf("%s:%d: name `%s` doesn't match `%s` from its path", path, i+2, name, wantName)
		}
		return ""
	}
	return missingName
}

func definitionName(path string) (string, bool) {
	for _, definitionPath := range []*regexp.Regexp{skillDefinitionPath, agentDefinitionPath} {
		if match := definitionPath.FindStringSubmatch(path); match != nil {
			return match[2], true
		}
	}
	return "", false
}
