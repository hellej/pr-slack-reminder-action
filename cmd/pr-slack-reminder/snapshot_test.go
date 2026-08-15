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
					mockgithubclient.NewReview(1, "APPROVED", "dana", "Dana Davis", "LGTM 🚀"),
					mockgithubclient.NewReview(2, "COMMENTED", "erin", "Erin Evans", "One question..."),
				},
			},
			timelineCommentsByPRNumber: map[int][]*github.IssueComment{
				32: {
					{
						Body:      github.Ptr("Could you split this into two commits?"),
						CreatedAt: &github.Timestamp{Time: now.Add(-1 * time.Hour)},
						User: &github.User{
							Login: github.Ptr("frank"),
							Name:  github.Ptr("Frank Foster"),
						},
					},
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

func TestSnapshotUpdateModeWithOpenMergedAndClosedPRs(t *testing.T) {
	overrides, sentSlackBlocksFilePath := getFilePathOverrides(t)
	overrides[config.InputRunMode] = config.RunModeUpdate
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &overrides)

	prByNumber := map[int]*github.PullRequest{
		61: getTestPR(GetTestPROptions{
			Number:      61,
			Title:       "Still open PR",
			HTMLURL:     "https://github.com/test-org/test-repo/pull/61",
			AuthorLogin: "alice",
			AuthorName:  "Alice Anderson",
			Labels:      []string{"feature"},
			AgeHours:    4,
			State:       "open",
		}),
		62: getTestPR(GetTestPROptions{
			Number:      62,
			Title:       "Merged PR",
			HTMLURL:     "https://github.com/test-org/test-repo/pull/62",
			AuthorLogin: "bob",
			AuthorName:  "Bob Brown",
			Labels:      []string{"fix"},
			AgeHours:    7,
			State:       "closed",
			Merged:      true,
		}),
		63: getTestPR(GetTestPROptions{
			Number:      63,
			Title:       "Closed PR without merge",
			HTMLURL:     "https://github.com/test-org/test-repo/pull/63",
			AuthorLogin: "carol",
			AuthorName:  "Carol Clark",
			Labels:      []string{"chore"},
			AgeHours:    10,
			State:       "closed",
			Merged:      false,
		}),
	}

	mockState := getTestState(GetTestStateOptions{PRNumbers: []int{61, 62, 63}})
	getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
		PRsByNumber:            prByNumber,
		MockStateForUpdateMode: &mockState,
	})
	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})

	if err := main.Run(getGitHubClient, mockslackclient.MakeSlackClientGetter(mockSlackAPI)); err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}

	assertSentBlocksMatchSnapshot(t, sentSlackBlocksFilePath)
}
