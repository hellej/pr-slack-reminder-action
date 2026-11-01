package main_test

import (
	"bytes"
	"errors"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-github/v78/github"
	main "github.com/hellej/pr-slack-reminder-action/cmd/pr-slack-reminder"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/state"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"github.com/hellej/pr-slack-reminder-action/testhelpers"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockgithubclient"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockslackclient"
)

const (
	readyToMergeHeading     = "✅ Ready to merge"
	waitingForAuthorHeading = "💬 Waiting for author"
	waitingForReviewHeading = "👀 Waiting for review"
	failingChecksEntry      = "❌ CI"
)

type requiredChecksRunOptions struct {
	runMode                    config.RunMode
	rememberedFailingPRNumbers []int
	// Only a post can run without a loaded state
	noLoadedState    bool
	failingPRNumbers []int
	passingPRNumbers []int
	pendingPRNumbers []int
	checkStatusError error
	// Defaults to PR 1, approved by bob
	prs               []*github.PullRequest
	reviewsByPRNumber map[int][]*github.PullRequestReview
	configOverrides   map[string]any
	recording         *mockgithubclient.FetchRecording
}

type requiredChecksRun struct {
	headingByTitle map[string]string
	textByTitle    map[string]string
	savedState     state.State
	mockSlackAPI   *mockslackclient.MockSlackAPI
	logOutput      *bytes.Buffer
}

func approvedPRFixture(number int, title string) *github.PullRequest {
	return getTestPR(GetTestPROptions{Number: number, Title: title, AuthorLogin: "alice", AgeHours: 3})
}

func approvedByBob(numbers ...int) map[int][]*github.PullRequestReview {
	reviewsByPRNumber := map[int][]*github.PullRequestReview{}
	for _, number := range numbers {
		reviewsByPRNumber[number] = []*github.PullRequestReview{
			mockgithubclient.NewReview("bob", "Bob Brown", "APPROVED"),
		}
	}
	return reviewsByPRNumber
}

func runWithRequiredChecks(t *testing.T, options requiredChecksRunOptions) requiredChecksRun {
	t.Helper()
	stateFilePath := filepath.Join(t.TempDir(), stateFileName)
	overrides := map[string]any{
		config.InputRunMode:     options.runMode,
		config.EnvStateFilePath: stateFilePath,
	}
	maps.Copy(overrides, options.configOverrides)
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &overrides)
	logOutput := testhelpers.CaptureLog(t)

	prs := options.prs
	reviewsByPRNumber := options.reviewsByPRNumber
	if prs == nil {
		prs = []*github.PullRequest{approvedPRFixture(1, "Fix retry loop")}
		reviewsByPRNumber = approvedByBob(1)
	}
	var loadedState *state.State
	if !options.noLoadedState {
		testState := getTestState(GetTestStateOptions{
			PRNumbers:                     []int{1},
			FailingRequiredCheckPRNumbers: options.rememberedFailingPRNumbers,
		})
		loadedState = &testState
	}
	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})

	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs:                           prs,
			ReviewsByPRNumber:             reviewsByPRNumber,
			FailingRequiredCheckPRNumbers: options.failingPRNumbers,
			PassingRequiredCheckPRNumbers: options.passingPRNumbers,
			PendingRequiredCheckPRNumbers: options.pendingPRNumbers,
			CheckStatusError:              options.checkStatusError,
			MockPreviousState:             loadedState,
			Recording:                     options.recording,
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
		false,
	)
	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}

	run := requiredChecksRun{
		headingByTitle: map[string]string{},
		textByTitle:    map[string]string{},
		savedState:     loadSavedState(t, stateFilePath),
		mockSlackAPI:   mockSlackAPI,
		logOutput:      logOutput,
	}
	for _, prList := range messageBlocksOfRun(mockSlackAPI, options.runMode).GetPRLists() {
		for _, item := range prList.PRListItems {
			for _, pr := range prs {
				if strings.Contains(item, pr.GetTitle()) {
					run.headingByTitle[pr.GetTitle()] = prList.Heading
					run.textByTitle[pr.GetTitle()] = item
				}
			}
		}
	}
	return run
}

func refsOfNumbers(numbers ...int) []models.PullRequestRef {
	return utilities.Map(numbers, stateRef)
}

func assertFixRetryLoopRow(t *testing.T, run requiredChecksRun, expectedHeading string, expectFailingChecksEntry bool) {
	t.Helper()
	if heading := run.headingByTitle["Fix retry loop"]; heading != expectedHeading {
		t.Errorf("Expected the PR under %q, got %q", expectedHeading, heading)
	}
	if hasEntry := strings.Contains(run.textByTitle["Fix retry loop"], failingChecksEntry); hasEntry != expectFailingChecksEntry {
		t.Errorf("Expected %q in the row: %v, got the row %q", failingChecksEntry, expectFailingChecksEntry, run.textByTitle["Fix retry loop"])
	}
}

func assertSavedFailingRefs(t *testing.T, run requiredChecksRun, expected []models.PullRequestRef) {
	t.Helper()
	saved := run.savedState.FailingRequiredCheckPRs
	if len(saved) == 0 && len(expected) == 0 {
		return
	}
	if !slices.Equal(saved, expected) {
		t.Errorf("Expected the saved failing PRs %v, got %v", expected, saved)
	}
}

var bothRunModes = []config.RunMode{config.RunModePost, config.RunModeUpdate}

// Each CI status rule of plan 019's requirements, in both run modes.
func TestRequiredCheckStatusChangesOnlyOnEvidence(t *testing.T) {
	testCases := []struct {
		name                       string
		rememberedFailingPRNumbers []int
		failingPRNumbers           []int
		passingPRNumbers           []int
		pendingPRNumbers           []int
		checkStatusError           error
		expectFailing              bool
	}{
		{name: "red turns green once every required check passed", rememberedFailingPRNumbers: []int{1}, passingPRNumbers: []int{1}},
		{name: "green turns red on a failed required check", failingPRNumbers: []int{1}, expectFailing: true},
		{name: "red stays red while the required checks run", rememberedFailingPRNumbers: []int{1}, pendingPRNumbers: []int{1}, expectFailing: true},
		{name: "green stays green while the required checks run", pendingPRNumbers: []int{1}},
		{name: "red stays red with no check reported", rememberedFailingPRNumbers: []int{1}, expectFailing: true},
		{
			name:                       "red stays red when the check status fetch fails",
			rememberedFailingPRNumbers: []int{1},
			checkStatusError:           errors.New("forbidden"),
			expectFailing:              true,
		},
		{name: "green stays green when the check status fetch fails", checkStatusError: errors.New("forbidden")},
	}

	for _, runMode := range bothRunModes {
		for _, tc := range testCases {
			t.Run(string(runMode)+": "+tc.name, func(t *testing.T) {
				run := runWithRequiredChecks(t, requiredChecksRunOptions{
					runMode:                    runMode,
					rememberedFailingPRNumbers: tc.rememberedFailingPRNumbers,
					failingPRNumbers:           tc.failingPRNumbers,
					passingPRNumbers:           tc.passingPRNumbers,
					pendingPRNumbers:           tc.pendingPRNumbers,
					checkStatusError:           tc.checkStatusError,
				})

				if tc.expectFailing {
					assertFixRetryLoopRow(t, run, waitingForAuthorHeading, true)
					assertSavedFailingRefs(t, run, refsOfNumbers(1))
					return
				}
				assertFixRetryLoopRow(t, run, readyToMergeHeading, false)
				assertSavedFailingRefs(t, run, nil)
			})
		}
	}
}

func TestPostWithoutALoadedStateCountsAPRAsGreenUnlessItsRequiredCheckFails(t *testing.T) {
	testCases := []struct {
		name             string
		failingPRNumbers []int
		pendingPRNumbers []int
		expectFailing    bool
	}{
		{name: "pending required checks", pendingPRNumbers: []int{1}},
		{name: "no check reported"},
		{name: "a failed required check", failingPRNumbers: []int{1}, expectFailing: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run := runWithRequiredChecks(t, requiredChecksRunOptions{
				runMode:          config.RunModePost,
				noLoadedState:    true,
				failingPRNumbers: tc.failingPRNumbers,
				pendingPRNumbers: tc.pendingPRNumbers,
			})

			if tc.expectFailing {
				assertFixRetryLoopRow(t, run, waitingForAuthorHeading, true)
				assertSavedFailingRefs(t, run, refsOfNumbers(1))
				return
			}
			assertFixRetryLoopRow(t, run, readyToMergeHeading, false)
			assertSavedFailingRefs(t, run, nil)
		})
	}
}

func TestBothTracksReadTheRememberedFailingStatus(t *testing.T) {
	for _, runMode := range bothRunModes {
		t.Run(string(runMode), func(t *testing.T) {
			run := runWithRequiredChecks(t, requiredChecksRunOptions{
				runMode:                    runMode,
				rememberedFailingPRNumbers: []int{1},
				pendingPRNumbers:           []int{1},
				configOverrides:            map[string]any{config.InputPRTrackerCanvasLink: testCanvasLink},
			})

			assertFixRetryLoopRow(t, run, waitingForAuthorHeading, true)
			canvasMarkdown := run.mockSlackAPI.ReplacedCanvas.Markdown
			if !strings.Contains(canvasMarkdown, "## 💬 Waiting for author") ||
				!strings.Contains(canvasMarkdown, "(✅ Bob Brown / ❌ CI)") {
				t.Errorf("Expected the canvas row under waiting for author with %q, got:\n%s", failingChecksEntry, canvasMarkdown)
			}
			if !strings.Contains(run.logOutput.String(), "test-org/test-repo/1") ||
				!strings.Contains(run.logOutput.String(), "failing a required check, as the previous run saw it") {
				t.Errorf("Expected a log line naming PR 1 as failing from the state, got:\n%s", run.logOutput)
			}
		})
	}
}

// PR 2 is not approved, PR 3 is a draft: neither is fetched for check status, so both stay
// failing from the state. PR 9 left the open fetch, so it leaves the set.
func TestRememberedFailingPRsNotFetchedForCheckStatusStayFailing(t *testing.T) {
	for _, runMode := range bothRunModes {
		t.Run(string(runMode), func(t *testing.T) {
			recording := &mockgithubclient.FetchRecording{}
			draft := getTestPR(GetTestPROptions{
				Number: 3, Title: "Draft the migration", AuthorLogin: "carol", Draft: github.Ptr(true),
			})
			reviewsByPRNumber := approvedByBob(1, 3)
			reviewsByPRNumber[2] = []*github.PullRequestReview{
				mockgithubclient.NewReview("bob", "Bob Brown", "COMMENTED"),
			}

			run := runWithRequiredChecks(t, requiredChecksRunOptions{
				runMode:                    runMode,
				rememberedFailingPRNumbers: []int{2, 3, 9},
				prs: []*github.PullRequest{
					approvedPRFixture(1, "Fix retry loop"),
					approvedPRFixture(2, "Only commented on"),
					draft,
				},
				reviewsByPRNumber: reviewsByPRNumber,
				configOverrides:   map[string]any{config.InputPRTrackerCanvasLink: testCanvasLink},
				recording:         recording,
			})

			if !slices.EqualFunc(recording.CheckStatusRequests, [][]models.PullRequestRef{refsOfNumbers(1)}, slices.Equal) {
				t.Errorf("Expected one check status request for PR 1, got %v", recording.CheckStatusRequests)
			}
			assertSavedFailingRefs(t, run, refsOfNumbers(2, 3))
			if heading := run.headingByTitle["Only commented on"]; heading != waitingForReviewHeading {
				t.Errorf("Expected the unapproved PR under %q, got %q", waitingForReviewHeading, heading)
			}
			if strings.Contains(run.textByTitle["Only commented on"], failingChecksEntry) {
				t.Errorf("Expected no %q on the unapproved PR, got %q", failingChecksEntry, run.textByTitle["Only commented on"])
			}
			if strings.Contains(run.mockSlackAPI.ReplacedCanvas.Markdown, failingChecksEntry) {
				t.Errorf("Expected no %q on the canvas, got:\n%s", failingChecksEntry, run.mockSlackAPI.ReplacedCanvas.Markdown)
			}
		})
	}
}

func TestPRsSeenFailingAndRememberedFailingAreBothSaved(t *testing.T) {
	for _, runMode := range bothRunModes {
		t.Run(string(runMode), func(t *testing.T) {
			run := runWithRequiredChecks(t, requiredChecksRunOptions{
				runMode:                    runMode,
				rememberedFailingPRNumbers: []int{2},
				failingPRNumbers:           []int{1},
				prs: []*github.PullRequest{
					approvedPRFixture(1, "Fix retry loop"),
					approvedPRFixture(2, "Remembered failing"),
				},
				reviewsByPRNumber: approvedByBob(1, 2),
			})

			assertSavedFailingRefs(t, run, refsOfNumbers(1, 2))
		})
	}
}

func TestPostAndUpdateWarnWhenTheCheckStatusFetchFails(t *testing.T) {
	for _, runMode := range bothRunModes {
		t.Run(string(runMode), func(t *testing.T) {
			run := runWithRequiredChecks(t, requiredChecksRunOptions{
				runMode:          runMode,
				checkStatusError: errors.New("resource not accessible by integration"),
			})

			assertFixRetryLoopRow(t, run, readyToMergeHeading, false)
			warningLines := testhelpers.LogLinesStartingWith(run.logOutput, "::warning")
			if len(warningLines) != 1 ||
				!strings.HasPrefix(warningLines[0], warningAnnotationPrefix+"Failed to fetch required check status: ") ||
				!strings.Contains(warningLines[0], "resource not accessible by integration") {
				t.Errorf("Expected one check status warning, got %q", warningLines)
			}
		})
	}
}

// The loaded state lists PR 9 as failing, but PR 9 left the open fetch, so the state saved back
// on either path carries the empty set this run saw, not the loaded one.
func TestUpdateModeSavesThisRunsFailingSetWhenItSavesTheLoadedStateBack(t *testing.T) {
	testCases := []struct {
		name                 string
		mergedPRsSearchError error
	}{
		{name: "message deleted"},
		{name: "message kept because the merged PR fetch failed", mergedPRsSearchError: errors.New("search failed")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stateFilePath := filepath.Join(t.TempDir(), stateFileName)
			testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
				config.InputRunMode:     config.RunModeUpdate,
				config.EnvStateFilePath: stateFilePath,
			})
			loadedState := getTestState(GetTestStateOptions{
				PRNumbers: []int{9}, FailingRequiredCheckPRNumbers: []int{9},
			})
			loadedState.LastWrittenMessage = seededLastWrittenMessage
			mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})

			err := main.Run(
				mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
					PRsByNumber: map[int]*github.PullRequest{
						9: getTestPR(GetTestPROptions{Number: 9, State: "closed"}),
					},
					MergedPRsSearchError: tc.mergedPRsSearchError,
					MockPreviousState:    &loadedState,
				}),
				mockslackclient.MakeSlackClientGetter(mockSlackAPI),
				false,
			)

			if err != nil {
				t.Fatalf("Expected Run to succeed, got error: %v", err)
			}
			savedState := loadSavedState(t, stateFilePath)
			assertLastWrittenMessageIsTheSeededOne(t, savedState.LastWrittenMessage)
			if len(savedState.FailingRequiredCheckPRs) != 0 {
				t.Errorf("Expected no saved failing PRs, got %v", savedState.FailingRequiredCheckPRs)
			}
		})
	}
}
