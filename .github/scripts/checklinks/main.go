// Prints every relative Markdown link, in the files given as arguments, whose target doesn't exist.
// Run from the repository root by check-style.sh.
package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type linkTarget struct {
	line   int
	target string
}

var (
	inlineCodeSpan = regexp.MustCompile("`[^`]*`")
	// Matches [text](target), ![alt](target), <target> in angle brackets, and an optional "title".
	markdownLink  = regexp.MustCompile(`!?\[[^\]]*\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)`)
	urlWithScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
)

func main() {
	broken, err := brokenLinks(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	for _, finding := range broken {
		fmt.Println(finding)
	}
}

func brokenLinks(markdownPaths []string) ([]string, error) {
	var broken []string
	for _, markdownPath := range markdownPaths {
		content, err := os.ReadFile(markdownPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", markdownPath, err)
		}
		for _, link := range relativeLinkTargets(string(content)) {
			resolved := filepath.Join(filepath.Dir(markdownPath), link.target)
			if _, err := os.Stat(resolved); err != nil {
				broken = append(broken, fmt.Sprintf("%s:%d: %s", markdownPath, link.line, link.target))
			}
		}
	}
	return broken, nil
}

// Skips code blocks and code spans: a link there is an example, not a link.
func relativeLinkTargets(markdown string) []linkTarget {
	var targets []linkTarget
	isInCodeBlock := false
	for i, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			isInCodeBlock = !isInCodeBlock
			continue
		}
		if isInCodeBlock {
			continue
		}
		proseOnly := inlineCodeSpan.ReplaceAllString(line, "")
		for _, match := range markdownLink.FindAllStringSubmatch(proseOnly, -1) {
			if target, isRelative := relativePath(match[1]); isRelative {
				targets = append(targets, linkTarget{line: i + 1, target: target})
			}
		}
	}
	return targets
}

func relativePath(target string) (string, bool) {
	if urlWithScheme.MatchString(target) || strings.HasPrefix(target, "/") {
		return "", false
	}
	path, _, _ := strings.Cut(target, "#")
	path, _, _ = strings.Cut(path, "?")
	if path == "" {
		return "", false
	}
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	return path, true
}
