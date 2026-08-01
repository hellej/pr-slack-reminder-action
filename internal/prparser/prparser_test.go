package prparser_test

import (
	"testing"
	"time"

	"github.com/google/go-github/v78/github"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
)

func testPR(number int, createdAt, updatedAt time.Time) githubclient.PR {
	return githubclient.PR{
		PullRequest: &github.PullRequest{
			Number:    &number,
			CreatedAt: &github.Timestamp{Time: createdAt},
			UpdatedAt: &github.Timestamp{Time: updatedAt},
			User:      &github.User{Login: github.Ptr("author")},
		},
		Author: githubclient.Collaborator{Login: "author"},
	}
}

func TestParsePRsSortsOldestCreatedFirst(t *testing.T) {
	now := time.Now()
	oldest := testPR(1, now.Add(-48*time.Hour), now.Add(-48*time.Hour))
	middle := testPR(2, now.Add(-24*time.Hour), now.Add(-24*time.Hour))
	newest := testPR(3, now.Add(-1*time.Hour), now.Add(-1*time.Hour))

	result := prparser.ParsePRs(
		[]githubclient.PR{newest, oldest, middle},
		config.ContentInputs{},
	)

	want := []int{1, 2, 3}
	for i, number := range want {
		if result[i].GetNumber() != number {
			t.Errorf("expected PR at position %d to be #%d, got #%d", i, number, result[i].GetNumber())
		}
	}
}

func TestParsePRsBreaksCreatedAtTiesByUpdatedAtOldestFirst(t *testing.T) {
	now := time.Now()
	sameCreatedAt := now.Add(-24 * time.Hour)
	updatedLater := testPR(1, sameCreatedAt, now.Add(-1*time.Hour))
	updatedEarlier := testPR(2, sameCreatedAt, now.Add(-2*time.Hour))

	result := prparser.ParsePRs(
		[]githubclient.PR{updatedLater, updatedEarlier},
		config.ContentInputs{},
	)

	if result[0].GetNumber() != 2 || result[1].GetNumber() != 1 {
		t.Errorf(
			"expected PR #2 (updated earlier) before PR #1 (updated later), got order [#%d, #%d]",
			result[0].GetNumber(), result[1].GetNumber(),
		)
	}
}
