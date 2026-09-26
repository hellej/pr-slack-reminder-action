package main

import (
	"path/filepath"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type commentStyle int

const (
	markdownProse commentStyle = iota
	slashComments
	hashComments
)

func commentStyleOf(path string) commentStyle {
	switch filepath.Ext(path) {
	case ".md":
		return markdownProse
	case ".go":
		return slashComments
	default:
		return hashComments
	}
}

func pathsWithStyle(paths []string, style commentStyle) []string {
	return utilities.Filter(paths, func(path string) bool { return commentStyleOf(path) == style })
}

// Keeps line numbers: a line that is not prose becomes empty. Code comments count from full-line comments only.
func proseLines(content string, style commentStyle) []string {
	switch style {
	case slashComments:
		return fullLineComments(content, "//")
	case hashComments:
		return fullLineComments(content, "#")
	default:
		return linesOutsideCodeBlocks(content)
	}
}

func fullLineComments(content, commentMarker string) []string {
	return utilities.Map(strings.Split(content, "\n"), func(line string) string {
		comment, isComment := strings.CutPrefix(strings.TrimSpace(line), commentMarker)
		if !isComment {
			return ""
		}
		return comment
	})
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

type codeSpan struct {
	start, end int
	content    string
}

// A run of backticks opens a code span, and the next run of the same length closes it. An unclosed run is plain text.
func codeSpans(line string) []codeSpan {
	var spans []codeSpan
	for offset := 0; offset < len(line); {
		openStart := strings.Index(line[offset:], "`")
		if openStart < 0 {
			break
		}
		openStart += offset
		fence := backtickRunAt(line, openStart)
		contentStart := openStart + len(fence)
		closeStart, isClosed := closingRun(line, contentStart, fence)
		if !isClosed {
			offset = contentStart
			continue
		}
		spans = append(spans, codeSpan{
			start:   openStart,
			end:     closeStart + len(fence),
			content: strings.TrimSpace(line[contentStart:closeStart]),
		})
		offset = closeStart + len(fence)
	}
	return spans
}

func backtickRunAt(line string, start int) string {
	end := start
	for end < len(line) && line[end] == '`' {
		end++
	}
	return line[start:end]
}

func closingRun(line string, from int, fence string) (int, bool) {
	for offset := from; offset < len(line); {
		runStart := strings.Index(line[offset:], "`")
		if runStart < 0 {
			return 0, false
		}
		runStart += offset
		run := backtickRunAt(line, runStart)
		if run == fence {
			return runStart, true
		}
		offset = runStart + len(run)
	}
	return 0, false
}

func isInsideCodeSpan(index int, spans []codeSpan) bool {
	for _, span := range spans {
		if index >= span.start && index < span.end {
			return true
		}
	}
	return false
}

// Blanks code spans with spaces, keeping every other character at its index.
func withoutCodeSpans(line string) string {
	blanked := []byte(line)
	for _, span := range codeSpans(line) {
		for i := span.start; i < span.end; i++ {
			blanked[i] = ' '
		}
	}
	return string(blanked)
}
