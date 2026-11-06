// Prints every way the PR description in the file given breaks the `pr` skill's limits, and exits 1 if any.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
)

const (
	maxHeadingSummaryWords = 8
	maxBulletWords         = 15
	maxSummaryWords        = 80
	// The GitHub MCP tools wrap a longer URL in backticks. `gh` has no such limit. See docs/third-party-facts.md § The GitHub MCP tools wrap a PR description URL over about 150 characters in backticks, breaking the link
	maxLinkLength = 150
)

type summarySection struct {
	heading         string
	requiresSummary bool
	minBullets      int
	maxBullets      int
}

var summarySections = []summarySection{
	{heading: "### The problem", requiresSummary: true, minBullets: 2, maxBullets: 3},
	{heading: "### The solution", minBullets: 2, maxBullets: 5},
}

var markdownLinkURL = regexp.MustCompile(`\]\(([^)\s]+)\)`)

const (
	relatedHeading = "## Related"
	openHeading    = "## Open"

	footerSeparator = "---"
)

var (
	issueOrPRReference       = regexp.MustCompile(`#\d+|/(issues|pull)/\d+`)
	startsWithIssueOrPRState = regexp.MustCompile(`^- (Open|Closed|Merged) `)
	closesOrFixesIssue       = regexp.MustCompile(`^(Closes|Fixes) #\d+`)
	numberedItem             = regexp.MustCompile(`^\d+\. `)
)

func main() {
	forMCPTools := flag.Bool("mcp", false, "also check link lengths, for a description written with the GitHub MCP tools")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: checkprdescription [-mcp] <description file>")
		os.Exit(2)
	}
	description, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	found := problems(string(description), *forMCPTools)
	for _, problem := range found {
		fmt.Println(problem)
	}
	if len(found) > 0 {
		os.Exit(1)
	}
}

func problems(description string, forMCPTools bool) []string {
	description = strings.ReplaceAll(description, "\r\n", "\n")
	lines := linesAboveFooter(strings.Split(description, "\n"))
	var found []string
	summaryWords := 0
	for _, section := range summarySections {
		sectionProblems, sectionWords := section.problems(lines)
		found = append(found, sectionProblems...)
		summaryWords += sectionWords
	}
	if summaryWords > maxSummaryWords {
		found = append(found, fmt.Sprintf("summary is %d words, want at most %d", summaryWords, maxSummaryWords))
	}
	found = append(found, linesNotListItems(lines)...)
	found = append(found, relatedWithoutState(lines)...)
	if forMCPTools {
		found = append(found, longLinks(description)...)
	}
	return found
}

func linesAboveFooter(lines []string) []string {
	footerIndex := slices.Index(lines, footerSeparator)
	if footerIndex == -1 {
		return lines
	}
	return lines[:footerIndex]
}

func linesNotListItems(lines []string) []string {
	var found []string
	currentHeading := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isListItem := isBullet(trimmed) || isSubItem(line) && numberedItem.MatchString(trimmed)
		switch {
		case trimmed == "":
		case strings.HasPrefix(line, "#"):
			currentHeading = line
		case currentHeading == "":
			if !closesOrFixesIssue.MatchString(trimmed) {
				found = append(found, "before the first heading: not a Closes or Fixes line: "+trimmed)
			}
		case currentHeading == openHeading && !isSubItem(line):
			if !numberedItem.MatchString(trimmed) {
				found = append(found, openHeading+": not a numbered item: "+trimmed)
			}
		case !isListItem:
			found = append(found, headingWithoutSummary(currentHeading)+": not a bullet: "+trimmed)
		case trimmed == "-":
			found = append(found, headingWithoutSummary(currentHeading)+": empty bullet")
		}
	}
	return found
}

func isSubItem(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

func isBullet(trimmedLine string) bool {
	return trimmedLine == "-" || strings.HasPrefix(trimmedLine, "- ")
}

func headingWithoutSummary(heading string) string {
	withoutSummary, _, _ := strings.Cut(heading, ":")
	return withoutSummary
}

// A reader can't tell an open issue or PR from a closed one by its number alone.
func relatedWithoutState(lines []string) []string {
	headingIndex := slices.Index(lines, relatedHeading)
	if headingIndex == -1 {
		return nil
	}
	var found []string
	for _, line := range linesUntilNextHeading(lines[headingIndex+1:]) {
		trimmed := strings.TrimSpace(line)
		if issueOrPRReference.MatchString(trimmed) && !startsWithIssueOrPRState.MatchString(trimmed) {
			found = append(found, relatedHeading+": start with Open, Closed or Merged: "+strings.TrimPrefix(trimmed, "- "))
		}
	}
	return found
}

func (s summarySection) problems(lines []string) (found []string, bulletWords int) {
	headingIndex := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, s.heading) })
	if headingIndex == -1 {
		return []string{"missing heading: " + s.heading}, 0
	}
	report := func(problem string) { found = append(found, s.heading+": "+problem) }

	if problem := s.headingSummaryProblem(lines[headingIndex]); problem != "" {
		report(problem)
	}

	topLevelBullets := 0
	for _, line := range linesUntilNextHeading(lines[headingIndex+1:]) {
		trimmed := strings.TrimSpace(line)
		if !isBullet(trimmed) {
			continue
		}
		if !isSubItem(line) {
			topLevelBullets++
		}
		bulletText := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
		wordsInBullet := len(strings.Fields(bulletText))
		bulletWords += wordsInBullet
		if wordsInBullet > maxBulletWords {
			report(fmt.Sprintf("bullet over %d words: %s", maxBulletWords, bulletText))
		}
	}

	if topLevelBullets < s.minBullets || topLevelBullets > s.maxBullets {
		report(fmt.Sprintf("%d bullets, want %d to %d", topLevelBullets, s.minBullets, s.maxBullets))
	}
	return found, bulletWords
}

func (s summarySection) headingSummaryProblem(headingLine string) string {
	afterHeading := strings.TrimSpace(strings.TrimPrefix(headingLine, s.heading))
	summary, hasColon := strings.CutPrefix(afterHeading, ":")
	if afterHeading != "" && !hasColon {
		return "summary not after a colon"
	}
	summary = strings.TrimSpace(summary)
	if s.requiresSummary && summary == "" {
		return "heading without a summary"
	}
	if strings.HasPrefix(summary, "<a few words") {
		return "template placeholder left in: " + summary
	}
	if len(strings.Fields(summary)) > maxHeadingSummaryWords {
		return fmt.Sprintf("heading summary over %d words", maxHeadingSummaryWords)
	}
	return ""
}

func linesUntilNextHeading(lines []string) []string {
	nextHeadingIndex := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, "#") })
	if nextHeadingIndex == -1 {
		return lines
	}
	return lines[:nextHeadingIndex]
}

func longLinks(description string) []string {
	var found []string
	for _, match := range markdownLinkURL.FindAllStringSubmatch(description, -1) {
		if url := match[1]; len(url) > maxLinkLength {
			found = append(found, fmt.Sprintf("link URL over %d characters: %s", maxLinkLength, url))
		}
	}
	return found
}
