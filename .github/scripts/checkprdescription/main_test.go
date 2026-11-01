package main

import (
	"slices"
	"strings"
	"testing"
)

const validDescription = `Closes #12

## Summary

### The problem: "ready to merge" lists PRs failing CI

- An approved PR with a failing required check shows as ready to merge
- Its merge button is red, so the team stops trusting the section

### The solution: demote them to "waiting for author"

- Fetch required check status for approved, non-draft PRs
- A failing required check moves the PR to "waiting for author"
  - Its row gets a "❌ checks failing" marker

## Docs

- [019_x.md](https://github.com/o/r/blob/b/docs/plans/019_x.md)
`

func TestProblems(t *testing.T) {
	tests := []struct {
		name         string
		description  string
		wantProblems []string
	}{
		{
			name:         "valid description",
			description:  validDescription,
			wantProblems: nil,
		},
		{
			name:        "solution heading without a summary",
			description: strings.Replace(validDescription, "### The solution: demote them", "### The solution", 1),
		},
		{
			name:         "missing problem section",
			description:  "### The solution\n\n- one\n- two\n",
			wantProblems: []string{"missing heading: ### The problem"},
		},
		{
			name:         "too few problem bullets",
			description:  "### The problem\n\n- one\n\n### The solution\n\n- one\n- two\n",
			wantProblems: []string{"### The problem: 1 bullets, want 2 to 3"},
		},
		{
			name:         "too many solution bullets, sub bullets not counted",
			description:  "### The problem\n\n- one\n- two\n\n### The solution\n\n- 1\n  - 1a\n- 2\n- 3\n- 4\n- 5\n- 6\n",
			wantProblems: []string{"### The solution: 6 bullets, want 2 to 5"},
		},
		{
			name:         "prose line",
			description:  "### The problem\n\nThis PR does things.\n\n- one\n- two\n\n### The solution\n\n- one\n- two\n",
			wantProblems: []string{"### The problem: not a bullet: This PR does things."},
		},
		{
			name:        "template placeholders left in",
			description: "### The problem: <a few words summarizing it>\n\n- one\n- two\n<!-- a comment -->\n\n### The solution\n\n- one\n- two\n-\n",
			wantProblems: []string{
				"### The problem: template placeholder left in: <a few words summarizing it>",
				"### The solution: empty bullet",
				"### The problem: not a bullet: <!-- a comment -->",
			},
		},
		{
			name:         "heading summary of 9 words",
			description:  strings.Replace(validDescription, `demote them to "waiting for author"`, "one two three four five six seven eight nine", 1),
			wantProblems: []string{"### The solution: heading summary over 8 words"},
		},
		{
			name:         "long bullet, 16 words",
			description:  "### The problem\n\n- " + strings.Repeat("word ", 16) + "\n- two\n\n### The solution\n\n- one\n- two\n",
			wantProblems: []string{"### The problem: bullet over 15 words: " + strings.TrimSpace(strings.Repeat("word ", 16))},
		},
		{
			name:         "bullet of exactly 15 words",
			description:  "### The problem\n\n- " + strings.Repeat("word ", 15) + "\n- two\n\n### The solution\n\n- one\n- two\n",
			wantProblems: nil,
		},
		{
			name: "summary of 81 words",
			description: "### The problem\n\n" + strings.Repeat("- "+strings.Repeat("w ", 15)+"\n", 3) +
				"\n### The solution\n\n" + strings.Repeat("- "+strings.Repeat("w ", 15)+"\n", 2) + "- w w w w w w\n",
			wantProblems: []string{"summary is 81 words, want at most 80"},
		},
		{
			name: "related issues and PRs start with their state",
			description: validDescription + "\n## Related\n\n" +
				"- Open issue #3: flaky E2E\n" +
				"- Closed issue #4\n" +
				"- Merged PR #5\n" +
				"- Closed PR https://github.com/o/r/pull/6\n" +
				"- [facts](https://github.com/o/r/blob/b/docs/third-party-facts.md)\n",
			wantProblems: nil,
		},
		{
			name: "related issue or PR without its state",
			description: validDescription + "\n## Related\n\n" +
				"- Issue #3: flaky E2E\n" +
				"- See https://github.com/o/r/pull/6\n" +
				"- Opened PR #7\n",
			wantProblems: []string{
				"## Related: start with Open, Closed or Merged: Issue #3: flaky E2E",
				"## Related: start with Open, Closed or Merged: See https://github.com/o/r/pull/6",
				"## Related: start with Open, Closed or Merged: Opened PR #7",
			},
		},
		{
			name: "open items numbered, with sub bullets",
			description: validDescription + "\n## Open\n\n" +
				"1. Whether pending checks demote\n" +
				"   - Needs a live PR\n" +
				"2. The E2E cost\n",
			wantProblems: nil,
		},
		{
			name:         "open items as bullets",
			description:  validDescription + "\n## Open\n\n- Whether pending checks demote\n",
			wantProblems: []string{"## Open: not a numbered item: - Whether pending checks demote"},
		},
		{
			name:        "prose outside the summary",
			description: "Some intro.\n" + validDescription + "\n## Related\n\nSee issue 3.\n",
			wantProblems: []string{
				"before the first heading: not a Closes or Fixes line: Some intro.",
				"## Related: not a bullet: See issue 3.",
			},
		},
		{
			name:         "link over 150 characters",
			description:  validDescription + "- [x](https://github.com/" + strings.Repeat("a", 132) + ")\n",
			wantProblems: []string{"link over 150 characters: https://github.com/" + strings.Repeat("a", 132)},
		},
		{
			name:         "link of exactly 150 characters",
			description:  validDescription + "- [x](https://github.com/" + strings.Repeat("a", 131) + ")\n",
			wantProblems: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := problems(tt.description)
			if !slices.Equal(got, tt.wantProblems) {
				t.Errorf("problems() =\n%q\nwant\n%q", got, tt.wantProblems)
			}
		})
	}
}
