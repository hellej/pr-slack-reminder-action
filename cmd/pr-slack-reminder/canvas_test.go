package main_test

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v78/github"
	main "github.com/hellej/pr-slack-reminder-action/cmd/pr-slack-reminder"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/testhelpers"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockgithubclient"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockslackclient"
)

// The canvas link the action parses the canvas ID F0BMEPVR1DL out of.
const testCanvasLink = "https://hellej.slack.com/docs/T08SGDGNB2B/F0BMEPVR1DL"

const testCanvasID = "F0BMEPVR1DL"

var canvasFooterLine = regexp.MustCompile(`_Updated \d{4}-\d{2}-\d{2} \d{2}:\d{2} UTC_`)

func assertCanvasContains(t *testing.T, markdown string, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if !strings.Contains(markdown, text) {
			t.Errorf("Expected canvas markdown to contain '%s', got:\n%s", text, markdown)
		}
	}
}

func assertCanvasDoesNotContain(t *testing.T, markdown string, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if strings.Contains(markdown, text) {
			t.Errorf("Expected canvas markdown not to contain '%s', got:\n%s", text, markdown)
		}
	}
}

func canvasTestPRs() []*github.PullRequest {
	return []*github.PullRequest{
		getTestPR(GetTestPROptions{Number: 1, Title: "Open PR one", AuthorLogin: "alice", AgeHours: 5}),
		getTestPR(GetTestPROptions{Number: 2, Title: "Open PR two", AuthorLogin: "bob", AgeHours: 3}),
		getTestPR(GetTestPROptions{
			Number: 3, Title: "Draft PR one", AuthorLogin: "carol", AgeHours: 10, Draft: github.Ptr(true),
		}),
	}
}

func TestPostModeCanvasRefresh(t *testing.T) {
	testCases := []struct {
		name              string
		groupByRepository bool
	}{
		{name: "flat"},
		{name: "grouped by repository", groupByRepository: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
				config.InputPRTrackerCanvasLink: testCanvasLink,
				config.InputGroupByRepository:   tc.groupByRepository,
			})
			prs := canvasTestPRs()

			mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
			err := main.Run(
				mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
					PRs:               prs,
					CommitsByPRNumber: map[int]time.Time{3: now.Add(-5 * time.Hour)},
				}),
				mockslackclient.MakeSlackClientGetter(mockSlackAPI),
			)

			if err != nil {
				t.Fatalf("Expected Run to succeed, got error: %v", err)
			}
			if !mockSlackAPI.ReplacedCanvas.Called {
				t.Fatal("Expected the canvas to be refreshed")
			}
			if mockSlackAPI.ReplacedCanvas.CanvasID != testCanvasID {
				t.Errorf(
					"Expected canvas ID '%s', got '%s'", testCanvasID, mockSlackAPI.ReplacedCanvas.CanvasID,
				)
			}
			markdown := mockSlackAPI.ReplacedCanvas.Markdown
			assertCanvasContains(t, markdown,
				"## Open PRs", "## Work in Progress", "Open PR one", "Open PR two", "Draft PR one",
			)
			if !canvasFooterLine.MatchString(markdown) {
				t.Errorf("Expected an updated-at footer line on the canvas, got:\n%s", markdown)
			}
			if tc.groupByRepository && !strings.Contains(markdown, "### [test-org/test-repo]") {
				t.Errorf("Expected a repository sub-heading on the canvas, got:\n%s", markdown)
			}

			if mockSlackAPI.SentMessage.Blocks.SomePRItemContainsText("Draft PR one") {
				t.Error("Expected the draft PR to be left out of the reminder message")
			}
			if mockSlackAPI.SentMessage.Blocks.GetPRCount() != 2 {
				t.Errorf(
					"Expected 2 PRs in the reminder message, got %d",
					mockSlackAPI.SentMessage.Blocks.GetPRCount(),
				)
			}
		})
	}
}

// The cap flags come from the fetch, no PR count downstream can tell that a cap fired.
func TestPostModeCanvasReportsCappedFetch(t *testing.T) {
	testCases := []struct {
		name         string
		openPRCount  int
		draftPRCount int
		expectedNote string
	}{
		{
			name:         "more drafts than the draft fetch cap",
			openPRCount:  1,
			draftPRCount: 16,
			expectedNote: "_Fetch limited to the newest 15 WIP PRs_",
		},
		{
			name:         "more open PRs than the open PR fetch cap",
			openPRCount:  51,
			draftPRCount: 1,
			expectedNote: "_Fetch limited to the newest 50 open PRs_",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
				config.InputPRTrackerCanvasLink: testCanvasLink,
			})
			var prs []*github.PullRequest
			for number := 1; number <= tc.openPRCount; number++ {
				prs = append(prs, getTestPR(GetTestPROptions{
					Number: number, Title: "Open PR", AuthorLogin: "alice",
				}))
			}
			for number := tc.openPRCount + 1; number <= tc.openPRCount+tc.draftPRCount; number++ {
				prs = append(prs, getTestPR(GetTestPROptions{
					Number: number, Title: "Draft PR", AuthorLogin: "carol", Draft: github.Ptr(true),
				}))
			}

			mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
			err := main.Run(
				mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{PRs: prs}),
				mockslackclient.MakeSlackClientGetter(mockSlackAPI),
			)

			if err != nil {
				t.Fatalf("Expected Run to succeed, got error: %v", err)
			}
			assertCanvasContains(t, mockSlackAPI.ReplacedCanvas.Markdown, tc.expectedNote)
		})
	}
}

// A canvas wiped to "no open PRs" whenever GitHub is unreachable would be worse than a stale one.
func TestPostModeCanvasIsNotRefreshedWhenFetchFails(t *testing.T) {
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputPRTrackerCanvasLink: testCanvasLink,
	})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs:                   canvasTestPRs(),
			ListPRsResponseStatus: 500,
			PRServiceError:        errors.New("unable to fetch PRs"),
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err == nil {
		t.Fatal("Expected Run to fail when fetching PRs fails")
	}
	if mockSlackAPI.ReplacedCanvas.Called {
		t.Error("Expected the canvas not to be refreshed when the PR fetch failed")
	}
	if !strings.Contains(err.Error(), "error fetching pull requests") {
		t.Errorf("Expected the fetch error to be reported, got: %v", err)
	}
	if strings.Contains(err.Error(), "canvas") {
		t.Errorf("Expected only the fetch error to be reported, got: %v", err)
	}
}

// Update mode's canvas fetch is its own attempt: it can fail while the state-tracked message
// update succeeds, and a failed fetch must never wipe the canvas.
func TestUpdateModeCanvasIsNotRefreshedWhenFetchFails(t *testing.T) {
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputRunMode:             config.RunModeUpdate,
		config.InputPRTrackerCanvasLink: testCanvasLink,
	})

	trackedPR := getTestPR(GetTestPROptions{Number: 1, Title: "Tracked open PR", AuthorLogin: "alice"})
	mockState := getTestState(GetTestStateOptions{PRNumbers: []int{1}})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRsByNumber:            map[int]*github.PullRequest{1: trackedPR},
			PRs:                    []*github.PullRequest{trackedPR},
			MockStateForUpdateMode: &mockState,
			// Only the open PR listing fails, so the message update still goes through.
			ListPRsResponseStatus: 500,
			PRServiceError:        errors.New("unable to fetch PRs"),
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err == nil {
		t.Fatal("Expected Run to fail when the canvas PR fetch fails")
	}
	if !strings.Contains(err.Error(), "canvas") {
		t.Errorf("Expected the error to name the canvas, got: %v", err)
	}
	if mockSlackAPI.ReplacedCanvas.Called {
		t.Error("Expected the canvas not to be refreshed when the canvas PR fetch failed")
	}
	if !mockSlackAPI.UpdatedMessage.Blocks.SomePRItemContainsText("Tracked open PR") {
		t.Error("Expected the reminder message to be updated anyway")
	}
}

// No PRs and no no-prs-message sends nothing, but the canvas still gets refreshed.
func TestPostModeCanvasIsRefreshedWhenNoPRsAreFound(t *testing.T) {
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputPRTrackerCanvasLink: testCanvasLink,
	})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	if mockSlackAPI.SentMessage.ChannelID != "" {
		t.Error("Expected no message to be sent when there are no PRs and no no-prs-message")
	}
	if !mockSlackAPI.ReplacedCanvas.Called {
		t.Fatal("Expected the canvas to be refreshed when the fetch found no PRs")
	}
	assertCanvasContains(t, mockSlackAPI.ReplacedCanvas.Markdown, "_No open PRs_", "_No work in progress_")
}

func TestCanvasIsNotRefreshedWhenLinkIsUnset(t *testing.T) {
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), nil)

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs: canvasTestPRs(),
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	if mockSlackAPI.ReplacedCanvas.Called {
		t.Error("Expected no canvas refresh when the canvas link is unset")
	}
}

// Update mode's message is state-tracked while its canvas shows what is open right now,
// so the two fetches carry deliberately different PRs here.
func TestUpdateModeCanvasShowsCurrentlyOpenPRs(t *testing.T) {
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputRunMode:             config.RunModeUpdate,
		config.InputPRTrackerCanvasLink: testCanvasLink,
	})

	trackedOpenPR := getTestPR(GetTestPROptions{Number: 1, Title: "Tracked open PR", AuthorLogin: "alice"})
	trackedMergedPR := getTestPR(GetTestPROptions{
		Number: 2, Title: "Tracked merged PR", AuthorLogin: "bob", State: "closed", Merged: true,
	})
	untrackedOpenPR := getTestPR(GetTestPROptions{Number: 3, Title: "Untracked open PR", AuthorLogin: "carol"})
	untrackedDraftPR := getTestPR(GetTestPROptions{
		Number: 4, Title: "Untracked draft PR", AuthorLogin: "dave", Draft: github.Ptr(true),
	})
	mockState := getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRsByNumber:            map[int]*github.PullRequest{1: trackedOpenPR, 2: trackedMergedPR},
			PRs:                    []*github.PullRequest{trackedOpenPR, untrackedOpenPR, untrackedDraftPR},
			MockStateForUpdateMode: &mockState,
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	if !mockSlackAPI.ReplacedCanvas.Called {
		t.Fatal("Expected the canvas to be refreshed in update mode")
	}
	markdown := mockSlackAPI.ReplacedCanvas.Markdown
	assertCanvasContains(t, markdown, "Tracked open PR", "Untracked open PR", "Untracked draft PR")
	assertCanvasDoesNotContain(t, markdown, "Tracked merged PR")

	updatedMessage := mockSlackAPI.UpdatedMessage.Blocks
	if !updatedMessage.SomePRItemContainsText("Tracked merged PR") {
		t.Error("Expected the state-tracked merged PR in the updated message")
	}
	if updatedMessage.SomePRItemContainsText("Untracked open PR") {
		t.Error("Expected an open PR that is not in state to stay out of the updated message")
	}
}

// The canvas refresh and the message path are independent attempts: either failing must not
// stop the other, and both errors reach the run's exit code.
func TestCanvasAndMessageFailuresAreIsolated(t *testing.T) {
	t.Run("canvas fails, message is still sent and state saved", func(t *testing.T) {
		stateFilePath := filepath.Join(t.TempDir(), stateFileName)
		testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
			config.InputPRTrackerCanvasLink: testCanvasLink,
			config.EnvStateFilePath:         stateFilePath,
		})

		mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{
			ReplaceCanvasError: errors.New("canvas_not_found"),
		})
		err := main.Run(
			mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
				PRs: canvasTestPRs(),
			}),
			mockslackclient.MakeSlackClientGetter(mockSlackAPI),
		)

		if err == nil {
			t.Fatal("Expected Run to fail when the canvas refresh fails")
		}
		if !strings.Contains(err.Error(), "canvas") {
			t.Errorf("Expected the error to name the canvas, got: %v", err)
		}
		if mockSlackAPI.SentMessage.Blocks.GetPRCount() != 2 {
			t.Errorf(
				"Expected the reminder message to be sent anyway, got %d PRs in it",
				mockSlackAPI.SentMessage.Blocks.GetPRCount(),
			)
		}
		if _, statErr := os.Stat(stateFilePath); statErr != nil {
			t.Errorf("Expected the state file to be saved anyway: %v", statErr)
		}
	})

	t.Run("message fails, canvas is still written", func(t *testing.T) {
		testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
			config.InputPRTrackerCanvasLink: testCanvasLink,
		})

		mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{
			PostMessageError: errors.New("error in sending Slack message"),
		})
		err := main.Run(
			mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
				PRs: canvasTestPRs(),
			}),
			mockslackclient.MakeSlackClientGetter(mockSlackAPI),
		)

		if err == nil {
			t.Fatal("Expected Run to fail when sending the message fails")
		}
		if !strings.Contains(err.Error(), "failed to send Slack message") {
			t.Errorf("Expected the message error to be reported, got: %v", err)
		}
		if !mockSlackAPI.ReplacedCanvas.Called {
			t.Error("Expected the canvas to be refreshed even though the message failed")
		}
	})

	t.Run("both fail, both errors are reported", func(t *testing.T) {
		testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
			config.InputPRTrackerCanvasLink: testCanvasLink,
		})

		mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{
			PostMessageError:   errors.New("error in sending Slack message"),
			ReplaceCanvasError: errors.New("canvas_not_found"),
		})
		err := main.Run(
			mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
				PRs: canvasTestPRs(),
			}),
			mockslackclient.MakeSlackClientGetter(mockSlackAPI),
		)

		if err == nil {
			t.Fatal("Expected Run to fail when both the message and the canvas fail")
		}
		if !strings.Contains(err.Error(), "failed to send Slack message") {
			t.Errorf("Expected the message error to be reported, got: %v", err)
		}
		if !strings.Contains(err.Error(), "canvas_not_found") {
			t.Errorf("Expected the canvas error to be reported, got: %v", err)
		}
	})
}

// Turning the canvas on switches drafts into the fetch. They must reach the canvas only,
// leaving every message block other than the canvas link footer byte-identical.
func TestCanvasDoesNotChangeMessageBlocks(t *testing.T) {
	runAndGetSentBlocks := func(t *testing.T, canvasLink string) []json.RawMessage {
		t.Helper()
		overrides, sentSlackBlocksFilePath := getFilePathOverrides(t)
		maps.Copy(overrides, map[string]any{config.InputPRTrackerCanvasLink: canvasLink})
		testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &overrides)

		mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
		err := main.Run(
			mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
				PRs: canvasTestPRs(),
			}),
			mockslackclient.MakeSlackClientGetter(mockSlackAPI),
		)
		if err != nil {
			t.Fatalf("Expected Run to succeed, got error: %v", err)
		}

		sentBlocks, readErr := os.ReadFile(sentSlackBlocksFilePath)
		if readErr != nil {
			t.Fatalf("Failed to read sent Slack blocks: %v", readErr)
		}
		// The file holds one entry per sent message, each entry the message's block array.
		var sentMessages [][]json.RawMessage
		if unmarshalErr := json.Unmarshal(sentBlocks, &sentMessages); unmarshalErr != nil {
			t.Fatalf("Failed to parse sent Slack blocks: %v", unmarshalErr)
		}
		if len(sentMessages) != 1 {
			t.Fatalf("Expected exactly one sent message, got %d", len(sentMessages))
		}
		return sentMessages[0]
	}

	blocksWithoutCanvas := runAndGetSentBlocks(t, "")
	blocksWithCanvas := runAndGetSentBlocks(t, testCanvasLink)

	if len(blocksWithCanvas) != len(blocksWithoutCanvas)+1 {
		t.Fatalf(
			"Expected the canvas link footer as the only extra block, got %d blocks against %d",
			len(blocksWithCanvas), len(blocksWithoutCanvas),
		)
	}
	for index, blockWithoutCanvas := range blocksWithoutCanvas {
		if string(blocksWithCanvas[index]) != string(blockWithoutCanvas) {
			t.Errorf(
				"Expected block %d to be identical.\nWithout canvas:\n%s\nWith canvas:\n%s",
				index, blockWithoutCanvas, blocksWithCanvas[index],
			)
		}
	}
}
