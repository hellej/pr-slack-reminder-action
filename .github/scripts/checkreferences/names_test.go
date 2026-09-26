package main

import (
	"slices"
	"testing"
)

func TestUnknownSkillAndAgentNames(t *testing.T) {
	r := newRepo("", []string{".agents/skills/coding/SKILL.md", ".agents/agents/reviewer.md"})
	markdown := "---\n" +
		"skills: [coding, writting]\n" +
		"---\n" +
		"Follow the `coding` skill's rules, then the `codding` skill.\n" +
		"Spawn the `reviewer` agent, `Explore` agents, or the `coding` agent.\n" +
		"```\nthe `nope` skill\n```"

	got := unknownSkillAndAgentNames(markdown, r)

	want := []nameMention{
		{line: 2, name: "writting", kind: "skill"},
		{line: 4, name: "codding", kind: "skill"},
		{line: 5, name: "coding", kind: "agent"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
