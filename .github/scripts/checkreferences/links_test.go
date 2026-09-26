package main

import (
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
			want:     []linkTarget{{line: 2, path: "../state/state.spec.md"}},
		},
		{
			name:     "fragment kept, title dropped",
			markdown: `[a](docs/facts.md#some-heading "Title")`,
			want:     []linkTarget{{line: 1, path: "docs/facts.md", fragment: "some-heading"}},
		},
		{
			name:     "a fragment alone links into the file holding it",
			markdown: "[c](#heading)",
			want:     []linkTarget{{line: 1, path: "", fragment: "heading"}},
		},
		{
			name:     "image and angle brackets",
			markdown: "![logo](<img/logo.png>)",
			want:     []linkTarget{{line: 1, path: "img/logo.png"}},
		},
		{
			name:     "two links on one line",
			markdown: "[a](one.md) and [b](two.md)",
			want:     []linkTarget{{line: 1, path: "one.md"}, {line: 1, path: "two.md"}},
		},
		{
			name:     "percent-encoded path",
			markdown: "[a](my%20file.md)",
			want:     []linkTarget{{line: 1, path: "my file.md"}},
		},
		{
			name:     "external, mail and absolute links are skipped",
			markdown: "[a](https://example.com/x.md#y) [b](mailto:a@b.c) [d](/abs.md)",
			want:     nil,
		},
		{
			name:     "a link inside an inline code span is an example, not a link",
			markdown: "Link there, e.g. `[AGENTS.md](../../AGENTS.md)`, and [real](real.md)",
			want:     []linkTarget{{line: 1, path: "real.md"}},
		},
		{
			name:     "a link inside a double-backtick code span is skipped, one after it is not",
			markdown: "`` [a](`x`.md) `` then [b](b.md)",
			want:     []linkTarget{{line: 1, path: "b.md"}},
		},
		{
			name:     "a link inside a fenced code block is skipped",
			markdown: "```md\n[a](inside.md)\n```\n~~~\n[b](tilde.md)\n~~~\n[c](after.md)",
			want:     []linkTarget{{line: 7, path: "after.md"}},
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

func TestHeadingAnchors(t *testing.T) {
	markdown := "# 🔑 GitHub Token Setup\n## 3. Update mode enabled\n" +
		"## `go-github` v78 retries [2026-09-26]\n## Tips\n## Tips\n```\n## Not A Heading\n```"

	got := headingAnchors(markdown)

	want := []string{"-github-token-setup", "3-update-mode-enabled", "go-github-v78-retries-2026-09-26", "tips", "tips-1"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBrokenLinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "docs", "exists.md")
	writeFile(t, target, "## Real Heading")
	skill := filepath.Join(root, "skills", "a", "SKILL.md")
	writeFile(t, skill, "[ok](../../docs/exists.md)\n"+
		"[broken](../docs/exists.md)\n"+
		"[dir](../../docs)\n"+
		"[anchor](../../docs/exists.md#real-heading)\n"+
		"[bad anchor](../../docs/exists.md#fake)\n"+
		"## Own\n"+
		"[own](#own)\n"+
		"[own bad](#nope)")

	got, err := brokenLinks([]string{skill})

	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		skill + ":2: ../docs/exists.md: no such file",
		skill + ":5: ../../docs/exists.md#fake: no heading with this anchor in " + target,
		skill + ":8: #nope: no heading with this anchor in " + skill,
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestBrokenLinksFailsOnUnreadableFile(t *testing.T) {
	_, err := brokenLinks([]string{filepath.Join(t.TempDir(), "missing.md")})

	if err == nil {
		t.Error("want an error for a file that can't be read")
	}
}
