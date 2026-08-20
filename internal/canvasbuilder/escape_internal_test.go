package canvasbuilder

import "testing"

func TestEscapeMarkdown(t *testing.T) {
	testCases := []struct {
		name string
		text string
		want string
	}{
		{name: "nothing to escape", text: "Add pagination", want: "Add pagination"},
		{name: "emphasis", text: "Add _debug_ flag", want: `Add \_debug\_ flag`},
		{name: "bold", text: "**WIP** rewrite", want: `\*\*WIP\*\* rewrite`},
		{name: "code span", text: "Use `make test`", want: "Use \\`make test\\`"},
		{name: "link label", text: "Fix [ABC-123] crash", want: `Fix \[ABC-123\] crash`},
		{name: "strikethrough", text: "Remove ~~legacy~~ shim", want: `Remove \~\~legacy\~\~ shim`},
		{name: "autolink", text: "See <https://example.com>", want: `See \<https://example.com\>`},
		{name: "html entity", text: "Fix &amp; in output", want: `Fix \&amp; in output`},
		// A backslash escaped last would escape the backslashes the other replacements just added.
		{name: "backslash before an escapable character", text: `C:\_path`, want: `C:\\\_path`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapeMarkdown(tc.text); got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
