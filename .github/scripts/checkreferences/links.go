package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type linkTarget struct {
	line int
	// Empty for a link to a heading of the file holding it.
	path     string
	fragment string
	text     string
	isImage  bool
	// A blob URL into this repository, such as one in a PR description, names its path from the repository root.
	isPathFromRepoRoot bool
}

var (
	// Matches `[text](target)`, `![alt](target)`, `<target>` in angle brackets, and an optional "title".
	// Groups: the leading "!", the text, the target.
	markdownLink    = regexp.MustCompile(`(!?)\[([^\]]*)\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)`)
	urlWithScheme   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	markdownHeading = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*$`)
	// Release tags here start with `v`, such as v3.9.0. See [checkreferences.spec.md § Oddities](checkreferences.spec.md#oddities)
	commitOrTagRef = regexp.MustCompile(`^(?:[0-9a-f]{7,40}|v\d[^/]*)/`)
)

type writtenLink struct {
	line    int
	target  string
	text    string
	isImage bool
}

func brokenLinks(paths []string, r repo) ([]string, error) {
	var broken []string
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		for _, link := range linkTargetsInRepo(string(content), commentStyleOf(path), r.knownBranchBlobURLPrefixes) {
			finding, err := checkLink(path, link, r.root)
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

// A PR description links its own branch or main, so any other branch is likely a typo, which brokenLinks would skip as external.
func blobURLsOnOtherBranches(paths []string, r repo) ([]string, error) {
	if r.blobURLRoot == "" {
		return nil, nil
	}
	var found []string
	for _, markdownPath := range pathsWithStyle(paths, markdownProse) {
		content, err := os.ReadFile(markdownPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", markdownPath, err)
		}
		for _, link := range writtenLinks(string(content), markdownProse) {
			if isBlobURLOnOtherBranch(link.target, r) {
				found = append(found, fmt.Sprintf("%s:%d: %s: not on main or the current branch", markdownPath, link.line, link.target))
			}
		}
	}
	return found, nil
}

func isBlobURLOnOtherBranch(link string, r repo) bool {
	refAndPath, isIntoRepo := strings.CutPrefix(link, r.blobURLRoot)
	_, isOnKnownBranch := matchingPrefix(link, r.knownBranchBlobURLPrefixes)
	return isIntoRepo && !isOnKnownBranch && !commitOrTagRef.MatchString(refAndPath)
}

func checkLink(holdingPath string, link linkTarget, repoRoot string) (string, error) {
	written := link.path
	if link.fragment != "" {
		written += "#" + link.fragment
	}
	location := fmt.Sprintf("%s:%d: %s", holdingPath, link.line, written)
	if link.path == "" && commentStyleOf(holdingPath) != markdownProse {
		return location + ": " + commentHasNoHeadings, nil
	}
	targetPath := holdingPath
	switch {
	case link.isPathFromRepoRoot:
		targetPath = filepath.Join(repoRoot, link.path)
	case link.path != "":
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
	headingText, hasHeading := headingTextByAnchor(string(target))[link.fragment]
	if !hasHeading {
		return fmt.Sprintf("%s: no heading with this anchor in %s", location, targetPath), nil
	}
	wantText := linkTextForHeading(holdingPath, targetPath, headingText)
	if !isLinkTextAccepted(holdingPath, link, wantText) {
		return fmt.Sprintf("%s: link text should be %q", location, wantText), nil
	}
	return "", nil
}

// `file.md § Heading` for a heading of another file, `plan skill § Heading` for one of a skill or agent, `§ Heading` for one of the file holding the link.
func linkTextForHeading(holdingPath, targetPath, headingText string) string {
	text := sectionSign + " " + factsEntryDateTail.ReplaceAllString(headingText, "")
	if filepath.Clean(holdingPath) == filepath.Clean(targetPath) {
		return text
	}
	return fileNameInLinkText(targetPath) + " " + text
}

// Every skill's file is a SKILL.md, so a skill or agent goes by its name instead.
func fileNameInLinkText(targetPath string) string {
	slashedPath := filepath.ToSlash(targetPath)
	if match := skillDefinitionPath.FindStringSubmatch(slashedPath); match != nil {
		return match[2] + " skill"
	}
	if match := agentDefinitionPath.FindStringSubmatch(slashedPath); match != nil {
		return match[2] + " agent"
	}
	return filepath.Base(targetPath)
}

// README is for users: its links may read freely, as long as they don't use the `§` form wrongly.
// Images and blob URLs (a PR description's links) are free too.
func isLinkTextAccepted(holdingPath string, link linkTarget, wantText string) bool {
	switch {
	case link.text == wantText, link.isImage, link.isPathFromRepoRoot:
		return true
	case filepath.Base(holdingPath) == "README.md":
		return !strings.Contains(link.text, sectionSign)
	}
	return false
}

// Matches with code spans blanked, so a link inside one is skipped. Reads the text from the written line, since link text may hold a code span.
func writtenLinks(content string, style commentStyle) []writtenLink {
	var links []writtenLink
	for i, line := range proseLines(content, style) {
		for _, match := range markdownLink.FindAllStringSubmatchIndex(withoutCodeSpans(line), -1) {
			links = append(links, writtenLink{
				line:    i + 1,
				isImage: line[match[2]:match[3]] == "!",
				text:    line[match[4]:match[5]],
				target:  line[match[6]:match[7]],
			})
		}
	}
	return links
}

// Relative links, and blob URLs starting with one of knownBranchBlobURLPrefixes.
func linkTargetsInRepo(content string, style commentStyle, knownBranchBlobURLPrefixes []string) []linkTarget {
	var targets []linkTarget
	for _, link := range writtenLinks(content, style) {
		if target, isInRepo := linkTargetInRepo(link.target, knownBranchBlobURLPrefixes); isInRepo {
			target.line, target.text, target.isImage = link.line, link.text, link.isImage
			targets = append(targets, target)
		}
	}
	return targets
}

func linkTargetInRepo(link string, knownBranchBlobURLPrefixes []string) (linkTarget, bool) {
	prefix, isBlobURL := matchingPrefix(link, knownBranchBlobURLPrefixes)
	if isBlobURL {
		path, fragment, _ := splitRelativeLink(strings.TrimPrefix(link, prefix))
		return linkTarget{path: path, fragment: fragment, isPathFromRepoRoot: true}, true
	}
	path, fragment, isRelative := splitRelativeLink(link)
	return linkTarget{path: path, fragment: fragment}, isRelative
}

func matchingPrefix(link string, prefixes []string) (string, bool) {
	return utilities.Find(prefixes, func(prefix string) bool { return strings.HasPrefix(link, prefix) })
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
func headingTextByAnchor(markdown string) map[string]string {
	headingTextByAnchor := map[string]string{}
	seenCountByAnchor := map[string]int{}
	for _, line := range linesOutsideCodeBlocks(markdown) {
		match := markdownHeading.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		anchor := githubAnchor(match[1])
		if seenCount := seenCountByAnchor[anchor]; seenCount > 0 {
			headingTextByAnchor[fmt.Sprintf("%s-%d", anchor, seenCount)] = match[1]
		} else {
			headingTextByAnchor[anchor] = match[1]
		}
		seenCountByAnchor[anchor]++
	}
	return headingTextByAnchor
}

// See [third-party-facts.md § GitHub builds a heading's anchor by lowercasing it, dropping punctuation and turning spaces into hyphens](../../../docs/third-party-facts.md#github-builds-a-headings-anchor-by-lowercasing-it-dropping-punctuation-and-turning-spaces-into-hyphens-2026-09-26)
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
