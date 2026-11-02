package main

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLinkTargetsInRepo(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     []linkTarget
	}{
		{
			name:     "plain link",
			markdown: "intro\nsee [spec](../state/state.spec.md) here",
			want:     []linkTarget{{line: 2, path: "../state/state.spec.md", text: "spec"}},
		},
		{
			name:     "fragment kept, title dropped",
			markdown: `[a](docs/facts.md#some-heading "Title")`,
			want:     []linkTarget{{line: 1, path: "docs/facts.md", fragment: "some-heading", text: "a"}},
		},
		{
			name:     "a fragment alone links into the file holding it",
			markdown: "[c](#heading)",
			want:     []linkTarget{{line: 1, path: "", fragment: "heading", text: "c"}},
		},
		{
			name:     "image and angle brackets",
			markdown: "![logo](<img/logo.png>)",
			want:     []linkTarget{{line: 1, path: "img/logo.png", text: "logo", isImage: true}},
		},
		{
			name:     "two links on one line",
			markdown: "[a](one.md) and [b](two.md)",
			want:     []linkTarget{{line: 1, path: "one.md", text: "a"}, {line: 1, path: "two.md", text: "b"}},
		},
		{
			name:     "percent-encoded path",
			markdown: "[a](my%20file.md)",
			want:     []linkTarget{{line: 1, path: "my file.md", text: "a"}},
		},
		{
			name:     "external, mail and absolute links are skipped",
			markdown: "[a](https://example.com/x.md#y) [b](mailto:a@b.c) [d](/abs.md)",
			want:     nil,
		},
		{
			name:     "a link inside an inline code span is an example, not a link",
			markdown: "Link there, e.g. `[AGENTS.md](../../AGENTS.md)`, and [real](real.md)",
			want:     []linkTarget{{line: 1, path: "real.md", text: "real"}},
		},
		{
			name:     "a link inside a double-backtick code span is skipped, one after it is not",
			markdown: "`` [a](`x`.md) `` then [b](b.md)",
			want:     []linkTarget{{line: 1, path: "b.md", text: "b"}},
		},
		{
			name:     "a blob URL into this repository links from the repository root",
			markdown: "[a](https://github.com/o/r/blob/claude/x/docs/facts.md#heading) [b](https://github.com/o/r/blob/main/README.md)",
			want: []linkTarget{
				{line: 1, path: "docs/facts.md", fragment: "heading", text: "a", isPathFromRepoRoot: true},
				{line: 1, path: "README.md", text: "b", isPathFromRepoRoot: true},
			},
		},
		{
			name:     "a blob URL into another repository or branch is skipped",
			markdown: "[a](https://github.com/o/other/blob/main/README.md) [b](https://github.com/o/r/blob/old/README.md)",
			want:     nil,
		},
		{
			name:     "a link inside a fenced code block is skipped",
			markdown: "```md\n[a](inside.md)\n```\n~~~\n[b](tilde.md)\n~~~\n[c](after.md)",
			want:     []linkTarget{{line: 7, path: "after.md", text: "c"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := linkTargetsInRepo(tt.markdown, []string{"https://github.com/o/r/blob/main/", "https://github.com/o/r/blob/claude/x/"})
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeadingTextByAnchor(t *testing.T) {
	markdown := "# 🔑 GitHub Token Setup\n## 3. Update mode enabled\n" +
		"## `go-github` v78 retries [2026-09-26]\n## Tips\n## Tips\n```\n## Not A Heading\n```"

	got := headingTextByAnchor(markdown)

	want := map[string]string{
		"-github-token-setup":              "🔑 GitHub Token Setup",
		"3-update-mode-enabled":            "3. Update mode enabled",
		"go-github-v78-retries-2026-09-26": "`go-github` v78 retries [2026-09-26]",
		"tips":                             "Tips",
		"tips-1":                           "Tips",
	}
	if !maps.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBrokenLinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "docs", "exists.md")
	writeFile(t, target, "## Real Heading")
	writeFile(t, filepath.Join(root, "docs", "facts.md"), "## Fact [2026-09-26]")
	skill := filepath.Join(root, "skills", "a", "SKILL.md")
	writeFile(t, skill, "[ok](../../docs/exists.md)\n"+
		"[broken](../docs/exists.md)\n"+
		"[dir](../../docs)\n"+
		"[exists.md § Real Heading](../../docs/exists.md#real-heading)\n"+
		"[bad anchor](../../docs/exists.md#fake)\n"+
		"## Own\n"+
		"[§ Own](#own)\n"+
		"[own bad](#nope)\n"+
		"[blob](https://github.com/o/r/blob/main/docs/exists.md#real-heading)\n"+
		"[blob bad anchor](https://github.com/o/r/blob/main/docs/exists.md#fake)\n"+
		"[blob missing](https://github.com/o/r/blob/main/docs/gone.md)\n"+
		"[wrong text](../../docs/exists.md#real-heading)\n"+
		"[own](#own)\n"+
		"![image text is free](../../docs/exists.md#real-heading)\n"+
		"[exists.md § Real](../../docs/exists.md#real-heading)\n"+
		"[facts.md § Fact](../../docs/facts.md#fact-2026-09-26)\n"+
		"[exists.md § Real Heading](../../docs/exists.md)")
	r := newRepo(root, nil)
	r.knownBranchBlobURLPrefixes = []string{"https://github.com/o/r/blob/main/"}

	got, err := brokenLinks([]string{skill}, r)

	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		skill + ":2: ../docs/exists.md: no such file",
		skill + ":5: ../../docs/exists.md#fake: no heading with this anchor in " + target,
		skill + ":8: #nope: no heading with this anchor in " + skill,
		skill + ":10: docs/exists.md#fake: no heading with this anchor in " + target,
		skill + ":11: docs/gone.md: no such file",
		skill + ":12: ../../docs/exists.md#real-heading: link text should be \"exists.md § Real Heading\"",
		skill + ":13: #own: link text should be \"§ Own\"",
		skill + ":15: ../../docs/exists.md#real-heading: link text should be \"exists.md § Real Heading\"",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestBrokenLinksInReadme(t *testing.T) {
	root := t.TempDir()
	readme := filepath.Join(root, "README.md")
	writeFile(t, readme, "## Intro\n[free text](#intro)\n[§ Intro](#intro)\n[§ Wrong](#intro)\n[Intro § free](#intro)")

	got, err := brokenLinks([]string{readme}, newRepo(root, nil))

	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		readme + ":4: #intro: link text should be \"§ Intro\"",
		readme + ":5: #intro: link text should be \"§ Intro\"",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestBrokenLinksFailsOnUnreadableFile(t *testing.T) {
	_, err := brokenLinks([]string{filepath.Join(t.TempDir(), "missing.md")}, newRepo("", nil))

	if err == nil {
		t.Error("want an error for a file that can't be read")
	}
}

func TestBlobURLsOnOtherBranches(t *testing.T) {
	description := filepath.Join(t.TempDir(), "pr.md")
	writeFile(t, description, "[ok](https://github.com/o/r/blob/main/README.md)\n"+
		"[ok](https://github.com/o/r/blob/claude/x/README.md)\n"+
		"[typo](https://github.com/o/r/blob/claude/y/README.md)\n"+
		"[other repo](https://github.com/o/other/blob/claude/y/README.md)\n"+
		"[short SHA](https://github.com/o/r/blob/8370f2b/README.md)\n"+
		"[full SHA](https://github.com/o/r/blob/8370f2b0c4e1d2a3b4c5d6e7f8091a2b3c4d5e6f/README.md)\n"+
		"[tag](https://github.com/o/r/blob/v1.0.0/README.md)\n"+
		"[6 hex characters, a branch](https://github.com/o/r/blob/cafe12/README.md)\n"+
		"[41 hex characters, a branch](https://github.com/o/r/blob/8370f2b0c4e1d2a3b4c5d6e7f8091a2b3c4d5e6f0/README.md)")
	tests := []struct {
		name        string
		blobURLRoot string
		want        []string
	}{
		{
			name:        "a branch other than main and the current one",
			blobURLRoot: "https://github.com/o/r/blob/",
			want: []string{
				description + ":3: https://github.com/o/r/blob/claude/y/README.md: not on main or the current branch",
				description + ":8: https://github.com/o/r/blob/cafe12/README.md: not on main or the current branch",
				description + ":9: https://github.com/o/r/blob/8370f2b0c4e1d2a3b4c5d6e7f8091a2b3c4d5e6f0/README.md: not on main or the current branch",
			},
		},
		{
			name:        "origin not on GitHub",
			blobURLRoot: "",
			want:        nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRepo("", nil)
			r.blobURLRoot = tt.blobURLRoot
			r.knownBranchBlobURLPrefixes = []string{"https://github.com/o/r/blob/main/", "https://github.com/o/r/blob/claude/x/"}

			got, err := blobURLsOnOtherBranches([]string{description}, r)

			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}
