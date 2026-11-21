package inputhelpers

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func FuzzParseMapping(f *testing.F) {
	seeds := []string{
		"",
		"a:b;c:d",
		"a:b;c",
		" ;a:b;c: ",
		"invalid-format",
		`repo1: {"authors": ["alice"]}`,
		`repo1: {"authors": ["alice"]}; repo2: {"ignored-labels": ["wip"]}`,
		`repo1: {}`,
		`repo1: {"authors": ["alice"], "ignored-authors": ["bob"]}`,
		"alice: U123\nbob: U456\n# comment\n\n",
		"owner/repo: https://example.com:8080/path",
		"a: b\nc: d;e",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		valueByKey, err := parseMapping("test", input)
		if err != nil {
			if valueByKey != nil {
				t.Fatalf("got both an error and a mapping: %v, %v", err, valueByKey)
			}
			return
		}
		if valueByKey == nil {
			t.Fatal("got neither an error nor a mapping")
		}
		for key, value := range valueByKey {
			assertValidMappingEntry(t, key, value)
		}
		reparsed, err := parseMapping("test", formatMapping(valueByKey))
		if err != nil {
			t.Fatalf("re-parsing the formatted mapping failed: %v", err)
		}
		if !maps.Equal(reparsed, valueByKey) {
			t.Fatalf("round trip changed the mapping: %q became %q", valueByKey, reparsed)
		}
	})
}

func assertValidMappingEntry(t *testing.T, key string, value string) {
	t.Helper()
	if key == "" || value == "" {
		t.Fatalf("empty key or value: %q: %q", key, value)
	}
	if strings.TrimSpace(key) != key || strings.TrimSpace(value) != value {
		t.Fatalf("untrimmed key or value: %q: %q", key, value)
	}
	if strings.Contains(key, ":") || strings.HasPrefix(key, "#") {
		t.Fatalf("key holds a ':' or starts with '#': %q", key)
	}
	if strings.Contains(key, ";") || strings.Contains(value, ";") {
		t.Fatalf("key or value holds the ';' separator: %q: %q", key, value)
	}
}

// Ends every entry with ';', so even a single entry splits on ';' and keeps any newline in its value.
func formatMapping(valueByKey map[string]string) string {
	var formatted strings.Builder
	for _, key := range slices.Sorted(maps.Keys(valueByKey)) {
		formatted.WriteString(key + ": " + valueByKey[key] + ";")
	}
	return formatted.String()
}
