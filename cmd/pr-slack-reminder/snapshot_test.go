package main_test

import (
	"bytes"
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

var footerTimestamp = regexp.MustCompile(`!date\^\d+\^\{time\}\|\d{2}:\d{2} UTC`)

// Run stamps the footer with the real clock, so a snapshot recorded a second ago would never
// match again. The rest of the message survives a moving clock: the fixture ages come off this
// package's own `now`.
func withFixedFooterTimestamp(blocks []byte) []byte {
	return footerTimestamp.ReplaceAll(blocks, []byte(`!date^0^{time}|00:00 UTC`))
}

func getSnapshotFilePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(snapshotDirectory, nonAlphanumericRuns.ReplaceAllString(t.Name(), "-")+".json")
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
	sentBlocks = withFixedFooterTimestamp(sentBlocks)

	snapshotFilePath := getSnapshotFilePath(t)
	if *updateSnapshots {
		if err := os.MkdirAll(snapshotDirectory, 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", snapshotDirectory, err)
		}
		if err := os.WriteFile(snapshotFilePath, sentBlocks, 0644); err != nil {
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
	if !bytes.Equal(snapshot, sentBlocks) {
		t.Errorf(
			"Sent Slack blocks do not match snapshot %s.\nSnapshot:\n%s\n\nSent:\n%s",
			snapshotFilePath, snapshot, sentBlocks,
		)
	}
}

func TestSnapshotsPostMode(t *testing.T) {
	testCases := []struct {
		name                       string
		configOverrides            map[string]any
		prs                        []*github.PullRequest
		prsByRepo                  map[string][]*github.PullRequest
		mergedPRs                  []*github.PullRequest
		reviewsByPRNumber          map[int][]*github.PullRequestReview
		timelineCommentsByPRNumber map[int][]*github.IssueComment
	}{
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			overrides, sentSlackBlocksFilePath := getFilePathOverrides(t)
			maps.Copy(overrides, tc.configOverrides)
			testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &overrides)

			getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
				PRs:                        tc.prs,
				PRsByRepo:                  tc.prsByRepo,
				MergedPRs:                  tc.mergedPRs,
				ReviewsByPRNumber:          tc.reviewsByPRNumber,
				TimelineCommentsByPRNumber: tc.timelineCommentsByPRNumber,
			})
			mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})

			if err := main.Run(getGitHubClient, mockslackclient.MakeSlackClientGetter(mockSlackAPI)); err != nil {
				t.Fatalf("Expected Run to succeed, got error: %v", err)
			}

			assertSentBlocksMatchSnapshot(t, sentSlackBlocksFilePath)
		})
	}
}

func TestSnapshotsUpdateMode(t *testing.T) {
	testCases := []struct {
		name              string
		configOverrides   map[string]any
		statePRNumbers    []int
		prByNumber        map[int]*github.PullRequest
		reviewsByPRNumber map[int][]*github.PullRequestReview
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			overrides, sentSlackBlocksFilePath := getFilePathOverrides(t)
			overrides[config.InputRunMode] = config.RunModeUpdate
			maps.Copy(overrides, tc.configOverrides)
			testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &overrides)

			mockState := getTestState(GetTestStateOptions{PRNumbers: tc.statePRNumbers})
			getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
				PRsByNumber:            tc.prByNumber,
				ReviewsByPRNumber:      tc.reviewsByPRNumber,
				MockStateForUpdateMode: &mockState,
			})
			mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})

			if err := main.Run(getGitHubClient, mockslackclient.MakeSlackClientGetter(mockSlackAPI)); err != nil {
				t.Fatalf("Expected Run to succeed, got error: %v", err)
			}

			assertSentBlocksMatchSnapshot(t, sentSlackBlocksFilePath)
		})
	}
}
