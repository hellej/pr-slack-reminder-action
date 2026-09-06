package main_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-github/v78/github"
	main "github.com/hellej/pr-slack-reminder-action/cmd/pr-slack-reminder"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/state"
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

// Merged PRs come from their own search, never from the open PR listing.
func canvasTestMergedPRs() []*github.PullRequest {
	return []*github.PullRequest{
		getTestPR(GetTestPROptions{
			Number: 11, Title: "Merged PR one", AuthorLogin: "alice", MergedHoursAgo: 30,
		}),
		getTestPR(GetTestPROptions{
			Number: 12, Title: "Merged PR two", AuthorLogin: "bob", MergedHoursAgo: 2,
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
					PRs:       prs,
					MergedPRs: canvasTestMergedPRs(),
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
				"## Open\n", "## WIP\n", "## Merged\n",
				"Open PR one", "Open PR two", "Draft PR one",
				"**[Merged PR two]", "_merged 2 hours ago_ by Bob 🚀",
			)
			if strings.Index(markdown, "Merged PR two") > strings.Index(markdown, "Merged PR one") {
				t.Errorf("Expected the newest merge first on the canvas, got:\n%s", markdown)
			}
			if !canvasFooterLine.MatchString(markdown) {
				t.Errorf("Expected an updated-at footer line on the canvas, got:\n%s", markdown)
			}
			if tc.groupByRepository && !strings.Contains(markdown, "### [test-org/test-repo]") {
				t.Errorf("Expected a repository sub-heading on the canvas, got:\n%s", markdown)
			}

			if mockSlackAPI.SentMessage.Blocks.SomePRItemContainsText("Draft PR one") {
				t.Error("Expected the draft PR to be left out of the reminder message")
			}
			if mockSlackAPI.SentMessage.Blocks.SomePRItemContainsText("Merged PR two") {
				t.Error("Expected a merged PR to be left out of the reminder message")
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
			expectedNote: "_Fetch limited to the newest 10 WIP PRs_",
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
	// The canvas refresh retries the fetch and fails on it too, so it reports its own failure.
	if !strings.Contains(err.Error(), "PR tracker canvas refresh failed") {
		t.Errorf("Expected the canvas failure to be reported, got: %v", err)
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
	assertCanvasContains(
		t, mockSlackAPI.ReplacedCanvas.Markdown,
		"_No open PRs_", "_No work in progress_", "_No merged PRs_",
	)
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
			MergedPRs:              canvasTestMergedPRs(),
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
	assertCanvasContains(
		t, markdown,
		"Tracked open PR", "Untracked open PR", "Untracked draft PR", "Merged PR one", "Merged PR two",
	)
	assertCanvasDoesNotContain(t, markdown, "Tracked merged PR")

	updatedMessage := mockSlackAPI.UpdatedMessage.Blocks
	if !updatedMessage.SomePRItemContainsText("Tracked merged PR") {
		t.Error("Expected the state-tracked merged PR in the updated message")
	}
	if updatedMessage.SomePRItemContainsText("Untracked open PR") {
		t.Error("Expected an open PR that is not in state to stay out of the updated message")
	}
	if updatedMessage.SomePRItemContainsText("Merged PR one") {
		t.Error("Expected a searched merged PR to stay out of the updated message")
	}
}

// The search cutoff is a day, so it returns up to one extra day of merges.
func TestCanvasLeavesOutMergesOlderThanTheWindow(t *testing.T) {
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputPRTrackerCanvasLink: testCanvasLink,
	})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs: canvasTestPRs(),
			MergedPRs: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number: 11, Title: "Merged inside the window", AuthorLogin: "alice",
					MergedHoursAgo: 7*24 - 1,
				}),
				getTestPR(GetTestPROptions{
					Number: 12, Title: "Merged before the window", AuthorLogin: "bob",
					MergedHoursAgo: 7*24 + 1,
				}),
			},
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	assertCanvasContains(t, mockSlackAPI.ReplacedCanvas.Markdown, "Merged inside the window")
	assertCanvasDoesNotContain(t, mockSlackAPI.ReplacedCanvas.Markdown, "Merged before the window")
}

// Each repository's merges are searched under their own alias, so a repository filter has to
// land on the merges of that repository only.
func TestCanvasAppliesRepositoryFiltersToMergedPRs(t *testing.T) {
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputPRTrackerCanvasLink: testCanvasLink,
		config.InputGithubRepositories:  "some-org/repo1; some-org/repo2",
		config.InputRepositoryFilters:   "repo1: {\"ignored-authors\": [\"alice\"]}",
	})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRsByRepo: map[string][]*github.PullRequest{"repo1": {}, "repo2": {}},
			MergedPRsByRepo: map[string][]*github.PullRequest{
				"repo1": {getTestPR(GetTestPROptions{
					Number: 11, Title: "Merged in repo1 by Alice", AuthorLogin: "alice",
					MergedHoursAgo: 2,
				})},
				"repo2": {getTestPR(GetTestPROptions{
					Number: 12, Title: "Merged in repo2 by Alice", AuthorLogin: "alice",
					MergedHoursAgo: 3,
				})},
			},
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	assertCanvasContains(t, mockSlackAPI.ReplacedCanvas.Markdown, "Merged in repo2 by Alice")
	assertCanvasDoesNotContain(t, mockSlackAPI.ReplacedCanvas.Markdown, "Merged in repo1 by Alice")
}

// A failed merged search degrades one section instead of the whole canvas, and still fails the run.
func TestCanvasIsWrittenWhenTheMergedFetchFails(t *testing.T) {
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputPRTrackerCanvasLink: testCanvasLink,
	})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs:                  canvasTestPRs(),
			MergedPRs:            canvasTestMergedPRs(),
			MergedPRsSearchError: errors.New("unable to search merged PRs"),
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err == nil {
		t.Fatal("Expected Run to fail when the merged PR fetch fails")
	}
	if !strings.Contains(err.Error(), "unable to search merged PRs") {
		t.Errorf("Expected the merged fetch error to be reported, got: %v", err)
	}
	if !mockSlackAPI.ReplacedCanvas.Called {
		t.Fatal("Expected the canvas to be refreshed anyway")
	}
	markdown := mockSlackAPI.ReplacedCanvas.Markdown
	assertCanvasContains(t, markdown, "Open PR one", "_Merged PRs could not be fetched_")
	assertCanvasDoesNotContain(t, markdown, "Merged PR one", "_No merged PRs_")
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

// A post run with the canvas on, returning the state it saved. Post mode loads no state, so it
// always writes the canvas: its saved hash is the hash of what is on the canvas now.
func runPostModeAndLoadSavedState(t *testing.T, prs []*github.PullRequest) state.State {
	t.Helper()
	stateFilePath := filepath.Join(t.TempDir(), stateFileName)
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputPRTrackerCanvasLink: testCanvasLink,
		config.EnvStateFilePath:         stateFilePath,
	})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs:       prs,
			MergedPRs: canvasTestMergedPRs(),
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)
	if err != nil {
		t.Fatalf("Expected the post run to succeed, got error: %v", err)
	}
	if !mockSlackAPI.ReplacedCanvas.Called {
		t.Fatal("Expected the post run to write the canvas")
	}

	var savedState state.State
	if loadErr := testhelpers.LoadJSONFromFile(stateFilePath, &savedState); loadErr != nil {
		t.Fatalf("Failed to load the state saved by the post run: %v", loadErr)
	}
	if savedState.CanvasContentHash == "" {
		t.Fatal("Expected the post run to save the hash of what it put on the canvas")
	}
	return savedState
}

// The saved hash has to be the hash of the markdown that was written, footer timestamp aside.
// A hash taken from a separately built Content would let the two drift, and the next run would
// then skip a write the canvas needs. The long-idle draft is the case where they drift: a zero
// "now" keeps it on the canvas, and its row wording no longer moves with age.
func TestPostModeSavesTheHashOfTheWrittenCanvasMarkdown(t *testing.T) {
	stateFilePath := filepath.Join(t.TempDir(), stateFileName)
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputPRTrackerCanvasLink: testCanvasLink,
		config.EnvStateFilePath:         stateFilePath,
	})
	longIdleDraftPR := getTestPR(GetTestPROptions{
		Number: 4, Title: "Draft PR two", AuthorLogin: "dave", AgeHours: 10,
		Draft: github.Ptr(true), UpdatedDaysAgo: 100,
	})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs:       append(canvasTestPRs(), longIdleDraftPR),
			MergedPRs: canvasTestMergedPRs(),
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	assertCanvasDoesNotContain(t, mockSlackAPI.ReplacedCanvas.Markdown, "Draft PR two")

	markdownWithZeroedFooter := canvasFooterLine.ReplaceAllString(
		mockSlackAPI.ReplacedCanvas.Markdown, "_Updated 0001-01-01 00:00 UTC_",
	)
	digest := sha256.Sum256([]byte(markdownWithZeroedFooter))
	expectedHash := hex.EncodeToString(digest[:])

	var savedState state.State
	if loadErr := testhelpers.LoadJSONFromFile(stateFilePath, &savedState); loadErr != nil {
		t.Fatalf("Failed to load the saved state file: %v", loadErr)
	}
	if savedState.CanvasContentHash != expectedHash {
		t.Errorf(
			"Expected the hash of the written markdown %s, got %s",
			expectedHash, savedState.CanvasContentHash,
		)
	}
}

// An update run whose canvas markdown is what is already on the canvas, using the state a post
// run saved as the seed. The Slack canvas client mis-merges a replace that lands while someone
// has the canvas open, so a write that would change nothing is worth skipping.
func TestUpdateModeSkipsTheCanvasWriteWhenTheContentIsUnchanged(t *testing.T) {
	prs := canvasTestPRs()
	seedState := runPostModeAndLoadSavedState(t, prs)

	stateFilePath := filepath.Join(t.TempDir(), stateFileName)
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputRunMode:             config.RunModeUpdate,
		config.InputPRTrackerCanvasLink: testCanvasLink,
		config.EnvStateFilePath:         stateFilePath,
	})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs:                    prs,
			PRsByNumber:            map[int]*github.PullRequest{1: prs[0], 2: prs[1]},
			MergedPRs:              canvasTestMergedPRs(),
			MockStateForUpdateMode: &seedState,
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	if mockSlackAPI.ReplacedCanvas.Called {
		t.Errorf(
			"Expected no canvas write when the content is unchanged, got:\n%s",
			mockSlackAPI.ReplacedCanvas.Markdown,
		)
	}
	var savedState state.State
	if loadErr := testhelpers.LoadJSONFromFile(stateFilePath, &savedState); loadErr != nil {
		t.Fatalf("Failed to load the saved state file: %v", loadErr)
	}
	if savedState.CanvasContentHash != seedState.CanvasContentHash {
		t.Errorf(
			"Expected the saved hash to stay %s, got %s",
			seedState.CanvasContentHash, savedState.CanvasContentHash,
		)
	}
}

func TestUpdateModeWritesTheCanvasWhenTheSeededHashDoesNotMatch(t *testing.T) {
	prs := canvasTestPRs()
	seedFromSamePRs := runPostModeAndLoadSavedState(t, prs)
	seedWithoutHash := getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})

	testCases := []struct {
		name      string
		seedState state.State
		prs       []*github.PullRequest
	}{
		{
			name:      "the canvas content changed since the seeded run",
			seedState: seedFromSamePRs,
			prs: append(prs, getTestPR(GetTestPROptions{
				Number: 4, Title: "Open PR three", AuthorLogin: "dave", AgeHours: 7,
			})),
		},
		{
			name:      "the seeded state carries no hash",
			seedState: seedWithoutHash,
			prs:       prs,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stateFilePath := filepath.Join(t.TempDir(), stateFileName)
			testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
				config.InputRunMode:             config.RunModeUpdate,
				config.InputPRTrackerCanvasLink: testCanvasLink,
				config.EnvStateFilePath:         stateFilePath,
			})
			seedState := tc.seedState

			mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
			err := main.Run(
				mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
					PRs:                    tc.prs,
					PRsByNumber:            map[int]*github.PullRequest{1: prs[0], 2: prs[1]},
					MergedPRs:              canvasTestMergedPRs(),
					MockStateForUpdateMode: &seedState,
				}),
				mockslackclient.MakeSlackClientGetter(mockSlackAPI),
			)

			if err != nil {
				t.Fatalf("Expected Run to succeed, got error: %v", err)
			}
			if !mockSlackAPI.ReplacedCanvas.Called {
				t.Error("Expected the canvas to be written")
			}
			var savedState state.State
			if loadErr := testhelpers.LoadJSONFromFile(stateFilePath, &savedState); loadErr != nil {
				t.Fatalf("Failed to load the saved state file: %v", loadErr)
			}
			if savedState.CanvasContentHash == tc.seedState.CanvasContentHash {
				t.Errorf(
					"Expected the saved hash to be the written content's, got the seeded %s",
					savedState.CanvasContentHash,
				)
			}
			if savedState.CanvasContentHash == "" {
				t.Error("Expected the written content's hash to be saved")
			}
		})
	}
}

// Dropping the hash on a run that wrote nothing would make the next run rewrite the canvas.
func TestUpdateModeCarriesTheSeededHashWhenNothingIsWritten(t *testing.T) {
	prs := canvasTestPRs()
	seedState := runPostModeAndLoadSavedState(t, prs)

	testCases := []struct {
		name           string
		canvasLink     string
		prFetchStatus  int
		prServiceError error
	}{
		{name: "the canvas is disabled"},
		{
			name:           "the canvas PR fetch fails",
			canvasLink:     testCanvasLink,
			prFetchStatus:  500,
			prServiceError: errors.New("unable to fetch PRs"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stateFilePath := filepath.Join(t.TempDir(), stateFileName)
			testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
				config.InputRunMode:             config.RunModeUpdate,
				config.InputPRTrackerCanvasLink: tc.canvasLink,
				config.EnvStateFilePath:         stateFilePath,
			})
			seedStateForRun := seedState

			mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
			// The fetch-failure case fails the run, and saves state either way.
			_ = main.Run(
				mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
					PRs:                    prs,
					PRsByNumber:            map[int]*github.PullRequest{1: prs[0], 2: prs[1]},
					MergedPRs:              canvasTestMergedPRs(),
					MockStateForUpdateMode: &seedStateForRun,
					ListPRsResponseStatus:  tc.prFetchStatus,
					PRServiceError:         tc.prServiceError,
				}),
				mockslackclient.MakeSlackClientGetter(mockSlackAPI),
			)

			if mockSlackAPI.ReplacedCanvas.Called {
				t.Error("Expected no canvas write")
			}
			var savedState state.State
			if loadErr := testhelpers.LoadJSONFromFile(stateFilePath, &savedState); loadErr != nil {
				t.Fatalf("Failed to load the saved state file: %v", loadErr)
			}
			if savedState.CanvasContentHash != seedState.CanvasContentHash {
				t.Errorf(
					"Expected the saved hash to stay the seeded %s, got %s",
					seedState.CanvasContentHash, savedState.CanvasContentHash,
				)
			}
		})
	}
}

// Recording a failed write as applied would leave the canvas stale until its content changes again.
func TestUpdateModeKeepsTheSeededHashWhenTheCanvasWriteFails(t *testing.T) {
	prs := canvasTestPRs()
	seedState := runPostModeAndLoadSavedState(t, prs)

	stateFilePath := filepath.Join(t.TempDir(), stateFileName)
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputRunMode:             config.RunModeUpdate,
		config.InputPRTrackerCanvasLink: testCanvasLink,
		config.EnvStateFilePath:         stateFilePath,
	})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{
		ReplaceCanvasError: errors.New("canvas_not_found"),
	})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs: append(prs, getTestPR(GetTestPROptions{
				Number: 4, Title: "Open PR three", AuthorLogin: "dave", AgeHours: 7,
			})),
			PRsByNumber:            map[int]*github.PullRequest{1: prs[0], 2: prs[1]},
			MergedPRs:              canvasTestMergedPRs(),
			MockStateForUpdateMode: &seedState,
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err == nil {
		t.Fatal("Expected Run to fail when the canvas write fails")
	}
	var savedState state.State
	if loadErr := testhelpers.LoadJSONFromFile(stateFilePath, &savedState); loadErr != nil {
		t.Fatalf("Failed to load the saved state file: %v", loadErr)
	}
	if savedState.CanvasContentHash != seedState.CanvasContentHash {
		t.Errorf(
			"Expected the saved hash to stay the seeded %s, got %s",
			seedState.CanvasContentHash, savedState.CanvasContentHash,
		)
	}
}

// Turning the canvas on switches drafts into the fetch. They must reach the canvas only,
// leaving the reminder message byte-identical.
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

	if len(blocksWithCanvas) != len(blocksWithoutCanvas) {
		t.Fatalf(
			"Expected the same number of blocks, got %d with the canvas against %d without",
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
