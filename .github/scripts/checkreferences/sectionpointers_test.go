package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSectionPointers(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		style        commentStyle
		wantPointers []sectionPointer
	}{
		{
			name:         "target file named before the pointer, text runs on",
			content:      "Branch per AGENTS.md § Git, a mandatory read",
			wantPointers: []sectionPointer{{line: 1, writtenTargetFile: "AGENTS.md", text: "Git, a mandatory read"}},
		},
		{
			name:    "chained pointers share the file named first, bold is unwrapped",
			content: "- **AGENTS.md § Purpose, § **Testing** and more",
			wantPointers: []sectionPointer{
				{line: 1, writtenTargetFile: "AGENTS.md", text: "Purpose, § **Testing** and more"},
				{line: 1, writtenTargetFile: "AGENTS.md", text: "Testing** and more"},
			},
		},
		{
			name:         "a link's target wins over its text",
			content:      "See [run.spec.md](cmd/run.spec.md#x) § Behaviour",
			wantPointers: []sectionPointer{{line: 1, writtenTargetFile: "cmd/run.spec.md", text: "Behaviour"}},
		},
		{
			name:    "a skill or agent named before the pointer targets its file",
			content: "Follow the `plan` skill § Structure, then the `reviewer` agent § Verify",
			wantPointers: []sectionPointer{
				{line: 1, writtenTargetFile: ".agents/skills/plan/SKILL.md", text: "Structure, then the `reviewer` agent § Verify"},
				{line: 1, writtenTargetFile: ".agents/agents/reviewer.md", text: "Verify"},
			},
		},
		{
			name:         "a file named earlier in the sentence, not right before the sign, doesn't count",
			content:      "Checks `docs/facts.md` and Go comments (see § References)",
			wantPointers: []sectionPointer{{line: 1, writtenTargetFile: "", text: "References)"}},
		},
		{
			name:         "a pointer in a double-backtick code span is an example",
			content:      "Write `` the `<name>` skill § <Heading> `` there",
			wantPointers: nil,
		},
		{
			name:         "no file named: the file holding the pointer, text wrapping onto the next line",
			content:      "intro\nSee § An escaped newline is a\n  line break\n\nnext paragraph",
			wantPointers: []sectionPointer{{line: 2, writtenTargetFile: "", text: "An escaped newline is a\nline break"}},
		},
		{
			name:         "a file named on the line before doesn't count",
			content:      "The facts file follows AGENTS.md\n§ Third-party Facts",
			wantPointers: []sectionPointer{{line: 2, writtenTargetFile: "", text: "Third-party Facts"}},
		},
		{
			name:         "pointers into external pages, in link text or right after the link, are skipped",
			content:      "[Docs, Memory § AGENTS.md](https://example.com/memory) and [Limits](https://example.com) § Timeouts",
			wantPointers: nil,
		},
		{
			name:         "pointers in code spans and code blocks are examples, skipped",
			content:      "e.g. `See docs/facts.md § <heading>`\n```\nAGENTS.md § Git\n```",
			wantPointers: nil,
		},
		{
			name:         "Go: full-line comments only, wrapping across comment lines",
			content:      "x := \"state.spec.md § Oddities\"\n// Falls back. See state.spec.md § Doesn't\n// Do, for more\nfunc f() {}",
			style:        slashComments,
			wantPointers: []sectionPointer{{line: 2, writtenTargetFile: "state.spec.md", text: "Doesn't\nDo, for more"}},
		},
		{
			name:         "Go: a pointer naming no file is skipped",
			content:      "// See § Oddities",
			style:        slashComments,
			wantPointers: nil,
		},
		{
			name:         "YAML, Makefile and shell: full-line # comments only",
			content:      "# See docs/facts.md § Dependabot skips\nkey: value # docs/x.md § Trailing",
			style:        hashComments,
			wantPointers: []sectionPointer{{line: 1, writtenTargetFile: "docs/facts.md", text: "Dependabot skips"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sectionPointers(tt.content, tt.style)
			if !slices.Equal(got, tt.wantPointers) {
				t.Errorf("got %#v, want %#v", got, tt.wantPointers)
			}
		})
	}
}

func TestSectionLabels(t *testing.T) {
	markdown := "# Title\n## Git\n### `go-github` v78 retries nothing [2026-09-26]\n" +
		"- **Readability > Speed:** Data sets are tiny\n```\n## Not A Heading\n```"

	got := sectionLabels(markdown)

	want := []string{"Title", "Git", "`go-github` v78 retries nothing", "Readability > Speed"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestShortened(t *testing.T) {
	long := "Output Stile now, in full, and apply it. The read is mandatory"
	if got := shortened(long + "\nnext line"); got != "Output Stile now, in full, and apply it.…" {
		t.Errorf("got %q", got)
	}
	if got := shortened("Git, a read\nnext line"); got != "Git, a read" {
		t.Errorf("got %q", got)
	}
}

func TestBrokenSectionPointers(t *testing.T) {
	root := t.TempDir()
	agentsFile := filepath.Join(root, "AGENTS.md")
	writeFile(t, agentsFile, "## Git\n## Code Style\n- **Name a map:** rule")
	planSkill := filepath.Join(root, ".agents", "skills", "plan", "SKILL.md")
	writeFile(t, planSkill, "## Structure")
	skill := filepath.Join(root, "skills", "a", "SKILL.md")
	writeFile(t, skill, "See [AGENTS.md](../../AGENTS.md) § Git, a read\n"+
		"[AGENTS.md](../../AGENTS.md) § Code Style's comment rule\n"+
		"[AGENTS.md](../../AGENTS.md) § Name a map\n"+
		"[AGENTS.md](../../AGENTS.md) § Gitt\n"+
		"[MISSING.md](../../MISSING.md) § Git\n"+
		"## Local\n"+
		"See § Local and § Nowhere\n"+
		"The `plan` skill § Structure and § Style\n"+
		"The `nope` skill § Structure")
	goSource := filepath.Join(root, "pkg", "pkg.go")
	writeFile(t, goSource, "// See pkg.spec.md § Oddities\n// See AGENTS.md § Git\npackage pkg")
	pkgSpec := filepath.Join(root, "pkg", "pkg.spec.md")
	writeFile(t, pkgSpec, "## Behaviour")
	dependabotConfig := filepath.Join(root, ".github", "dependabot.yml")
	writeFile(t, dependabotConfig, "# See AGENTS.md § Gitt\nversion: 2")

	got, err := brokenSectionPointers([]string{skill, goSource, dependabotConfig}, repoOfFilesUnder(t, root))

	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		skill + ":4: § Gitt: no heading or bold text starting it in " + agentsFile,
		skill + ":5: § Git: target file ../../MISSING.md not found",
		skill + ":7: § Nowhere: no heading or bold text starting it in " + skill,
		skill + ":8: § Style: no heading or bold text starting it in " + planSkill,
		skill + ":9: § Structure: target file .agents/skills/nope/SKILL.md not found",
		goSource + ":1: § Oddities: no heading or bold text starting it in " + pkgSpec,
		dependabotConfig + ":1: § Gitt: no heading or bold text starting it in " + agentsFile,
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
