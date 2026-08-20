package canvasbuilder_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/canvasbuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/canvascontent"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
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
}

func testPR(options prOptions) prparser.PR {
	repository := models.Repository{Owner: "test-org", Name: "test-repo"}
	if options.repository != "" {
		repository = models.Repository{Owner: "test-org", Name: options.repository}
	}
	var lastActivityAt *time.Time
	if options.activityAge != nil {
		timestamp := time.Now().Add(-*options.activityAge)
		lastActivityAt = &timestamp
	}
	return prparser.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{
				Number:         options.number,
				Title:          options.title,
				HTMLURL:        fmt.Sprintf("https://github.com/%s/pull/%d", repository.GetPath(), options.number),
				CreatedAt:      time.Now().Add(-options.age),
				LastActivityAt: lastActivityAt,
			},
			Repository: repository,
		},
		Author:     testCollaborator(options.authorName),
		Approvers:  utilities.Map(options.approvers, testCollaborator),
		Commenters: utilities.Map(options.commenters, testCollaborator),
		IsOldPR:    options.isOldPR,
	}
}

func testCollaborator(name string) prparser.Collaborator {
	return prparser.NewCollaborator(githubclient.Collaborator{Login: "login", Name: name}, "U1234567890")
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

	testCases := []struct {
		name    string
		content canvascontent.Content
	}{
		{
			name: "flat open PRs and WIP PRs",
			content: canvascontent.Content{
				OpenPRs:     []prparser.PR{openPR, otherOpenPR},
				WIPPRs:      []prparser.PR{wipPR},
				GeneratedAt: generatedAt,
			},
		},
		{
			name: "open PRs grouped by repository",
			content: canvascontent.Content{
				OpenPRsGroupedByRepository: prparser.GroupPRsByRepositories(
					[]prparser.PR{openPR, otherOpenPR},
				),
				GroupedByRepository: true,
				WIPPRs:              []prparser.PR{wipPR},
				GeneratedAt:         generatedAt,
			},
		},
		{
			name: "no open PRs",
			content: canvascontent.Content{
				WIPPRs:      []prparser.PR{wipPR},
				GeneratedAt: generatedAt,
			},
		},
		{
			name: "no open PRs while grouping by repository",
			content: canvascontent.Content{
				GroupedByRepository: true,
				WIPPRs:              []prparser.PR{wipPR},
				GeneratedAt:         generatedAt,
			},
		},
		{
			name: "no WIP PRs",
			content: canvascontent.Content{
				OpenPRs:     []prparser.PR{openPR},
				GeneratedAt: generatedAt,
			},
		},
		{
			name:    "no PRs at all",
			content: canvascontent.Content{GeneratedAt: generatedAt},
		},
		{
			name: "old open PR",
			content: canvascontent.Content{
				OpenPRs: []prparser.PR{
					testPR(prOptions{
						number: 4, title: "Old PR past the threshold", authorName: "Bob Brown",
						age: oldAge, isOldPR: true,
					}),
					openPR,
				},
				GeneratedAt: generatedAt,
			},
		},
		{
			name: "idle WIP PR",
			content: canvascontent.Content{
				WIPPRs: []prparser.PR{
					testPR(prOptions{
						number: 5, title: "Refactor state store", authorName: "Carol Clark",
						age: oldAge, activityAge: durationPointer(idleAge),
						approvers: []string{"Dana Davis"}, commenters: []string{"Erin Evans"},
					}),
				},
				GeneratedAt: generatedAt,
			},
		},
		{
			name: "WIP PR with unknown activity",
			content: canvascontent.Content{
				WIPPRs: []prparser.PR{
					testPR(prOptions{
						number: 6, title: "Prototype canvas rendering", authorName: "Alice Anderson",
						age: idleAge,
					}),
				},
				GeneratedAt: generatedAt,
			},
		},
		{
			name: "open PRs capped",
			content: canvascontent.Content{
				OpenPRs:       []prparser.PR{openPR},
				WIPPRs:        []prparser.PR{wipPR},
				OpenPRsCapped: true,
				GeneratedAt:   generatedAt,
			},
		},
		{
			name: "WIP PRs capped",
			content: canvascontent.Content{
				OpenPRs:      []prparser.PR{openPR},
				WIPPRs:       []prparser.PR{wipPR},
				WIPPRsCapped: true,
				GeneratedAt:  generatedAt,
			},
		},
		{
			name: "both sections capped",
			content: canvascontent.Content{
				OpenPRs:       []prparser.PR{openPR},
				WIPPRs:        []prparser.PR{wipPR},
				OpenPRsCapped: true,
				WIPPRsCapped:  true,
				GeneratedAt:   generatedAt,
			},
		},
		{
			name: "generated at a non-UTC time",
			content: canvascontent.Content{
				OpenPRs: []prparser.PR{openPR},
				WIPPRs:  []prparser.PR{wipPR},
				// The same moment as every other case's generatedAt, three hours east of UTC.
				GeneratedAt: generatedAt.In(time.FixedZone("EEST", 3*60*60)),
			},
		},
		{
			name: "markdown characters in titles and names",
			content: canvascontent.Content{
				OpenPRsGroupedByRepository: prparser.GroupPRsByRepositories([]prparser.PR{
					testPR(prOptions{
						number:     7,
						title:      "Fix [ABC-123] crash in `make test` & **WIP** _debug_ ~legacy~ C:\\path <b>",
						repository: "repo_two",
						authorName: "Al*ice_And[erson]",
						commenters: []string{"E~rin <Evans>"},
						age:        hoursAge,
					}),
				}),
				GroupedByRepository: true,
				WIPPRs: []prparser.PR{
					testPR(prOptions{
						number:      8,
						title:       "Draft: &amp; <https://example.com> \\_escaped_",
						authorName:  "B`ob & Brown",
						age:         hoursAge,
						activityAge: durationPointer(minutesAge),
					}),
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
