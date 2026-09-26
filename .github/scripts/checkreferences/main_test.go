package main

import (
	"io/fs"
	"os"
	"path/filepath"
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
