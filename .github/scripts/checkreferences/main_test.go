package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRelativeLinkTargets(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     []linkTarget
	}{
		{
			name:     "plain link",
			markdown: "intro\nsee [spec](../state/state.spec.md) here",
			want:     []linkTarget{{line: 2, target: "../state/state.spec.md"}},
		},
		{
			name:     "fragment and title are dropped",
			markdown: `[a](docs/facts.md#some-heading "Title")`,
			want:     []linkTarget{{line: 1, target: "docs/facts.md"}},
		},
		{
			name:     "image and angle brackets",
			markdown: "![logo](<img/logo.png>)",
			want:     []linkTarget{{line: 1, target: "img/logo.png"}},
		},
		{
			name:     "two links on one line",
			markdown: "[a](one.md) and [b](two.md)",
			want:     []linkTarget{{line: 1, target: "one.md"}, {line: 1, target: "two.md"}},
		},
		{
			name:     "percent-encoded path",
			markdown: "[a](my%20file.md)",
			want:     []linkTarget{{line: 1, target: "my file.md"}},
		},
		{
			name:     "external, mail, anchor-only and absolute links are skipped",
			markdown: "[a](https://example.com/x.md) [b](mailto:a@b.c) [c](#heading) [d](/abs.md)",
			want:     nil,
		},
		{
			name:     "a link inside an inline code span is an example, not a link",
			markdown: "Link there, e.g. `[AGENTS.md](../../AGENTS.md)`, and [real](real.md)",
			want:     []linkTarget{{line: 1, target: "real.md"}},
		},
		{
			name:     "a link inside a fenced code block is skipped",
			markdown: "```md\n[a](inside.md)\n```\n~~~\n[b](tilde.md)\n~~~\n[c](after.md)",
			want:     []linkTarget{{line: 7, target: "after.md"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := relativeLinkTargets(tt.markdown)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBrokenLinks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "exists.md"), "")
	writeFile(t, filepath.Join(root, "skills", "a", "SKILL.md"),
		"[ok](../../docs/exists.md)\n[broken](../docs/exists.md)\n[dir](../../docs)")

	got, err := brokenLinks([]string{filepath.Join(root, "skills", "a", "SKILL.md")})

	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "skills", "a", "SKILL.md") + ":2: ../docs/exists.md"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBrokenLinksFailsOnUnreadableFile(t *testing.T) {
	_, err := brokenLinks([]string{filepath.Join(t.TempDir(), "missing.md")})

	if err == nil {
		t.Error("want an error for a file that can't be read")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSectionPointers(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		isGoSource   bool
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
			name:         "no file named: the file holding the pointer, text wrapping onto the next line",
			content:      "intro\nSee § An escaped newline is a\n  line break\n\nnext paragraph",
			wantPointers: []sectionPointer{{line: 2, writtenTargetFile: "", text: "An escaped newline is a\nline break"}},
		},
		{
			name:         "file named at the end of the previous line",
			content:      "The facts file follows AGENTS.md\n§ Third-party Facts. Reading it",
			wantPointers: []sectionPointer{{line: 2, writtenTargetFile: "AGENTS.md", text: "Third-party Facts. Reading it"}},
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
			isGoSource:   true,
			wantPointers: []sectionPointer{{line: 2, writtenTargetFile: "state.spec.md", text: "Doesn't\nDo, for more"}},
		},
		{
			name:         "Go: a pointer naming no file is skipped",
			content:      "// See § Oddities",
			isGoSource:   true,
			wantPointers: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sectionPointers(tt.content, tt.isGoSource)
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

func TestBrokenSectionPointers(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "AGENTS.md")
	writeFile(t, target, "## Git\n## Code Style\n- **Name a map:** rule")
	skill := filepath.Join(root, "skills", "a", "SKILL.md")
	writeFile(t, skill, "See [AGENTS.md](../../AGENTS.md) § Git, a read\n"+
		"[AGENTS.md](../../AGENTS.md) § Code Style's comment rule\n"+
		"[AGENTS.md](../../AGENTS.md) § Name a map\n"+
		"[AGENTS.md](../../AGENTS.md) § Gitt\n"+
		"[MISSING.md](../../MISSING.md) § Git\n"+
		"## Local\nSee § Local and § Nowhere")
	goSource := filepath.Join(root, "pkg", "pkg.go")
	writeFile(t, goSource, "// See pkg.spec.md § Oddities\n// See AGENTS.md § Git\npackage pkg")
	writeFile(t, filepath.Join(root, "pkg", "pkg.spec.md"), "## Behaviour")

	got, err := brokenSectionPointers([]string{skill, goSource}, root)

	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		skill + ":4: § Gitt: no heading or bold text starting it in " + target,
		skill + ":5: § Git: target file ../../MISSING.md not found",
		skill + ":7: § Nowhere: no heading or bold text starting it in " + skill,
		goSource + ":1: § Oddities: no heading or bold text starting it in " + filepath.Join(root, "pkg", "pkg.spec.md"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
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
