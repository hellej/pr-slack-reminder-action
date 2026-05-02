package githubclient

import (
	"log"
	"regexp"
	"strconv"
	"time"

	"github.com/google/go-github/v78/github"
)

var snoozeRegex = regexp.MustCompile(`(?i)^/?snooze(?:\s+pr[\s-]*reminder)?(?:\s+for)?\s+(\d+)\s*(d|days?)?$`)

func parseSnoozeComment(body string, createdAt time.Time) *time.Time {
	matches := snoozeRegex.FindStringSubmatch(body)
	if matches == nil {
		return nil
	}

	days, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil
	}

	expiration := createdAt.Add(time.Duration(days) * 24 * time.Hour)
	return &expiration
}

func findActiveSnooze(timelineComments []*github.IssueComment) *time.Time {
	var latestSnooze *time.Time
	var latestSnoozeCommentTime time.Time

	for _, comment := range timelineComments {
		commentTime := comment.GetCreatedAt().Time
		expiration := parseSnoozeComment(comment.GetBody(), commentTime)
		if expiration == nil {
			continue
		}
		if latestSnooze == nil || commentTime.After(latestSnoozeCommentTime) {
			latestSnooze = expiration
			latestSnoozeCommentTime = commentTime
		}
	}

	if latestSnooze != nil && latestSnooze.After(time.Now()) {
		return latestSnooze
	}
	return nil
}

func excludeSnoozedPRs(prs []PR) []PR {
	filtered := make([]PR, 0, len(prs))
	for _, pr := range prs {
		if pr.SnoozedUntil != nil {
			log.Printf("PR %s/%d snoozed until %s", pr.Repository.GetPath(), pr.GetNumber(), pr.SnoozedUntil.Format(time.DateOnly))
			continue
		}
		filtered = append(filtered, pr)
	}
	return filtered
}
