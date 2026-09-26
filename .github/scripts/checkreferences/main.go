// Prints every broken reference in the files given as arguments:
//   - a relative Markdown link whose target doesn't exist
//   - a `<file>.md § <section>` pointer whose text doesn't start with a heading or bold text of that file
//
// Run from the repository root by check-style.sh.
package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type linkTarget struct {
	line   int
	target string
}

type sectionPointer struct {
	line int
	// Empty when the pointer names no file: it points into the file holding it.
	writtenTargetFile string
	// Runs on past the section name, which has no end marker. Lines of a wrapped pointer are joined by "\n".
	text string
}

const sectionSign = "§"

var (
	inlineCodeSpan = regexp.MustCompile("`[^`]*`")
	// Matches [text](target), ![alt](target), <target> in angle brackets, and an optional "title".
	markdownLink       = regexp.MustCompile(`!?\[[^\]]*\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)`)
	urlWithScheme      = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	bareMarkdownFile   = regexp.MustCompile(`[A-Za-z0-9_./-]+\.md\b`)
	markdownHeading    = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*$`)
	boldText           = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	factsEntryDateTail = regexp.MustCompile(`\s+\[\d{4}-\d{2}-\d{2}\]$`)
	whitespaceRun      = regexp.MustCompile(`\s+`)
)

func main() {
	markdownPaths := filterByExtension(os.Args[1:], ".md")
	brokenLinkFindings, err := brokenLinks(markdownPaths)
	if err != nil {
		exitWithError(err)
	}
	brokenPointerFindings, err := brokenSectionPointers(os.Args[1:], ".")
	if err != nil {
		exitWithError(err)
	}
	for _, finding := range append(brokenLinkFindings, brokenPointerFindings...) {
		fmt.Println(finding)
	}
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}

func filterByExtension(paths []string, extension string) []string {
	var matching []string
	for _, path := range paths {
		if filepath.Ext(path) == extension {
			matching = append(matching, path)
		}
	}
	return matching
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
	for i, line := range linesOutsideCodeBlocks(markdown) {
		proseOnly := inlineCodeSpan.ReplaceAllString(line, "")
		for _, match := range markdownLink.FindAllStringSubmatch(proseOnly, -1) {
			if target, isRelative := relativePath(match[1]); isRelative {
				targets = append(targets, linkTarget{line: i + 1, target: target})
			}
		}
	}
	return targets
}

// Blanks the lines inside fenced code blocks, keeping line numbers.
func linesOutsideCodeBlocks(markdown string) []string {
	lines := strings.Split(markdown, "\n")
	isInCodeBlock := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		isFence := strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
		if isFence {
			isInCodeBlock = !isInCodeBlock
		}
		if isFence || isInCodeBlock {
			lines[i] = ""
		}
	}
	return lines
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

func brokenSectionPointers(paths []string, repoRoot string) ([]string, error) {
	var broken []string
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		for _, pointer := range sectionPointers(string(content), filepath.Ext(path) == ".go") {
			finding, err := checkSectionPointer(path, pointer, repoRoot)
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

func checkSectionPointer(holdingPath string, pointer sectionPointer, repoRoot string) (string, error) {
	location := fmt.Sprintf("%s:%d: %s %s", holdingPath, pointer.line, sectionSign, shortened(pointer.text))
	targetPath := holdingPath
	if pointer.writtenTargetFile != "" {
		var isFound bool
		targetPath, isFound = resolveTargetFile(holdingPath, pointer.writtenTargetFile, repoRoot)
		if !isFound {
			return fmt.Sprintf("%s: target file %s not found", location, pointer.writtenTargetFile), nil
		}
	}
	target, err := os.ReadFile(targetPath)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", targetPath, err)
	}
	if !startsWithAnyLabel(pointer.text, sectionLabels(string(target))) {
		return fmt.Sprintf("%s: no heading or bold text starting it in %s", location, targetPath), nil
	}
	return "", nil
}

func shortened(pointerText string) string {
	const maxLength = 40
	firstLine, _, _ := strings.Cut(pointerText, "\n")
	if utf8.RuneCountInString(firstLine) <= maxLength {
		return firstLine
	}
	return string([]rune(firstLine)[:maxLength]) + "…"
}

// A written name is relative to the file holding it, as a link is, or else to the repository root, as `AGENTS.md` usually is.
func resolveTargetFile(holdingPath, writtenTargetFile, repoRoot string) (string, bool) {
	for _, candidate := range []string{
		filepath.Join(filepath.Dir(holdingPath), writtenTargetFile),
		filepath.Join(repoRoot, writtenTargetFile),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}
	return "", false
}

// The section name has no end marker, so the text must start with a label and end it on a word boundary: `§ Gitt` doesn't match `Git`.
func startsWithAnyLabel(pointerText string, labels []string) bool {
	text := whitespaceRun.ReplaceAllString(pointerText, " ")
	for _, label := range labels {
		rest, hasPrefix := strings.CutPrefix(text, label)
		if !hasPrefix {
			continue
		}
		next, _ := utf8.DecodeRuneInString(rest)
		if rest == "" || !(unicode.IsLetter(next) || unicode.IsDigit(next)) {
			return true
		}
	}
	return false
}

// Facts file headings end in their date, which pointers leave out.
func sectionLabels(markdown string) []string {
	var labels []string
	for _, line := range linesOutsideCodeBlocks(markdown) {
		if match := markdownHeading.FindStringSubmatch(line); match != nil {
			labels = append(labels, factsEntryDateTail.ReplaceAllString(match[1], ""))
			continue
		}
		for _, match := range boldText.FindAllStringSubmatch(line, -1) {
			labels = append(labels, strings.TrimSuffix(match[1], ":"))
		}
	}
	return labels
}

// Skips pointers in code blocks and code spans, which are examples, and pointers into external pages or inside link text.
// In Go source, reads full-line comments only, and skips a pointer naming no file.
func sectionPointers(content string, isGoSource bool) []sectionPointer {
	lines := proseLines(content, isGoSource)
	var pointers []sectionPointer
	for i, line := range lines {
		for _, signIndex := range sectionSignIndexes(line) {
			if !isGoSource && isInsideAny(signIndex, inlineCodeSpan.FindAllStringIndex(line, -1)) {
				continue
			}
			writtenTargetFile, isUncheckable := nearestFileNamedBefore(line[:signIndex], previousLine(lines, i))
			if isUncheckable || (isGoSource && writtenTargetFile == "") {
				continue
			}
			pointers = append(pointers, sectionPointer{
				line:              i + 1,
				writtenTargetFile: writtenTargetFile,
				text:              pointerText(line[signIndex+len(sectionSign):], lines[i+1:]),
			})
		}
	}
	return pointers
}

// Keeps line numbers: a line that is not prose becomes empty.
func proseLines(content string, isGoSource bool) []string {
	if !isGoSource {
		return linesOutsideCodeBlocks(content)
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		comment, isComment := strings.CutPrefix(strings.TrimSpace(line), "//")
		if !isComment {
			comment = ""
		}
		lines[i] = comment
	}
	return lines
}

func sectionSignIndexes(line string) []int {
	var indexes []int
	for offset := 0; ; {
		index := strings.Index(line[offset:], sectionSign)
		if index < 0 {
			return indexes
		}
		indexes = append(indexes, offset+index)
		offset += index + len(sectionSign)
	}
}

func isInsideAny(index int, ranges [][]int) bool {
	for _, r := range ranges {
		if index >= r[0] && index < r[1] {
			return true
		}
	}
	return false
}

func previousLine(lines []string, i int) string {
	if i == 0 {
		return ""
	}
	return lines[i-1]
}

// Looks back over the pointer's own line, then the line before it, for the last file named. A link names its target.
func nearestFileNamedBefore(lineBeforeSign, previousLine string) (writtenTargetFile string, isUncheckable bool) {
	for _, text := range []string{lineBeforeSign, previousLine} {
		if isInsideLinkText(text) {
			return "", true
		}
		nearestEnd := -1
		for _, match := range markdownLink.FindAllStringSubmatchIndex(text, -1) {
			nearestEnd = match[1]
			path, isRelative := relativePath(text[match[2]:match[3]])
			writtenTargetFile, isUncheckable = path, !isRelative
		}
		for _, match := range bareMarkdownFile.FindAllStringIndex(text, -1) {
			if match[1] > nearestEnd {
				nearestEnd = match[1]
				writtenTargetFile, isUncheckable = text[match[0]:match[1]], false
			}
		}
		if nearestEnd >= 0 {
			return writtenTargetFile, isUncheckable
		}
	}
	return "", false
}

// True for `[Docs § Memory` with its `](...)` still to come: a page title, not a pointer.
func isInsideLinkText(lineBeforeSign string) bool {
	openBracket := strings.LastIndex(lineBeforeSign, "[")
	return openBracket > strings.LastIndex(lineBeforeSign, "]")
}

// Joins the rest of the pointer's line with the lines after it, up to a blank line.
func pointerText(restOfLine string, linesAfter []string) string {
	parts := []string{strings.TrimSpace(restOfLine)}
	for _, line := range linesAfter {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		parts = append(parts, trimmed)
	}
	return strings.TrimPrefix(strings.Join(parts, "\n"), "**")
}
