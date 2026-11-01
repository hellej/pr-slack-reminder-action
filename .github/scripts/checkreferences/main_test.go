package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Lists every file under root, as git would for a repository holding only those files.
func repoOfFilesUnder(t *testing.T, root string) repo {
	t.Helper()
	var listedFiles []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(root, path)
		listedFiles = append(listedFiles, relative)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return newRepo(root, listedFiles)
}

func TestFindings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "exists.md"), "")
	description := filepath.Join(root, "pr.md")
	writeFile(t, description, "- `docs/gone.md`\n- [typo](https://github.com/o/r/blob/claude/y/README.md)\n")
	missingRepoPath := description + ":1: `docs/gone.md`: not in the repository"
	tests := []struct {
		name            string
		isPRDescription bool
		blobURLRoot     string
		want            []string
		wantNote        string
	}{
		{
			name:        "repository paths checked, blob URLs on another branch skipped",
			blobURLRoot: "https://github.com/o/r/blob/",
			want:        []string{missingRepoPath},
		},
		{
			name:            "PR description: blob URLs on another branch checked, repository paths skipped",
			isPRDescription: true,
			blobURLRoot:     "https://github.com/o/r/blob/",
			want:            []string{description + ":2: https://github.com/o/r/blob/claude/y/README.md: not on main or the current branch"},
		},
		{
			name:            "PR description, origin not on GitHub: a note that blob URLs went unchecked",
			isPRDescription: true,
			blobURLRoot:     "",
			want:            nil,
			wantNote:        "note: blob URLs not checked: origin is missing or not a recognised GitHub URL",
		},
		{
			name:        "origin not on GitHub, outside a PR description: no note",
			blobURLRoot: "",
			want:        []string{missingRepoPath},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := repoOfFilesUnder(t, root)
			r.blobURLRoot = tt.blobURLRoot
			r.knownBranchBlobURLPrefixes = []string{"https://github.com/o/r/blob/main/"}

			got, note, err := findings([]string{description}, r, map[string]bool{}, tt.isPRDescription)

			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if note != tt.wantNote {
				t.Errorf("got note %q, want %q", note, tt.wantNote)
			}
		})
	}
}
