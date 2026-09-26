package main

import (
	"slices"
	"testing"
)

func TestUnknownSkillAndAgentNames(t *testing.T) {
	r := newRepo("", []string{
		".agents/skills/coding/SKILL.md", ".agents/skills/plan/SKILL.md", ".agents/skills/writing/SKILL.md", ".agents/agents/reviewer.md",
	})
	markdown := "---\n" +
		"skills: [coding, writting]\n" +
		"---\n" +
		"Follow the `coding` skill's rules, then the `codding` skill.\n" +
		"Spawn the `reviewer` agent, `Explore` agents, or the `coding` agent.\n" +
		"```\nthe `nope` skill\n```\n" +
		"Use the `plan` and `writting` skills, or `coding`, `codding` and `writing` skills."

	got := unknownSkillAndAgentNames(markdown, r)

	want := []nameMention{
		{line: 2, name: "writting", kind: "skill"},
		{line: 4, name: "codding", kind: "skill"},
		{line: 5, name: "coding", kind: "agent"},
		{line: 9, name: "writting", kind: "skill"},
		{line: 9, name: "codding", kind: "skill"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFrontmatterNameMismatches(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		content     string
		wantFinding string
	}{
		{name: "skill matching its folder", path: "repo/.agents/skills/coding/SKILL.md", content: "---\nname: coding\n---\nbody"},
		{name: "agent matching its file", path: ".agents/agents/reviewer.md", content: "---\nname: reviewer\nmodel: opus\n---"},
		{name: "not a skill or agent file", path: "docs/notes.md", content: "---\nname: other\n---"},
		{
			name:        "skill named differently from its folder",
			path:        ".agents/skills/coding/SKILL.md",
			content:     "---\nname: code\n---",
			wantFinding: ".agents/skills/coding/SKILL.md:2: name `code` doesn't match `coding` from its path",
		},
		{
			name:        "agent with no name",
			path:        ".agents/agents/reviewer.md",
			content:     "---\nmodel: opus\n---\nname: reviewer",
			wantFinding: ".agents/agents/reviewer.md:1: no `name:` in the frontmatter, want `reviewer`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frontmatterNameMismatch(tt.path, tt.content); got != tt.wantFinding {
				t.Errorf("got %q, want %q", got, tt.wantFinding)
			}
		})
	}
}
