// Prints every way the PR description in the file given breaks the `pr` skill's limits, and exits 1 if any.
package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
)

const (
	maxHeadingSummaryWords = 6
	maxBulletWords         = 15
	maxSummaryWords        = 80
	// The GitHub MCP tools wrap a longer URL in backticks. See docs/third-party-facts.md § The GitHub MCP tools wrap a PR description URL over about 150 characters in backticks, breaking the link
	maxLinkLength = 150
)

type summarySection struct {
	heading    string
	minBullets int
	maxBullets int
}

var summarySections = []summarySection{
	{heading: "### The problem", minBullets: 2, maxBullets: 3},
	{heading: "### The solution", minBullets: 2, maxBullets: 5},
}

var markdownLinkURL = regexp.MustCompile(`\]\(([^)\s]+)\)`)

const relatedHeading = "## Related"

var (
	issueOrPRReference = regexp.MustCompile(`#\d+|/(issues|pull)/\d+`)
	issueOrPRState     = regexp.MustCompile(`^- (Open|Closed|Merged) `)
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: checkprdescription <description file>")
		os.Exit(2)
	}
	description, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	found := problems(string(description))
	for _, problem := range found {
		fmt.Println(problem)
	}
	if len(found) > 0 {
		os.Exit(1)
	}
}

func problems(description string) []string {
	lines := strings.Split(description, "\n")
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
	found = append(found, relatedWithoutState(lines)...)
	return append(found, longLinks(description)...)
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
		if issueOrPRReference.MatchString(trimmed) && !issueOrPRState.MatchString(trimmed) {
			found = append(found, relatedHeading+": start with Open, Closed or Merged: "+strings.TrimPrefix(trimmed, "- "))
		}
	}
	return found
}

// Returns the section's problems and the word count of its bullets.
func (s summarySection) problems(lines []string) ([]string, int) {
	headingIndex := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, s.heading) })
	if headingIndex == -1 {
		return []string{"missing heading: " + s.heading}, 0
	}
	var found []string
	report := func(problem string) { found = append(found, s.heading+": "+problem) }

	if problem := headingSummaryProblem(strings.TrimPrefix(lines[headingIndex], s.heading)); problem != "" {
		report(problem)
	}

	topLevelBullets, words := 0, 0
	for _, line := range linesUntilNextHeading(lines[headingIndex+1:]) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if trimmed != "-" && !strings.HasPrefix(trimmed, "- ") {
			report("not a bullet: " + trimmed)
			continue
		}
		if strings.HasPrefix(line, "-") {
			topLevelBullets++
		}
		bulletText := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
		bulletWords := len(strings.Fields(bulletText))
		words += bulletWords
		switch {
		case bulletWords == 0:
			report("empty bullet")
		case bulletWords > maxBulletWords:
			report(fmt.Sprintf("bullet over %d words: %s", maxBulletWords, bulletText))
		}
	}

	if topLevelBullets < s.minBullets || topLevelBullets > s.maxBullets {
		report(fmt.Sprintf("%d bullets, want %d to %d", topLevelBullets, s.minBullets, s.maxBullets))
	}
	return found, words
}

// headingRest is what follows "### The problem", such as ": reviews stall".
func headingSummaryProblem(headingRest string) string {
	summary := strings.TrimSpace(strings.TrimPrefix(headingRest, ":"))
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
			found = append(found, fmt.Sprintf("link over %d characters: %s", maxLinkLength, url))
		}
	}
	return found
}
