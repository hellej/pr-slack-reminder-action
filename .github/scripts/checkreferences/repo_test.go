package main

import (
	"slices"
	"testing"
)

func TestBlobURLPrefixes(t *testing.T) {
	tests := []struct {
		name          string
		remoteURL     string
		currentBranch string
		want          []string
	}{
		{
			name:          "SSH remote, on a feature branch",
			remoteURL:     "git@github.com:o/r.git",
			currentBranch: "claude/x",
			want:          []string{"https://github.com/o/r/blob/main/", "https://github.com/o/r/blob/claude/x/"},
		},
		{
			name:          "HTTPS remote without .git, on main",
			remoteURL:     "https://github.com/o/r",
			currentBranch: "main",
			want:          []string{"https://github.com/o/r/blob/main/"},
		},
		{
			name:          "HTTPS remote with .git, detached HEAD",
			remoteURL:     "https://github.com/o/r.git",
			currentBranch: "",
			want:          []string{"https://github.com/o/r/blob/main/"},
		},
		{
			name:          "remote not on GitHub",
			remoteURL:     "https://gitlab.com/o/r.git",
			currentBranch: "main",
			want:          nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blobURLPrefixes(tt.remoteURL, tt.currentBranch)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
