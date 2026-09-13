package canvasbuilder_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/canvasbuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/canvascontent"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

var updateSnapshots = flag.Bool(
	"update-snapshots", false, "record the rendered canvas markdown as snapshot files instead of comparing against them",
)

const snapshotDirectory = "testdata"

var nonAlphanumericRuns = regexp.MustCompile(`[^a-zA-Z0-9]+`)

var generatedAt = time.Date(2026, 8, 8, 6, 15, 0, 0, time.UTC)

// Offsets stay clear of every boundary the age, activity and idle texts round or threshold on,
// so the rendered text can't flip while the test runs.
const (
	minutesAge = 30 * time.Minute
	hoursAge   = 5 * time.Hour
	idleAge    = 3 * 24 * time.Hour
	oldAge     = 10 * 24 * time.Hour
)

type prOptions struct {
	number      int
	title       string
	repository  string
	authorName  string
	approvers   []string
	commenters  []string
	isOldPR     bool
	age         time.Duration
	activityAge *time.Duration
	mergeAge    *time.Duration
}

func testPR(options prOptions) prview.PR {
	repository := models.Repository{Owner: "test-org", Name: "test-repo"}
	if options.repository != "" {
		repository = models.Repository{Owner: "test-org", Name: options.repository}
	}
	var updatedAt time.Time
	if options.activityAge != nil {
		updatedAt = time.Now().Add(-*options.activityAge)
	}
	var mergedAt *time.Time
	if options.mergeAge != nil {
		timestamp := time.Now().Add(-*options.mergeAge)
		mergedAt = &timestamp
	}
	return prview.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{
				Number:    options.number,
				Title:     options.title,
				HTMLURL:   fmt.Sprintf("https://github.com/%s/pull/%d", repository.GetPath(), options.number),
				CreatedAt: time.Now().Add(-options.age),
				UpdatedAt: updatedAt,
				MergedAt:  mergedAt,
			},
			Repository: repository,
		},
		Author:     testCollaborator(options.authorName),
		Approvers:  utilities.Map(options.approvers, testCollaborator),
		Commenters: utilities.Map(options.commenters, testCollaborator),
		IsOldPR:    options.isOldPR,
	}
}

func testCollaborator(name string) prview.Collaborator {
	return prview.NewCollaborator(githubclient.Collaborator{Login: "login", Name: name}, "U1234567890")
}

func durationPointer(duration time.Duration) *time.Duration {
	return &duration
}

func TestBuildMarkdownSnapshots(t *testing.T) {
	openPR := testPR(prOptions{
		number: 1, title: "Add pagination to the PR listing", authorName: "Alice Anderson",
		age: hoursAge, approvers: []string{"Dana Davis"}, commenters: []string{"Erin Evans"},
	})
	otherOpenPR := testPR(prOptions{
		number: 2, title: "Bump the Slack SDK", repository: "repo-two", authorName: "Bob Brown",
		age: minutesAge,
	})
	wipPR := testPR(prOptions{
		number: 3, title: "Spike: replace mux with chi", authorName: "Carol Clark",
		age: hoursAge, activityAge: durationPointer(hoursAge),
	})
	mergedPR := testPR(prOptions{
		number: 9, title: "Drop the REST fallback", authorName: "Alice Anderson",
		age: oldAge, mergeAge: durationPointer(idleAge),
		approvers: []string{"Dana Davis"}, commenters: []string{"Erin Evans"},
	})
	otherWIPPR := testPR(prOptions{
		number: 11, title: "Draft the migration guide", repository: "repo-two",
		authorName: "Bob Brown", age: hoursAge, activityAge: durationPointer(minutesAge),
	})
	otherMergedPR := testPR(prOptions{
		number: 2, title: "Bump the Slack SDK", repository: "repo-two", authorName: "Bob Brown",
		age: idleAge, mergeAge: durationPointer(hoursAge),
	})

	// Every bucket is oldest first, as canvascontent hands them over.
	readyToMergePRs := []prview.PR{
		testPR(prOptions{
			number: 23, title: "Drop the deprecated input", repository: "repo-three",
			authorName: "Carol Clark", age: idleAge,
			approvers: []string{"Frank Foster"}, commenters: []string{"Erin Evans"},
		}),
		testPR(prOptions{
			number: 21, title: "Cache the repository lookups", authorName: "Alice Anderson",
			age: hoursAge, approvers: []string{"Dana Davis"},
		}),
		testPR(prOptions{
			number: 24, title: "Pin the action digests", authorName: "Dana Davis", age: hoursAge,
			approvers:  []string{"Alice Anderson", "Bob Brown"},
			commenters: []string{"Carol Clark", "Erin Evans"},
		}),
		testPR(prOptions{
			number: 22, title: "Retry the artifact download", repository: "repo-two",
			authorName: "Bob Brown", age: minutesAge,
			approvers: []string{"Dana Davis", "Erin Evans"},
		}),
	}
	waitingForAuthorPRs := []prview.PR{
		testPR(prOptions{
			number: 31, title: "Rework the snooze parser", authorName: "Bob Brown",
			age: oldAge, isOldPR: true, commenters: []string{"Dana Davis"},
		}),
		testPR(prOptions{
			number: 32, title: "Split the config package", repository: "repo-two",
			authorName: "Carol Clark", age: idleAge,
			commenters: []string{"Alice Anderson", "Erin Evans"},
		}),
		testPR(prOptions{
			number: 33, title: "Handle the 404 on missing repositories", repository: "repo-three",
			authorName: "Dana Davis", age: hoursAge,
			approvers: []string{"Alice Anderson"}, commenters: []string{"Bob Brown"},
		}),
		testPR(prOptions{
			number: 34, title: "Escape the canvas titles", authorName: "Erin Evans",
			age: minutesAge, commenters: []string{"Frank Foster"},
		}),
	}
	waitingForReviewPRs := []prview.PR{
		testPR(prOptions{
			number: 44, title: "Trim the trailing newline", authorName: "Dana Davis",
			age: oldAge, isOldPR: true,
		}),
		testPR(prOptions{
			number: 43, title: "Log the rate limit cost", repository: "repo-three",
			authorName: "Carol Clark", age: idleAge,
		}),
		testPR(prOptions{
			number: 41, title: "Add the dry-run flag", authorName: "Alice Anderson", age: hoursAge,
		}),
		testPR(prOptions{
			number: 45, title: "Bump golangci-lint", repository: "repo-two",
			authorName: "Erin Evans", age: hoursAge,
		}),
		testPR(prOptions{
			number: 42, title: "Document the canvas scopes", repository: "repo-two",
			authorName: "Bob Brown", age: minutesAge,
		}),
		testPR(prOptions{
			number: 46, title: "Collapse the duplicate filters", repository: "repo-three",
			authorName: "Frank Foster", age: minutesAge,
		}),
	}

	testCases := []struct {
		name    string
		content canvascontent.Content
	}{
		{
			// A whole canvas at a real day's volume, so the golden file shows what three
			// open headings cost a reader.
			name: "flat open PRs and WIP PRs",
			content: canvascontent.Content{
				ReadyToMerge:     canvascontent.PRSection{PRs: readyToMergePRs},
				WaitingForAuthor: canvascontent.PRSection{PRs: waitingForAuthorPRs},
				WaitingForReview: canvascontent.PRSection{PRs: waitingForReviewPRs},
				WIP:              canvascontent.PRSection{PRs: []prview.PR{wipPR, otherWIPPR}},
				Merged:           canvascontent.PRSection{PRs: []prview.PR{otherMergedPR, mergedPR}},
				GeneratedAt:      generatedAt,
			},
		},
		{
			name: "merged PRs could not be fetched",
			content: canvascontent.Content{
				WaitingForReview:     canvascontent.PRSection{PRs: []prview.PR{openPR}},
				WIP:                  canvascontent.PRSection{PRs: []prview.PR{wipPR}},
				MergedPRsUnavailable: true,
				GeneratedAt:          generatedAt,
			},
		},
		{
			name: "open PRs grouped by repository",
			content: canvascontent.Content{
				WaitingForReview: canvascontent.PRSection{
					Groups: prview.GroupPRsByRepositories([]prview.PR{openPR, otherOpenPR}),
				},
				GroupedByRepository: true,
				WIP:                 canvascontent.PRSection{PRs: []prview.PR{wipPR}},
				GeneratedAt:         generatedAt,
			},
		},
		{
			// Each section's groups come in their own order, so the repository leading one
			// section need not lead the next, and nothing dedupes a repository across
			// sections: the golden file shows what the ### headings add up to.
			name: "all sections grouped by repository",
			content: canvascontent.Content{
				ReadyToMerge: canvascontent.PRSection{
					Groups: prview.GroupPRsByRepositoriesInGivenOrder(readyToMergePRs),
				},
				WaitingForAuthor: canvascontent.PRSection{
					Groups: prview.GroupPRsByRepositoriesInGivenOrder(waitingForAuthorPRs),
				},
				WaitingForReview: canvascontent.PRSection{
					Groups: prview.GroupPRsByRepositoriesInGivenOrder(waitingForReviewPRs),
				},
				WIP: canvascontent.PRSection{
					Groups: prview.GroupPRsByRepositoriesInGivenOrder([]prview.PR{otherWIPPR, wipPR}),
				},
				Merged: canvascontent.PRSection{
					Groups: prview.GroupPRsByRepositoriesInGivenOrder([]prview.PR{otherMergedPR, mergedPR}),
				},
				GroupedByRepository: true,
				GeneratedAt:         generatedAt,
			},
		},
		{
			// The two empty buckets render nothing at all, heading included.
			name: "ready to merge PRs only",
			content: canvascontent.Content{
				ReadyToMerge: canvascontent.PRSection{PRs: readyToMergePRs},
				WIP:          canvascontent.PRSection{PRs: []prview.PR{wipPR}},
				GeneratedAt:  generatedAt,
			},
		},
		{
			name: "nothing ready to merge",
			content: canvascontent.Content{
				WaitingForAuthor: canvascontent.PRSection{PRs: waitingForAuthorPRs},
				WaitingForReview: canvascontent.PRSection{PRs: waitingForReviewPRs},
				WIP:              canvascontent.PRSection{PRs: []prview.PR{wipPR}},
				GeneratedAt:      generatedAt,
			},
		},
		{
			// A hidden bucket while grouping: only the populated section gets its ###
			// sub-headings.
			name: "ready to merge PRs only, grouped by repository",
			content: canvascontent.Content{
				ReadyToMerge: canvascontent.PRSection{
					Groups: prview.GroupPRsByRepositoriesInGivenOrder(readyToMergePRs),
				},
				WIP: canvascontent.PRSection{
					Groups: prview.GroupPRsByRepositoriesInGivenOrder([]prview.PR{wipPR}),
				},
				GroupedByRepository: true,
				GeneratedAt:         generatedAt,
			},
		},
		{
			name: "no open PRs",
			content: canvascontent.Content{
				WIP:         canvascontent.PRSection{PRs: []prview.PR{wipPR}},
				GeneratedAt: generatedAt,
			},
		},
		{
			name: "no open PRs while grouping by repository",
			content: canvascontent.Content{
				GroupedByRepository: true,
				WIP:                 canvascontent.PRSection{PRs: []prview.PR{wipPR}},
				GeneratedAt:         generatedAt,
			},
		},
		{
			name: "no WIP PRs",
			content: canvascontent.Content{
				WaitingForReview: canvascontent.PRSection{PRs: []prview.PR{openPR}},
				GeneratedAt:      generatedAt,
			},
		},
		{
			name:    "no PRs at all",
			content: canvascontent.Content{GeneratedAt: generatedAt},
		},
		{
			name: "old open PR",
			content: canvascontent.Content{
				WaitingForReview: canvascontent.PRSection{
					PRs: []prview.PR{
						testPR(prOptions{
							number: 4, title: "Old PR past the threshold", authorName: "Bob Brown",
							age: oldAge, isOldPR: true,
						}),
						openPR,
					},
				},
				GeneratedAt: generatedAt,
			},
		},
		{
			name: "recently updated and idle WIP PRs",
			content: canvascontent.Content{
				WIP: canvascontent.PRSection{
					PRs: []prview.PR{
						wipPR,
						testPR(prOptions{
							number: 5, title: "Refactor state store", authorName: "Carol Clark",
							age: oldAge, activityAge: durationPointer(idleAge),
							approvers: []string{"Dana Davis"}, commenters: []string{"Erin Evans"},
						}),
					},
				},
				GeneratedAt: generatedAt,
			},
		},
		{
			name: "WIP PR with unknown activity",
			content: canvascontent.Content{
				WIP: canvascontent.PRSection{
					PRs: []prview.PR{
						testPR(prOptions{
							number: 6, title: "Prototype canvas rendering", authorName: "Alice Anderson",
							age: idleAge,
						}),
					},
				},
				GeneratedAt: generatedAt,
			},
		},
		{
			name: "merged PR with unknown merge time",
			content: canvascontent.Content{
				Merged: canvascontent.PRSection{
					PRs: []prview.PR{
						testPR(prOptions{
							number: 10, title: "Restore the deleted branch", authorName: "Carol Clark",
							age: oldAge,
						}),
					},
				},
				GeneratedAt: generatedAt,
			},
		},
		{
			name: "open PRs capped",
			content: canvascontent.Content{
				WaitingForReview: canvascontent.PRSection{PRs: []prview.PR{openPR}},
				WIP:              canvascontent.PRSection{PRs: []prview.PR{wipPR}},
				OpenPRsCapped:    true,
				GeneratedAt:      generatedAt,
			},
		},
		{
			name: "WIP PRs capped",
			content: canvascontent.Content{
				WaitingForReview: canvascontent.PRSection{PRs: []prview.PR{openPR}},
				WIP:              canvascontent.PRSection{PRs: []prview.PR{wipPR}},
				WIPPRsCapped:     true,
				GeneratedAt:      generatedAt,
			},
		},
		{
			name: "both sections capped",
			content: canvascontent.Content{
				WaitingForReview: canvascontent.PRSection{PRs: []prview.PR{openPR}},
				WIP:              canvascontent.PRSection{PRs: []prview.PR{wipPR}},
				OpenPRsCapped:    true,
				WIPPRsCapped:     true,
				GeneratedAt:      generatedAt,
			},
		},
		{
			name: "generated at a non-UTC time",
			content: canvascontent.Content{
				WaitingForReview: canvascontent.PRSection{PRs: []prview.PR{openPR}},
				WIP:              canvascontent.PRSection{PRs: []prview.PR{wipPR}},
				// The same moment as every other case's generatedAt, three hours east of UTC.
				GeneratedAt: generatedAt.In(time.FixedZone("EEST", 3*60*60)),
			},
		},
		{
			name: "markdown characters in titles and names",
			content: canvascontent.Content{
				WaitingForReview: canvascontent.PRSection{
					Groups: prview.GroupPRsByRepositories([]prview.PR{
						testPR(prOptions{
							number:     7,
							title:      "Fix [ABC-123] crash in `make test` & **WIP** _debug_ ~legacy~ C:\\path <b>",
							repository: "repo_two",
							authorName: "Al*ice_And[erson]",
							commenters: []string{"E~rin <Evans>"},
							age:        hoursAge,
						}),
					}),
				},
				GroupedByRepository: true,
				WIP: canvascontent.PRSection{
					PRs: []prview.PR{
						testPR(prOptions{
							number:      8,
							title:       "Draft: &amp; <https://example.com> \\_escaped_",
							authorName:  "B`ob & Brown",
							age:         hoursAge,
							activityAge: durationPointer(minutesAge),
						}),
					},
				},
				GeneratedAt: generatedAt,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assertMarkdownMatchesSnapshot(t, canvasbuilder.BuildMarkdown(tc.content))
		})
	}
}

func assertMarkdownMatchesSnapshot(t *testing.T, markdown string) {
	t.Helper()
	snapshotFilePath := filepath.Join(
		snapshotDirectory, nonAlphanumericRuns.ReplaceAllString(t.Name(), "-")+".md",
	)

	if *updateSnapshots {
		if err := os.MkdirAll(snapshotDirectory, 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", snapshotDirectory, err)
		}
		if err := os.WriteFile(snapshotFilePath, []byte(markdown), 0644); err != nil {
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
	if !bytes.Equal(snapshot, []byte(markdown)) {
		t.Errorf(
			"Rendered canvas markdown does not match snapshot %s.\nSnapshot:\n%s\n\nRendered:\n%s",
			snapshotFilePath, snapshot, markdown,
		)
	}
}

// A snapshot would accept a heading appearing or vanishing on a re-record, so the presence of
// each open heading is asserted on its own.
func TestBuildMarkdownHidesEmptyOpenSections(t *testing.T) {
	pr := testPR(prOptions{
		number: 1, title: "Add pagination to the PR listing", authorName: "Alice Anderson",
		age: hoursAge,
	})
	filled := canvascontent.PRSection{PRs: []prview.PR{pr}}

	testCases := []struct {
		name             string
		content          canvascontent.Content
		expectedHeadings []string
	}{
		{
			name:             "only ready to merge",
			content:          canvascontent.Content{ReadyToMerge: filled},
			expectedHeadings: []string{"## Ready to merge"},
		},
		{
			name:             "only waiting for author",
			content:          canvascontent.Content{WaitingForAuthor: filled},
			expectedHeadings: []string{"## Waiting for author"},
		},
		{
			name:             "only waiting for review",
			content:          canvascontent.Content{WaitingForReview: filled},
			expectedHeadings: []string{"## Waiting for review"},
		},
		{
			name: "ready to merge and waiting for review",
			content: canvascontent.Content{
				ReadyToMerge: filled, WaitingForReview: filled,
			},
			expectedHeadings: []string{"## Ready to merge", "## Waiting for review"},
		},
		{
			name: "all three",
			content: canvascontent.Content{
				ReadyToMerge: filled, WaitingForAuthor: filled, WaitingForReview: filled,
			},
			expectedHeadings: []string{
				"## Ready to merge", "## Waiting for author", "## Waiting for review",
			},
		},
		{
			// Nothing open at all keeps one heading, so the canvas does not open at ## WIP.
			name:             "nothing open",
			content:          canvascontent.Content{},
			expectedHeadings: []string{"## Open"},
		},
		{
			name: "nothing open while grouping by repository",
			content: canvascontent.Content{
				GroupedByRepository: true,
			},
			expectedHeadings: []string{"## Open"},
		},
	}

	allHeadings := []string{
		"## Ready to merge", "## Waiting for author", "## Waiting for review", "## Open",
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			markdown := canvasbuilder.BuildMarkdown(tc.content)

			for _, heading := range allHeadings {
				wantHeading := slices.Contains(tc.expectedHeadings, heading)
				if gotHeading := strings.Contains(markdown, heading+"\n"); gotHeading != wantHeading {
					t.Errorf(
						"heading %q present: %t, expected present: %t, in:\n%s",
						heading, gotHeading, wantHeading, markdown,
					)
				}
			}
			if strings.Contains(markdown, noOpenPRsTextInRender) != (len(tc.expectedHeadings) == 1 &&
				tc.expectedHeadings[0] == "## Open") {
				t.Errorf("unexpected open-PR fallback line in:\n%s", markdown)
			}
			// A hidden section must drop out entirely: an empty block instead would leave a
			// third newline where two blocks join.
			if strings.Contains(markdown, "\n\n\n") {
				t.Errorf("expected no blank block between sections, got:\n%q", markdown)
			}
		})
	}
}

// The fallback line canvasbuilder renders for an empty open section, spelled out here so the
// test fails if the renderer stops emitting it.
const noOpenPRsTextInRender = "_No open PRs_"

// A snapshot can't hold this: re-recording would silently accept an added title. Slack renders
// the canvas title as its own sticky H1 and doesn't dedupe a matching one from the body, so a
// top-level heading here shows the title twice.
func TestBuildMarkdownHasNoTopLevelHeading(t *testing.T) {
	openPR := testPR(prOptions{
		number: 1, title: "Add pagination to the PR listing", authorName: "Alice Anderson",
		age: hoursAge,
	})
	wipPR := testPR(prOptions{
		number: 3, title: "Spike: replace mux with chi", authorName: "Carol Clark",
		age: hoursAge, activityAge: durationPointer(hoursAge),
	})
	mergedPR := testPR(prOptions{
		number: 9, title: "Drop the REST fallback", authorName: "Alice Anderson",
		age: oldAge, mergeAge: durationPointer(idleAge),
	})

	testCases := []struct {
		name    string
		content canvascontent.Content
	}{
		{
			name: "flat",
			content: canvascontent.Content{
				WaitingForReview: canvascontent.PRSection{PRs: []prview.PR{openPR}},
				WIP:              canvascontent.PRSection{PRs: []prview.PR{wipPR}},
				Merged:           canvascontent.PRSection{PRs: []prview.PR{mergedPR}},
				GeneratedAt:      generatedAt,
			},
		},
		{
			name: "grouped by repository",
			content: canvascontent.Content{
				WaitingForReview: canvascontent.PRSection{
					Groups: prview.GroupPRsByRepositories([]prview.PR{openPR}),
				},
				GroupedByRepository: true,
				WIP:                 canvascontent.PRSection{PRs: []prview.PR{wipPR}},
				Merged:              canvascontent.PRSection{PRs: []prview.PR{mergedPR}},
				GeneratedAt:         generatedAt,
			},
		},
		{
			name:    "no PRs at all",
			content: canvascontent.Content{GeneratedAt: generatedAt},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			markdown := canvasbuilder.BuildMarkdown(tc.content)
			for lineNumber, line := range strings.Split(markdown, "\n") {
				if strings.HasPrefix(line, "# ") {
					t.Errorf(
						"Expected no top-level heading, got one on line %d: %s", lineNumber+1, line,
					)
				}
			}
		})
	}
}
