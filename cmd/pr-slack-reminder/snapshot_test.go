package main_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/google/go-github/v78/github"
	main "github.com/hellej/pr-slack-reminder-action/cmd/pr-slack-reminder"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/state"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"github.com/hellej/pr-slack-reminder-action/testhelpers"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockgithubclient"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockslackclient"
)

var updateSnapshots = flag.Bool(
	"update-snapshots", false, "record the sent Slack blocks as snapshot files instead of comparing against them",
)

const snapshotDirectory = "testdata/snapshots"

const stateFileName = "pr-slack-reminder-state.json"

var nonAlphanumericRuns = regexp.MustCompile(`[^a-zA-Z0-9]+`)

var updateTimeFooterTimestamp = regexp.MustCompile(`!date\^\d+\^\{time\}\|\d{2}:\d{2} UTC`)

var stalenessWarningTimestamp = regexp.MustCompile(`!date\^\d+\^\{date_pretty\} at \{time\}\|[A-Z][a-z]{2} \d{1,2} \d{2}:\d{2} UTC`)

// Run stamps the update-time footer, and the staleness warning after it, with the real clock,
// so a snapshot recorded a second ago would never match again. The rest of the message survives
// a moving clock: the fixture ages come off this package's own `now`.
func withFixedMessageTimestamps(blocks []byte) []byte {
	blocks = updateTimeFooterTimestamp.ReplaceAll(blocks, []byte(`!date^0^{time}|00:00 UTC`))
	return stalenessWarningTimestamp.ReplaceAll(blocks, []byte(`!date^0^{date_pretty} at {time}|Jan 1 00:00 UTC`))
}

var stateTimestampField = regexp.MustCompile(`"(createdAt|generatedAt)": "[^"]*"`)

var neverSetTimeJSON = []byte(`"0001-01-01T00:00:00Z"`)

func withFixedStateTimestamps(stateJSON []byte) []byte {
	return stateTimestampField.ReplaceAllFunc(stateJSON, func(field []byte) []byte {
		if bytes.HasSuffix(field, neverSetTimeJSON) {
			return field
		}
		fieldName := stateTimestampField.FindSubmatch(field)[1]
		return []byte(`"` + string(fieldName) + `": "1970-01-01T00:00:00Z"`)
	})
}

func getSnapshotFilePath(t *testing.T, extension string) string {
	t.Helper()
	return filepath.Join(snapshotDirectory, nonAlphanumericRuns.ReplaceAllString(t.Name(), "-")+extension)
}

// Points the run at temporary output files and returns the sent Slack blocks file path.
func getFilePathOverrides(t *testing.T) (map[string]any, string) {
	t.Helper()
	tempDir := t.TempDir()
	sentSlackBlocksFilePath := filepath.Join(tempDir, "sent-slack-blocks.json")
	return map[string]any{
		config.EnvSentSlackBlocksFilePath: sentSlackBlocksFilePath,
		config.EnvStateFilePath:           filepath.Join(tempDir, stateFileName),
	}, sentSlackBlocksFilePath
}

func assertSentBlocksMatchSnapshot(t *testing.T, sentSlackBlocksFilePath string) {
	t.Helper()
	sentBlocks, err := os.ReadFile(sentSlackBlocksFilePath)
	if err != nil {
		t.Fatalf("Failed to read sent Slack blocks from %s: %v", sentSlackBlocksFilePath, err)
	}
	assertBlocksMatchSnapshot(t, sentBlocks)
}

func assertBlocksMatchSnapshot(t *testing.T, sentBlocks []byte) {
	t.Helper()
	assertMatchesSnapshot(t, withFixedMessageTimestamps(sentBlocks), getSnapshotFilePath(t, ".json"))
}

// The saved state is read by the next run, possibly under a newer action version, so its exact
// JSON is a contract.
func assertSavedStateMatchesSnapshot(t *testing.T, stateFilePath string) {
	t.Helper()
	savedState, err := os.ReadFile(stateFilePath)
	if err != nil {
		t.Fatalf("Failed to read the saved state from %s: %v", stateFilePath, err)
	}
	normalisedState := withFixedStateTimestamps(withFixedMessageTimestamps(savedState))
	assertMatchesSnapshot(t, normalisedState, getSnapshotFilePath(t, ".state.json"))
}

func assertMatchesSnapshot(t *testing.T, actual []byte, snapshotFilePath string) {
	t.Helper()
	if *updateSnapshots {
		if err := os.MkdirAll(snapshotDirectory, 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", snapshotDirectory, err)
		}
		if err := os.WriteFile(snapshotFilePath, actual, 0644); err != nil {
			t.Fatalf("Failed to write snapshot %s: %v", snapshotFilePath, err)
		}
		return
	}

	snapshot, err := os.ReadFile(snapshotFilePath)
	if os.IsNotExist(err) {
		t.Fatalf("Snapshot %s does not exist, run make update-test-snapshots to record it", snapshotFilePath)
	}
	if err != nil {
		t.Fatalf("Failed to read snapshot %s: %v", snapshotFilePath, err)
	}
	if !bytes.Equal(snapshot, actual) {
		t.Errorf(
			"Output does not match snapshot %s.\nSnapshot:\n%s\n\nActual:\n%s",
			snapshotFilePath, snapshot, actual,
		)
	}
}

type snapshotScenario struct {
	name                       string
	configOverrides            map[string]any
	prs                        []*github.PullRequest
	prsByRepo                  map[string][]*github.PullRequest
	mergedPRs                  []*github.PullRequest
	reviewsByPRNumber          map[int][]*github.PullRequestReview
	timelineCommentsByPRNumber map[int][]*github.IssueComment
}

func snapshotScenarios() []snapshotScenario {
	return []snapshotScenario{
		{
			name: "grouped by repository over two repositories",
			configOverrides: map[string]any{
				config.InputGithubRepositories: "test-org/repo-one; test-org/repo-two",
				config.InputGroupByRepository:  true,
			},
			prsByRepo: map[string][]*github.PullRequest{
				"repo-one": {
					getTestPR(GetTestPROptions{
						Number:      11,
						Title:       "Add pagination to the PR listing",
						HTMLURL:     "https://github.com/test-org/repo-one/pull/11",
						AuthorLogin: "alice",
						AuthorName:  "Alice Anderson",
						Labels:      []string{"feature"},
						AgeHours:    2,
					}),
				},
				"repo-two": {
					getTestPR(GetTestPROptions{
						Number:      21,
						Title:       "Fix the flaky reminder test",
						HTMLURL:     "https://github.com/test-org/repo-two/pull/21",
						AuthorLogin: "bob",
						AuthorName:  "Bob Brown",
						Labels:      []string{"fix"},
						AgeHours:    5,
					}),
					getTestPR(GetTestPROptions{
						Number:      22,
						Title:       "Bump the Slack SDK",
						HTMLURL:     "https://github.com/test-org/repo-two/pull/22",
						AuthorLogin: "carol",
						AuthorName:  "Carol Clark",
						Labels:      []string{"chore"},
						AgeHours:    8,
					}),
				},
			},
		},
		{
			name: "approver and commenter on one PR, commenter only on another",
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number:      31,
					Title:       "Refactor the review fetching",
					HTMLURL:     "https://github.com/test-org/test-repo/pull/31",
					AuthorLogin: "alice",
					AuthorName:  "Alice Anderson",
					Labels:      []string{"refactor"},
					AgeHours:    4,
				}),
				getTestPR(GetTestPROptions{
					Number:      32,
					Title:       "PR whose only reviewer input is a timeline comment",
					HTMLURL:     "https://github.com/test-org/test-repo/pull/32",
					AuthorLogin: "bob",
					AuthorName:  "Bob Brown",
					Labels:      []string{"fix"},
					AgeHours:    6,
				}),
			},
			reviewsByPRNumber: map[int][]*github.PullRequestReview{
				31: {
					mockgithubclient.NewReview("dana", "Dana Davis", "APPROVED"),
					mockgithubclient.NewReview("erin", "Erin Evans", "COMMENTED"),
				},
			},
			timelineCommentsByPRNumber: map[int][]*github.IssueComment{
				32: {
					mockgithubclient.NewTimelineComment("frank", "Frank Foster", "Could you split this into two commits?", now.Add(-1*time.Hour)),
				},
			},
		},
		{
			name: "PR past the old PR threshold",
			configOverrides: map[string]any{
				config.InputOldPRThresholdHours: 12,
			},
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number:      41,
					Title:       "Recent PR below the threshold",
					HTMLURL:     "https://github.com/test-org/test-repo/pull/41",
					AuthorLogin: "alice",
					AuthorName:  "Alice Anderson",
					Labels:      []string{"feature"},
					AgeHours:    3,
				}),
				getTestPR(GetTestPROptions{
					Number:      42,
					Title:       "Old PR past the threshold",
					HTMLURL:     "https://github.com/test-org/test-repo/pull/42",
					AuthorLogin: "bob",
					AuthorName:  "Bob Brown",
					Labels:      []string{"fix"},
					AgeHours:    72,
				}),
			},
		},
		{
			name: "author mapped to a Slack user",
			configOverrides: map[string]any{
				config.InputSlackUserIdByGitHubUsername: map[string]string{"alice": "U2234567890"},
			},
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number:      51,
					Title:       "PR by a mapped author",
					HTMLURL:     "https://github.com/test-org/test-repo/pull/51",
					AuthorLogin: "alice",
					AuthorName:  "Alice Anderson",
					Labels:      []string{"feature"},
					AgeHours:    6,
				}),
				getTestPR(GetTestPROptions{
					Number:      52,
					Title:       "PR by an unmapped author",
					HTMLURL:     "https://github.com/test-org/test-repo/pull/52",
					AuthorLogin: "bob",
					AuthorName:  "Bob Brown",
					Labels:      []string{"fix"},
					AgeHours:    9,
				}),
			},
		},
		{
			name: "every section under load",
			configOverrides: map[string]any{
				config.InputOldPRThresholdHours: 48,
			},
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number: 71, Title: "Approved and ready to go", AuthorLogin: "alice",
					AuthorName: "Alice Anderson", HTMLURL: "https://github.com/test-org/test-repo/pull/71",
					AgeHours: 2,
				}),
				getTestPR(GetTestPROptions{
					Number: 72, Title: "Approved by two reviewers", AuthorLogin: "bob",
					AuthorName: "Bob Brown", HTMLURL: "https://github.com/test-org/test-repo/pull/72",
					AgeHours: 9,
				}),
				getTestPR(GetTestPROptions{
					Number: 73, Title: "Changes were requested here", AuthorLogin: "carol",
					AuthorName: "Carol Clark", HTMLURL: "https://github.com/test-org/test-repo/pull/73",
					AgeHours: 30,
				}),
				getTestPR(GetTestPROptions{
					Number: 74, Title: "A reviewer left a comment", AuthorLogin: "dana",
					AuthorName: "Dana Davis", HTMLURL: "https://github.com/test-org/test-repo/pull/74",
					AgeHours: 5,
				}),
				getTestPR(GetTestPROptions{
					Number: 75, Title: "Nobody has looked at this yet", AuthorLogin: "erin",
					AuthorName: "Erin Evans", HTMLURL: "https://github.com/test-org/test-repo/pull/75",
					AgeHours: 1,
				}),
				getTestPR(GetTestPROptions{
					Number: 76, Title: "Sitting here since last week", AuthorLogin: "frank",
					AuthorName: "Frank Foster", HTMLURL: "https://github.com/test-org/test-repo/pull/76",
					AgeHours: 170,
				}),
			},
			// Four merges reach the cap of three, newest first.
			mergedPRs: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number: 81, Title: "Landed this morning", AuthorLogin: "alice",
					AuthorName: "Alice Anderson", HTMLURL: "https://github.com/test-org/test-repo/pull/81",
					AgeHours: 20, State: "closed", Merged: true, MergedHoursAgo: 3,
				}),
				getTestPR(GetTestPROptions{
					Number: 82, Title: "Landed yesterday", AuthorLogin: "bob",
					AuthorName: "Bob Brown", HTMLURL: "https://github.com/test-org/test-repo/pull/82",
					AgeHours: 40, State: "closed", Merged: true, MergedHoursAgo: 26,
				}),
				getTestPR(GetTestPROptions{
					Number: 83, Title: "Landed an hour ago", AuthorLogin: "carol",
					AuthorName: "Carol Clark", HTMLURL: "https://github.com/test-org/test-repo/pull/83",
					AgeHours: 12, State: "closed", Merged: true, MergedHoursAgo: 1,
				}),
				getTestPR(GetTestPROptions{
					Number: 84, Title: "Landed three days ago", AuthorLogin: "dana",
					AuthorName: "Dana Davis", HTMLURL: "https://github.com/test-org/test-repo/pull/84",
					AgeHours: 100, State: "closed", Merged: true, MergedHoursAgo: 74,
				}),
			},
			reviewsByPRNumber: map[int][]*github.PullRequestReview{
				71: {mockgithubclient.NewReview("dana", "Dana Davis", "APPROVED")},
				72: {
					mockgithubclient.NewReview("dana", "Dana Davis", "APPROVED"),
					mockgithubclient.NewReview("erin", "Erin Evans", "APPROVED"),
				},
				73: {mockgithubclient.NewReview("dana", "Dana Davis", "CHANGES_REQUESTED")},
				74: {mockgithubclient.NewReview("erin", "Erin Evans", "COMMENTED")},
				// Only one of the merged PRs was reviewed, so the snapshot keeps a merged row
				// with reviewers apart from one without.
				83: {
					mockgithubclient.NewReview("dana", "Dana Davis", "APPROVED"),
					mockgithubclient.NewReview("erin", "Erin Evans", "COMMENTED"),
				},
			},
		},
		{
			name: "one open PR and one merged PR",
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number: 91, Title: "The only open PR", AuthorLogin: "alice",
					AuthorName: "Alice Anderson", HTMLURL: "https://github.com/test-org/test-repo/pull/91",
					AgeHours: 4,
				}),
			},
			mergedPRs: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number: 92, Title: "The only merged PR", AuthorLogin: "bob",
					AuthorName: "Bob Brown", HTMLURL: "https://github.com/test-org/test-repo/pull/92",
					AgeHours: 20, State: "closed", Merged: true, MergedHoursAgo: 5,
				}),
			},
		},
		{
			name: "no open PRs and no no-prs-message, but a merged one",
			mergedPRs: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number: 93, Title: "Merged with nothing left open", AuthorLogin: "alice",
					AuthorName: "Alice Anderson", HTMLURL: "https://github.com/test-org/test-repo/pull/93",
					AgeHours: 20, State: "closed", Merged: true, MergedHoursAgo: 5,
				}),
			},
		},
		{
			name: "no PRs message",
			configOverrides: map[string]any{
				config.InputNoPRsMessage: "No open PRs, happy coding! 🎉",
			},
		},
	}
}

func runSnapshotScenario(
	t *testing.T,
	scenario snapshotScenario,
	previousState *state.State,
	postedMessageTimestamp string,
) *mockslackclient.MockSlackAPI {
	t.Helper()
	getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
		PRs:                        scenario.prs,
		PRsByRepo:                  scenario.prsByRepo,
		MergedPRs:                  scenario.mergedPRs,
		ReviewsByPRNumber:          scenario.reviewsByPRNumber,
		TimelineCommentsByPRNumber: scenario.timelineCommentsByPRNumber,
		MockPreviousState:          previousState,
	})
	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{
		PostMessageTimestamp: postedMessageTimestamp,
	})

	if err := main.Run(getGitHubClient, mockslackclient.MakeSlackClientGetter(mockSlackAPI)); err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	return mockSlackAPI
}

func TestSnapshotsPostMode(t *testing.T) {
	for _, scenario := range snapshotScenarios() {
		t.Run(scenario.name, func(t *testing.T) {
			overrides, sentSlackBlocksFilePath := getFilePathOverrides(t)
			maps.Copy(overrides, scenario.configOverrides)
			testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &overrides)

			runSnapshotScenario(t, scenario, nil, "")

			assertSentBlocksMatchSnapshot(t, sentSlackBlocksFilePath)
			assertSavedStateMatchesSnapshot(t, overrides[config.EnvStateFilePath].(string))
		})
	}
}

func TestSnapshotsPreviousMessageMarkedStale(t *testing.T) {
	testCases := []struct {
		name              string
		scenarioName      string
		editedByUpdateRun bool
	}{
		{name: "every section under load", scenarioName: "every section under load"},
		{name: "grouped by repository over two repositories", scenarioName: "grouped by repository over two repositories"},
		{
			name:              "every section under load, edited by an update run",
			scenarioName:      "every section under load",
			editedByUpdateRun: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scenario, found := utilities.Find(snapshotScenarios(), func(scenario snapshotScenario) bool {
				return scenario.name == tc.scenarioName
			})
			if !found {
				t.Fatalf("No snapshot scenario named %q", tc.scenarioName)
			}
			overrides, _ := getFilePathOverrides(t)
			maps.Copy(overrides, scenario.configOverrides)
			stateFilePath := overrides[config.EnvStateFilePath].(string)
			testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &overrides)

			runSnapshotScenario(t, scenario, nil, "1234567890.123456")
			previousState := loadSavedState(t, stateFilePath)
			if tc.editedByUpdateRun {
				updateOverrides := maps.Clone(overrides)
				updateOverrides[config.InputRunMode] = config.RunModeUpdate
				testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &updateOverrides)
				runSnapshotScenario(t, scenario, &previousState, "")
				previousState = loadSavedState(t, stateFilePath)
				testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &overrides)
			}
			mockSlackAPI := runSnapshotScenario(t, scenario, &previousState, "1234567899.000200")

			markAsStaleEdit := mockSlackAPI.UpdatedMessage
			if markAsStaleEdit.ChannelID != "C12345678" || markAsStaleEdit.Timestamp != "1234567890.123456" {
				t.Fatalf(
					"Expected the mark-as-stale edit on the first post's message, C12345678 at 1234567890.123456, got %s at %s",
					markAsStaleEdit.ChannelID, markAsStaleEdit.Timestamp,
				)
			}
			var indentedMarkedMessageBlocks bytes.Buffer
			if err := json.Indent(&indentedMarkedMessageBlocks, markAsStaleEdit.BlocksAsSent, "", "  "); err != nil {
				t.Fatalf("Failed to indent the marked message blocks: %v", err)
			}
			assertBlocksMatchSnapshot(t, indentedMarkedMessageBlocks.Bytes())
			assertSavedStateMatchesSnapshot(t, stateFilePath)
		})
	}
}

func TestSnapshotsUpdateMode(t *testing.T) {
	testCases := []struct {
		name            string
		configOverrides map[string]any
		statePRNumbers  []int
		prByNumber      map[int]*github.PullRequest
		// The open-PR fetch is live, so it returns the state PRs that are still open, by number,
		// and any PR opened since the message was posted.
		openPRNumbers       []int
		openPRsNotInState   []*github.PullRequest
		mergedPRsFromSearch []*github.PullRequest
		reviewsByPRNumber   map[int][]*github.PullRequestReview
		deletesTheMessage   bool
	}{
		{
			name: "every section under load",
			configOverrides: map[string]any{
				config.InputOldPRThresholdHours: 48,
			},
			statePRNumbers: []int{61, 62, 63, 64, 65, 66, 67, 68},
			prByNumber: map[int]*github.PullRequest{
				61: getTestPR(GetTestPROptions{
					Number: 61, Title: "Approved and ready to go", AuthorLogin: "alice",
					AuthorName: "Alice Anderson", HTMLURL: "https://github.com/test-org/test-repo/pull/61",
					AgeHours: 2, State: "open",
				}),
				62: getTestPR(GetTestPROptions{
					Number: 62, Title: "Changes were requested here", AuthorLogin: "bob",
					AuthorName: "Bob Brown", HTMLURL: "https://github.com/test-org/test-repo/pull/62",
					AgeHours: 30, State: "open",
				}),
				63: getTestPR(GetTestPROptions{
					Number: 63, Title: "Nobody has looked at this yet", AuthorLogin: "carol",
					AuthorName: "Carol Clark", HTMLURL: "https://github.com/test-org/test-repo/pull/63",
					AgeHours: 1, State: "open",
				}),
				64: getTestPR(GetTestPROptions{
					Number: 64, Title: "Sitting here since last week", AuthorLogin: "dana",
					AuthorName: "Dana Davis", HTMLURL: "https://github.com/test-org/test-repo/pull/64",
					AgeHours: 170, State: "open",
				}),
				65: getTestPR(GetTestPROptions{
					Number: 65, Title: "A reviewer left a comment", AuthorLogin: "erin",
					AuthorName: "Erin Evans", HTMLURL: "https://github.com/test-org/test-repo/pull/65",
					AgeHours: 6, State: "open",
				}),
				66: getTestPR(GetTestPROptions{
					Number: 66, Title: "Landed an hour ago", AuthorLogin: "frank",
					AuthorName: "Frank Foster", HTMLURL: "https://github.com/test-org/test-repo/pull/66",
					AgeHours: 12, State: "closed", Merged: true, MergedHoursAgo: 1,
				}),
				67: getTestPR(GetTestPROptions{
					Number: 67, Title: "Landed three days ago", AuthorLogin: "alice",
					AuthorName: "Alice Anderson", HTMLURL: "https://github.com/test-org/test-repo/pull/67",
					AgeHours: 100, State: "closed", Merged: true, MergedHoursAgo: 74,
				}),
				68: getTestPR(GetTestPROptions{
					Number: 68, Title: "Closed without merging", AuthorLogin: "bob",
					AuthorName: "Bob Brown", HTMLURL: "https://github.com/test-org/test-repo/pull/68",
					AgeHours: 50, State: "closed", Merged: false,
				}),
			},
			openPRNumbers: []int{61, 62, 63, 64, 65},
			openPRsNotInState: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number: 69, Title: "Opened after the message was posted", AuthorLogin: "grace",
					AuthorName: "Grace Green", HTMLURL: "https://github.com/test-org/test-repo/pull/69",
					AgeHours: 3, State: "open",
				}),
			},
			mergedPRsFromSearch: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number: 70, Title: "Merged without ever being listed", AuthorLogin: "heidi",
					AuthorName: "Heidi Hill", HTMLURL: "https://github.com/test-org/test-repo/pull/70",
					AgeHours: 40, State: "closed", Merged: true, MergedHoursAgo: 4,
				}),
			},
			reviewsByPRNumber: map[int][]*github.PullRequestReview{
				61: {mockgithubclient.NewReview("dana", "Dana Davis", "APPROVED")},
				62: {mockgithubclient.NewReview("dana", "Dana Davis", "CHANGES_REQUESTED")},
				65: {mockgithubclient.NewReview("frank", "Frank Foster", "COMMENTED")},
				66: {mockgithubclient.NewReview("erin", "Erin Evans", "APPROVED")},
			},
		},
		{
			name:           "one open PR and one merged PR",
			statePRNumbers: []int{71, 72},
			openPRNumbers:  []int{71},
			prByNumber: map[int]*github.PullRequest{
				71: getTestPR(GetTestPROptions{
					Number: 71, Title: "The only open PR", AuthorLogin: "alice",
					AuthorName: "Alice Anderson", HTMLURL: "https://github.com/test-org/test-repo/pull/71",
					AgeHours: 4, State: "open",
				}),
				72: getTestPR(GetTestPROptions{
					Number: 72, Title: "The only merged PR", AuthorLogin: "bob",
					AuthorName: "Bob Brown", HTMLURL: "https://github.com/test-org/test-repo/pull/72",
					AgeHours: 20, State: "closed", Merged: true, MergedHoursAgo: 5,
				}),
			},
		},
		{
			name: "no open PRs left with the no-prs-message set",
			configOverrides: map[string]any{
				config.InputNoPRsMessage: "No open PRs, happy coding! 🎉",
			},
			statePRNumbers: []int{81, 82},
			prByNumber: map[int]*github.PullRequest{
				81: getTestPR(GetTestPROptions{
					Number: 81, Title: "Merged since the message was posted", AuthorLogin: "alice",
					AuthorName: "Alice Anderson", HTMLURL: "https://github.com/test-org/test-repo/pull/81",
					AgeHours: 20, State: "closed", Merged: true, MergedHoursAgo: 2,
				}),
				82: getTestPR(GetTestPROptions{
					Number: 82, Title: "Closed without merging", AuthorLogin: "bob",
					AuthorName: "Bob Brown", HTMLURL: "https://github.com/test-org/test-repo/pull/82",
					AgeHours: 30, State: "closed", Merged: false,
				}),
			},
		},
		{
			name:           "nothing left to show deletes a message saved before LastWrittenMessage existed",
			statePRNumbers: []int{91},
			prByNumber: map[int]*github.PullRequest{
				91: getTestPR(GetTestPROptions{
					Number: 91, Title: "Closed without merging", AuthorLogin: "alice",
					AuthorName: "Alice Anderson", HTMLURL: "https://github.com/test-org/test-repo/pull/91",
					AgeHours: 30, State: "closed", Merged: false,
				}),
			},
			deletesTheMessage: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			overrides, sentSlackBlocksFilePath := getFilePathOverrides(t)
			overrides[config.InputRunMode] = config.RunModeUpdate
			maps.Copy(overrides, tc.configOverrides)
			testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &overrides)

			mockState := getTestState(GetTestStateOptions{PRNumbers: tc.statePRNumbers})
			getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
				PRsByNumber:       tc.prByNumber,
				PRs:               openPRsOfFetch(tc.prByNumber, tc.openPRNumbers, tc.openPRsNotInState),
				MergedPRs:         tc.mergedPRsFromSearch,
				ReviewsByPRNumber: tc.reviewsByPRNumber,
				MockPreviousState: &mockState,
			})
			mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})

			if err := main.Run(getGitHubClient, mockslackclient.MakeSlackClientGetter(mockSlackAPI)); err != nil {
				t.Fatalf("Expected Run to succeed, got error: %v", err)
			}

			if tc.deletesTheMessage {
				if mockSlackAPI.DeletedMessage.Timestamp != "1623850245.000200" {
					t.Fatalf("Expected the loaded message deleted, got %+v", mockSlackAPI.DeletedMessage)
				}
			} else {
				assertSentBlocksMatchSnapshot(t, sentSlackBlocksFilePath)
			}
			assertSavedStateMatchesSnapshot(t, overrides[config.EnvStateFilePath].(string))
		})
	}
}
