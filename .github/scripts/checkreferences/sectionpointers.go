package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type sectionPointer struct {
	line int
	// Empty when the pointer names no file: it points into the file holding it.
	writtenTargetFile string
	// Lines of a wrapped pointer are joined by "\n". See checkreferences.spec.md § Oddities
	text string
}

const sectionSign = "§"

var (
	bareMarkdownFile       = regexp.MustCompile(`[A-Za-z0-9_./-]+\.md\b`)
	backtickedSkillOrAgent = regexp.MustCompile("`([A-Za-z0-9-]+)` (skill|agent)")
	boldText               = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	factsEntryDateTail     = regexp.MustCompile(`\s+\[\d{4}-\d{2}-\d{2}\]$`)
	whitespaceRun          = regexp.MustCompile(`\s+`)
)

func brokenSectionPointers(paths []string, r repo) ([]string, error) {
	var broken []string
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		for _, pointer := range sectionPointers(string(content), commentStyleOf(path)) {
			finding, err := checkSectionPointer(path, pointer, r)
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

func checkSectionPointer(holdingPath string, pointer sectionPointer, r repo) (string, error) {
	location := fmt.Sprintf("%s:%d: %s %s", holdingPath, pointer.line, sectionSign, shortened(pointer.text))
	if pointer.writtenTargetFile == "" && commentStyleOf(holdingPath) != markdownProse {
		return location + ": a code comment has no headings, name the target file", nil
	}
	targetPath := holdingPath
	if pointer.writtenTargetFile != "" {
		var isFound bool
		targetPath, isFound = resolveTargetFile(holdingPath, pointer.writtenTargetFile, r.root)
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
	return utilities.Find([]string{
		filepath.Join(filepath.Dir(holdingPath), writtenTargetFile),
		filepath.Join(repoRoot, writtenTargetFile),
	}, func(candidate string) bool {
		_, err := os.Stat(candidate)
		return err == nil
	})
}

// `§ Gitt` doesn't match `Git`. See checkreferences.spec.md § Oddities
func startsWithAnyLabel(pointerText string, labels []string) bool {
	text := whitespaceRun.ReplaceAllString(pointerText, " ")
	return slices.ContainsFunc(labels, func(label string) bool {
		rest, hasPrefix := strings.CutPrefix(text, label)
		next, _ := utf8.DecodeRuneInString(rest)
		return hasPrefix && (rest == "" || !(unicode.IsLetter(next) || unicode.IsDigit(next)))
	})
}

// Facts file headings end in their date, which pointers leave out. See checkreferences.spec.md § Behaviour
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

// See checkreferences.spec.md § Doesn't Do
func sectionPointers(content string, style commentStyle) []sectionPointer {
	lines := proseLines(content, style)
	var pointers []sectionPointer
	for i, line := range lines {
		spans := codeSpans(line)
		var previousTarget pointerTarget
		for _, signIndex := range sectionSignIndexes(line) {
			if isInsideCodeSpan(signIndex, spans) {
				continue
			}
			target, isNamed := fileNamedRightBefore(line[:signIndex])
			if !isNamed {
				target = previousTarget
			}
			previousTarget = target
			if target.isIntoExternalPageOrLinkText {
				continue
			}
			pointers = append(pointers, sectionPointer{
				line:              i + 1,
				writtenTargetFile: target.writtenTargetFile,
				text:              pointerText(line[signIndex+len(sectionSign):], lines[i+1:]),
			})
		}
	}
	return pointers
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

type pointerTarget struct {
	writtenTargetFile            string
	isIntoExternalPageOrLinkText bool
}

// See checkreferences.spec.md § Oddities
func fileNamedRightBefore(lineBeforeSign string) (pointerTarget, bool) {
	if isInsideLinkText(lineBeforeSign) {
		return pointerTarget{isIntoExternalPageOrLinkText: true}, true
	}
	var nearest pointerTarget
	nearestEnd := -1
	for _, match := range markdownLink.FindAllStringSubmatchIndex(lineBeforeSign, -1) {
		path, _, isRelative := splitRelativeLink(lineBeforeSign[match[2]:match[3]])
		nearest, nearestEnd = pointerTarget{writtenTargetFile: path, isIntoExternalPageOrLinkText: !isRelative}, match[1]
	}
	for _, match := range bareMarkdownFile.FindAllStringIndex(lineBeforeSign, -1) {
		if match[1] > nearestEnd {
			nearest, nearestEnd = pointerTarget{writtenTargetFile: lineBeforeSign[match[0]:match[1]]}, match[1]
		}
	}
	for _, match := range backtickedSkillOrAgent.FindAllStringSubmatchIndex(lineBeforeSign, -1) {
		if match[1] > nearestEnd {
			name, kind := lineBeforeSign[match[2]:match[3]], lineBeforeSign[match[4]:match[5]]
			nearest, nearestEnd = pointerTarget{writtenTargetFile: skillOrAgentFile(name, kind)}, match[1]
		}
	}
	isRightBefore := nearestEnd >= 0 && strings.TrimRight(lineBeforeSign[nearestEnd:], " `*") == ""
	return nearest, isRightBefore
}

func skillOrAgentFile(name, kind string) string {
	if kind == "skill" {
		return skillFile(name)
	}
	return agentFile(name)
}

// True for `[Docs § Memory` with its `](...)` still to come: a page title, not a pointer.
func isInsideLinkText(lineBeforeSign string) bool {
	openBracket := strings.LastIndex(lineBeforeSign, "[")
	return openBracket > strings.LastIndex(lineBeforeSign, "]")
}

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
