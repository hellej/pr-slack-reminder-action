package main

import (
	"maps"
	"slices"
	"testing"
)

func TestReadmeInputNames(t *testing.T) {
	readme := "## Usage\n| `not-an-input` | x |\n\n## ➡️ Inputs\n\n| Input | Required | Description |\n| --- | --- | --- |\n" +
		"| `slack-bot-token`   | ✅ | Token<br>Example: `${{ secrets.X }}` |\n| `run-mode` | ❌ | `post` or `update` |\n\n" +
		"Text mentioning `mentioned-input`.\n\n## Outputs\n| `an-output` | x |"

	got := readmeInputNames(readme)

	want := []string{"run-mode", "slack-bot-token"}
	if gotNames := slices.Sorted(maps.Keys(got)); !slices.Equal(gotNames, want) {
		t.Errorf("got %v, want %v", gotNames, want)
	}
}
