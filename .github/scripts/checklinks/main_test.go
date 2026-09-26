package main

import (
	"os"
	"path/filepath"
	"slices"
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
