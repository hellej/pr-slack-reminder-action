package composer

import (
	"fmt"
	"math"
	"time"

	"github.com/google/go-github/v72/github"
	"github.com/slack-go/slack"
)

func getPRCreationTimeText(createdAt time.Time) string {
	duration := time.Since(createdAt)
	if duration.Hours() >= 24 {
		days := int(math.Round(duration.Hours())) / 24
		return fmt.Sprintf("%d days ago ", days)
	} else if duration.Hours() >= 1 {
		hours := int(math.Round(duration.Hours()))
		return fmt.Sprintf("%d hours ago ", hours)
	} else {
		minutes := int(math.Round(duration.Minutes()))
		return fmt.Sprintf("%d minutes ago ", minutes)
	}
}

func composePRBulletPointBlock(pr *github.PullRequest) slack.RichTextElement {
	var loginOrName string
	if pr.GetUser().GetName() != "" {
		loginOrName = pr.GetUser().GetName()
	} else {
		loginOrName = pr.GetUser().GetLogin()
	}

	return slack.NewRichTextSection(
		slack.NewRichTextSectionLinkElement(pr.GetHTMLURL(), pr.GetTitle(), &slack.RichTextSectionTextStyle{Bold: true}),
		slack.NewRichTextSectionTextElement(
			" ("+getPRCreationTimeText(pr.CreatedAt.Time)+"by "+loginOrName+")", &slack.RichTextSectionTextStyle{}),
	)
}

func composePRListBlock(openPRs []*github.PullRequest) *slack.RichTextBlock {
	var prBlocks []slack.RichTextElement
	for _, pr := range openPRs {
		prBlocks = append(prBlocks, composePRBulletPointBlock(pr))
	}
	return slack.NewRichTextBlock(
		"open_prs",
		slack.NewRichTextList(slack.RichTextListElementType("bullet"), 0,
			prBlocks...,
		),
	)
}

func getOldPRsThresholdTimeLabel(oldPRThresholdHours int) string {
	if oldPRThresholdHours < 24 {
		return fmt.Sprintf("%d hours", oldPRThresholdHours)
	}
	days := int(math.Round(float64(oldPRThresholdHours / 24)))
	return fmt.Sprintf("%d days", days)
}

type PRCategory struct {
	Heading string
	PRs     []*github.PullRequest
}

type PRCategories struct {
	NewPRs PRCategory
	OldPRs PRCategory
}
type PRCategoryHeadings struct {
	NewPRsHeading string
	OldPRsHeading string
}

func getPRCategoryHeadings(oldPRThresholdHours int) PRCategoryHeadings {
	timeThresholdLabel := getOldPRsThresholdTimeLabel(oldPRThresholdHours)
	return PRCategoryHeadings{
		NewPRsHeading: "🚀 New PRs since " + timeThresholdLabel + " ago",
		OldPRsHeading: "⌛️ Old PRs since " + timeThresholdLabel + " ago",
	}
}

func getPRCategories(openPRs []*github.PullRequest, oldPRThresholdHours int) PRCategories {
	var prCategories PRCategories

	for _, pr := range openPRs {
		if pr.GetCreatedAt().After(time.Now().Add(-time.Duration(oldPRThresholdHours) * time.Hour)) {
			prCategories.NewPRs.PRs = append(prCategories.NewPRs.PRs, pr)
		} else {
			prCategories.OldPRs.PRs = append(prCategories.OldPRs.PRs, pr)
		}
	}

	headings := getPRCategoryHeadings(oldPRThresholdHours)
	prCategories.NewPRs.Heading = headings.NewPRsHeading
	prCategories.OldPRs.Heading = headings.OldPRsHeading
	return prCategories
}

func ComposeMessage(openPRs []*github.PullRequest, oldPRThresholdHours int) (slack.Message, string) {
	var blocks []slack.Block

	if len(openPRs) == 0 {
		text := "No open PRs, happy coding! 🎉"
		blocks = append(blocks,
			slack.NewRichTextBlock("no_prs_block",
				slack.NewRichTextSection(
					slack.NewRichTextSectionTextElement(text, &slack.RichTextSectionTextStyle{}),
				),
			),
		)
		return slack.NewBlockMessage(blocks...), text
	}

	if oldPRThresholdHours == 0 {
		blocks = append(blocks, slack.NewHeaderBlock(
			slack.NewTextBlockObject("plain_text", "Open PRs", false, false),
		),
			composePRListBlock(openPRs),
		)
		return slack.NewBlockMessage(blocks...), fmt.Sprintf("%d new PRs are waiting for attention", len(openPRs))
	}

	prCategories := getPRCategories(openPRs, oldPRThresholdHours)

	if len(prCategories.NewPRs.PRs) > 0 {
		blocks = append(blocks, slack.NewHeaderBlock(
			slack.NewTextBlockObject("plain_text", prCategories.NewPRs.Heading, false, false),
		),
			composePRListBlock(openPRs),
		)
	}

	return slack.NewBlockMessage(blocks...), fmt.Sprintf("%d new PRs are waiting for attention", len(prCategories.NewPRs.PRs))
}
