package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

type linkTarget struct {
	line int
	// Empty for a link to a heading of the file holding it.
	path     string
	fragment string
}

var (
	// Matches [text](target), ![alt](target), <target> in angle brackets, and an optional "title".
	markdownLink    = regexp.MustCompile(`!?\[[^\]]*\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)`)
	urlWithScheme   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	markdownHeading = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*$`)
)

func brokenLinks(paths []string) ([]string, error) {
	var broken []string
	for _, markdownPath := range pathsWithStyle(paths, markdownProse) {
		content, err := os.ReadFile(markdownPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", markdownPath, err)
		}
		for _, link := range relativeLinkTargets(string(content)) {
			finding, err := checkLink(markdownPath, link)
			if err != nil {
				return nil, err
			}
			if finding != "" {
				broken = append(broken, finding)
			}
		}
	}
	return broken, nil
}

func checkLink(holdingPath string, link linkTarget) (string, error) {
	written := link.path
	if link.fragment != "" {
		written += "#" + link.fragment
	}
	location := fmt.Sprintf("%s:%d: %s", holdingPath, link.line, written)
	targetPath := holdingPath
	if link.path != "" {
		targetPath = filepath.Join(filepath.Dir(holdingPath), link.path)
	}
	if _, err := os.Stat(targetPath); err != nil {
		return location + ": no such file", nil
	}
	if link.fragment == "" || filepath.Ext(targetPath) != ".md" {
		return "", nil
	}
	target, err := os.ReadFile(targetPath)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", targetPath, err)
	}
	if !slices.Contains(headingAnchors(string(target)), link.fragment) {
		return fmt.Sprintf("%s: no heading with this anchor in %s", location, targetPath), nil
	}
	return "", nil
}

// Skips code blocks and code spans: a link there is an example, not a link.
func relativeLinkTargets(markdown string) []linkTarget {
	var targets []linkTarget
	for i, line := range linesOutsideCodeBlocks(markdown) {
		for _, match := range markdownLink.FindAllStringSubmatch(withoutCodeSpans(line), -1) {
			if path, fragment, isRelative := splitRelativeLink(match[1]); isRelative {
				targets = append(targets, linkTarget{line: i + 1, path: path, fragment: fragment})
			}
		}
	}
	return targets
}

func splitRelativeLink(target string) (path, fragment string, isRelative bool) {
	if urlWithScheme.MatchString(target) || strings.HasPrefix(target, "/") {
		return "", "", false
	}
	path, fragment, _ = strings.Cut(target, "#")
	path, _, _ = strings.Cut(path, "?")
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	return path, fragment, path != "" || fragment != ""
}

// A repeated heading gets `-1`, `-2` and so on, as GitHub numbers them.
func headingAnchors(markdown string) []string {
	var anchors []string
	seenCountByAnchor := map[string]int{}
	for _, line := range linesOutsideCodeBlocks(markdown) {
		match := markdownHeading.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		anchor := githubAnchor(match[1])
		if seenCount := seenCountByAnchor[anchor]; seenCount > 0 {
			anchors = append(anchors, fmt.Sprintf("%s-%d", anchor, seenCount))
		} else {
			anchors = append(anchors, anchor)
		}
		seenCountByAnchor[anchor]++
	}
	return anchors
}

// See docs/third-party-facts.md § GitHub builds a heading's anchor by lowercasing it, dropping punctuation and turning spaces into hyphens
func githubAnchor(heading string) string {
	var anchor strings.Builder
	for _, char := range strings.ToLower(heading) {
		switch {
		case char == ' ':
			anchor.WriteRune('-')
		case unicode.IsLetter(char) || unicode.IsDigit(char) || char == '-' || char == '_':
			anchor.WriteRune(char)
		}
	}
	return anchor.String()
}
