package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

type nameMention struct {
	line int
	name string
	kind string
}

var skillsFrontmatter = regexp.MustCompile(`^skills:\s*\[([^\]]*)\]`)

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
	for _, match := range backtickedSkillOrAgent.FindAllStringSubmatch(line, -1) {
		mentions = append(mentions, nameMention{line: lineNumber, name: match[1], kind: match[2]})
	}
	return mentions
}

func isKnown(mention nameMention, r repo) bool {
	if mention.kind == "skill" {
		return r.isListed(skillFile(mention.name))
	}
	return isBuiltInByAgentName[mention.name] || r.isListed(agentFile(mention.name))
}
