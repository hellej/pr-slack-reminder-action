package messagecontent_test

import (
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
)

const testCanvasURL = "https://hellej.slack.com/docs/T08SGDGNB2B/F0BMEPVR1DL"

func testPR() prparser.PR {
	return prparser.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{Title: "A PR"},
			Repository:  models.Repository{Owner: "test-org", Name: "test-repo"},
		},
	}
}

func TestGetContentCanvasURL(t *testing.T) {
	testCases := []struct {
		name          string
		prs           []prparser.PR
		contentInputs config.ContentInputs
	}{
		{
			name:          "no PRs",
			contentInputs: config.ContentInputs{NoPRsMessage: "No PRs", CanvasURL: testCanvasURL},
		},
		{
			name:          "flat list",
			prs:           []prparser.PR{testPR()},
			contentInputs: config.ContentInputs{PRListHeading: "Open PRs", CanvasURL: testCanvasURL},
		},
		{
			name:          "grouped by repository",
			prs:           []prparser.PR{testPR()},
			contentInputs: config.ContentInputs{GroupByRepository: true, CanvasURL: testCanvasURL},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content := messagecontent.GetContent(tc.prs, tc.contentInputs)

			if content.CanvasURL != testCanvasURL {
				t.Errorf("expected canvas URL '%s', got '%s'", testCanvasURL, content.CanvasURL)
			}
		})
	}

	t.Run("unset canvas URL stays empty", func(t *testing.T) {
		content := messagecontent.GetContent(
			[]prparser.PR{testPR()}, config.ContentInputs{PRListHeading: "Open PRs"},
		)

		if content.CanvasURL != "" {
			t.Errorf("expected an empty canvas URL, got '%s'", content.CanvasURL)
		}
	})
}
