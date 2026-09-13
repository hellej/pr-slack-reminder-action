package githubclient

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
)

func decodeAuthorNode(t *testing.T, authorJSON string) *authorNode {
	t.Helper()
	var wrapper struct {
		Author *authorNode `json:"author"`
	}
	if err := json.Unmarshal([]byte(`{"author":`+authorJSON+`}`), &wrapper); err != nil {
		t.Fatalf("unable to decode author node: %v", err)
	}
	return wrapper.Author
}

func TestCollaboratorFromAuthorNode(t *testing.T) {
	tests := []struct {
		name                 string
		authorJSON           string
		expectedCollaborator Collaborator
		expectedGitHubName   string
		expectedValid        bool
	}{
		{
			name:                 "user with a name",
			authorJSON:           `{"login":"user1","__typename":"User","name":"User One"}`,
			expectedCollaborator: Collaborator{Login: "user1", Name: "User One"},
			expectedGitHubName:   "User One",
			expectedValid:        true,
		},
		{
			name:                 "user without a name",
			authorJSON:           `{"login":"user1","__typename":"User","name":null}`,
			expectedCollaborator: Collaborator{Login: "user1"},
			expectedGitHubName:   "user1",
			expectedValid:        true,
		},
		{
			name:                 "bot",
			authorJSON:           `{"login":"dependabot","__typename":"Bot"}`,
			expectedCollaborator: Collaborator{Login: "dependabot[bot]"},
			expectedGitHubName:   "dependabot[bot]",
			expectedValid:        false,
		},
		{
			name:                 "mannequin",
			authorJSON:           `{"login":"mannequin1","__typename":"Mannequin"}`,
			expectedCollaborator: Collaborator{Login: "mannequin1"},
			expectedGitHubName:   "mannequin1",
			expectedValid:        true,
		},
		{
			name:                 "null author",
			authorJSON:           `null`,
			expectedCollaborator: Collaborator{},
			expectedGitHubName:   "",
			expectedValid:        false,
		},
		{
			name:                 "empty login",
			authorJSON:           `{"login":"","__typename":"User","name":"User One"}`,
			expectedCollaborator: Collaborator{Name: "User One"},
			expectedGitHubName:   "User One",
			expectedValid:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			author := decodeAuthorNode(t, tt.authorJSON)

			collaborator := collaboratorFromAuthorNode(author)
			if !reflect.DeepEqual(collaborator, tt.expectedCollaborator) {
				t.Errorf("collaboratorFromAuthorNode() = %+v, expected %+v", collaborator, tt.expectedCollaborator)
			}
			if gitHubName := collaborator.GetGitHubName(); gitHubName != tt.expectedGitHubName {
				t.Errorf("GetGitHubName() = %s, expected %s", gitHubName, tt.expectedGitHubName)
			}
			if valid := hasKnownNonBotAuthorNode(author); valid != tt.expectedValid {
				t.Errorf("hasKnownNonBotAuthorNode() = %t, expected %t", valid, tt.expectedValid)
			}
		})
	}
}

func TestPullRequestFromNodeMapsStateAndMerged(t *testing.T) {
	tests := []struct {
		name           string
		nodeState      string
		nodeMerged     bool
		expectedState  string
		expectedMerged bool
	}{
		{name: "open PR", nodeState: "OPEN", expectedState: "open"},
		{name: "closed PR without merge", nodeState: "CLOSED", expectedState: "closed"},
		{
			name: "merged PR", nodeState: "MERGED", nodeMerged: true,
			expectedState: "closed", expectedMerged: true,
		},
		{name: "missing state renders as open", expectedState: "open"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pullRequest := pullRequestFromNode(
				pullRequestNode{State: tt.nodeState, Merged: tt.nodeMerged},
			)

			if pullRequest.GetState() != tt.expectedState {
				t.Errorf("GetState() = %q, expected %q", pullRequest.GetState(), tt.expectedState)
			}
			if pullRequest.GetMerged() != tt.expectedMerged {
				t.Errorf("GetMerged() = %t, expected %t", pullRequest.GetMerged(), tt.expectedMerged)
			}
		})
	}
}

func TestTimelineCommentFromNode(t *testing.T) {
	createdAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		commentJSON string
		expected    TimelineComment
	}{
		{
			name:        "user comment",
			commentJSON: `{"createdAt":"2026-05-01T12:00:00Z","body":"looks good","author":{"login":"user1","__typename":"User","name":"User One"}}`,
			expected: TimelineComment{
				Body:      "looks good",
				CreatedAt: createdAt,
			},
		},
		{
			name:        "bot comment",
			commentJSON: `{"createdAt":"2026-05-01T12:00:00Z","body":"/snooze [pr-reminder] for 2 days","author":{"login":"dependabot","__typename":"Bot"}}`,
			expected: TimelineComment{
				Body:      "/snooze [pr-reminder] for 2 days",
				CreatedAt: createdAt,
			},
		},
		{
			name:        "comment without an author",
			commentJSON: `{"createdAt":"2026-05-01T12:00:00Z","body":"orphaned","author":null}`,
			expected: TimelineComment{
				Body:      "orphaned",
				CreatedAt: createdAt,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var comment commentNode
			if err := json.Unmarshal([]byte(tt.commentJSON), &comment); err != nil {
				t.Fatalf("unable to decode comment node: %v", err)
			}
			if got := timelineCommentFromNode(comment); !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("timelineCommentFromNode() = %+v, expected %+v", got, tt.expected)
			}
		})
	}
}

func userAuthorNode(login string) *authorNode {
	return &authorNode{Login: login, Typename: userTypename, Name: login}
}

func botAuthorNode(login string) *authorNode {
	return &authorNode{Login: login, Typename: botTypename}
}

func threadWithLastComment(isResolved bool, lastCommentAuthor *authorNode) reviewThreadNode {
	return reviewThreadNode{
		IsResolved: isResolved,
		Comments:   connection[commentNode]{Nodes: []commentNode{{Author: lastCommentAuthor}}},
	}
}

func threadWithoutComments(isResolved bool) reviewThreadNode {
	return reviewThreadNode{IsResolved: isResolved}
}

func prWithReviewersOfNode(author *authorNode, node pullRequestNode) PR {
	node.Author = author
	return prWithReviewers(pullRequestFromNode(node), models.Repository{}, node)
}

// The last comment decides, so the same thread state flips on who wrote into it last.
func TestPRWithReviewersDerivesThreadWaitingForAuthor(t *testing.T) {
	tests := []struct {
		name     string
		author   *authorNode
		threads  []reviewThreadNode
		expected bool
	}{
		{
			name:    "no threads at all",
			author:  userAuthorNode("alice"),
			threads: nil,
		},
		{
			name:    "resolved thread with a reviewer's last comment",
			author:  userAuthorNode("alice"),
			threads: []reviewThreadNode{threadWithLastComment(true, userAuthorNode("bob"))},
		},
		{
			name:    "unresolved thread with the author's last comment",
			author:  userAuthorNode("alice"),
			threads: []reviewThreadNode{threadWithLastComment(false, userAuthorNode("alice"))},
		},
		{
			name:     "unresolved thread with a reviewer's last comment",
			author:   userAuthorNode("alice"),
			threads:  []reviewThreadNode{threadWithLastComment(false, userAuthorNode("bob"))},
			expected: true,
		},
		{
			name:     "unresolved thread without comments",
			author:   userAuthorNode("alice"),
			threads:  []reviewThreadNode{threadWithoutComments(false)},
			expected: true,
		},
		{
			name:     "unresolved thread whose last comment has no author",
			author:   userAuthorNode("alice"),
			threads:  []reviewThreadNode{threadWithLastComment(false, nil)},
			expected: true,
		},
		{
			// An authorless PR must not swallow an authorless comment as its own.
			name:     "unresolved thread without an author on either side",
			author:   nil,
			threads:  []reviewThreadNode{threadWithLastComment(false, nil)},
			expected: true,
		},
		{
			name:   "only the third thread blocks",
			author: userAuthorNode("alice"),
			threads: []reviewThreadNode{
				threadWithLastComment(true, userAuthorNode("bob")),
				threadWithLastComment(false, userAuthorNode("alice")),
				threadWithLastComment(false, userAuthorNode("carol")),
			},
			expected: true,
		},
		{
			name:     "unresolved thread whose last comment has an empty login",
			author:   userAuthorNode("alice"),
			threads:  []reviewThreadNode{threadWithLastComment(false, &authorNode{Typename: userTypename})},
			expected: true,
		},
		{
			// A review bot opens a thread on every new PR, which would hand each one back
			// to its author before a human had looked at it.
			name:    "unresolved thread a bot left on a human's PR",
			author:  userAuthorNode("alice"),
			threads: []reviewThreadNode{threadWithLastComment(false, botAuthorNode("copilot"))},
		},
		{
			name:    "unresolved thread a bot left on its own PR",
			author:  botAuthorNode("dependabot"),
			threads: []reviewThreadNode{threadWithLastComment(false, botAuthorNode("dependabot"))},
		},
		{
			// The shape a repository with a GitHub App reviewing every commit produces.
			name:   "unresolved bot threads and nothing from a person",
			author: userAuthorNode("alice"),
			threads: []reviewThreadNode{
				threadWithLastComment(false, botAuthorNode("copilot")),
				threadWithLastComment(false, botAuthorNode("copilot")),
				threadWithLastComment(false, botAuthorNode("sonarqube")),
			},
		},
		{
			name:   "one person's thread among the bot threads",
			author: userAuthorNode("alice"),
			threads: []reviewThreadNode{
				threadWithLastComment(false, botAuthorNode("copilot")),
				threadWithLastComment(false, userAuthorNode("bob")),
				threadWithLastComment(false, botAuthorNode("sonarqube")),
			},
			expected: true,
		},
		{
			name:     "unresolved thread a reviewer left on a bot's PR",
			author:   botAuthorNode("dependabot"),
			threads:  []reviewThreadNode{threadWithLastComment(false, userAuthorNode("bob"))},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := prWithReviewersOfNode(tt.author, pullRequestNode{
				ReviewThreads: connection[reviewThreadNode]{Nodes: tt.threads},
			})

			if pr.HasThreadWaitingForAuthor != tt.expected {
				t.Errorf(
					"HasThreadWaitingForAuthor = %t, expected %t",
					pr.HasThreadWaitingForAuthor, tt.expected,
				)
			}
		})
	}
}

func TestPRWithReviewersDerivesConflicting(t *testing.T) {
	tests := []struct {
		name      string
		mergeable string
		expected  bool
	}{
		{name: "conflicting", mergeable: "CONFLICTING", expected: true},
		{name: "mergeable", mergeable: "MERGEABLE"},
		{name: "mergeability still being computed", mergeable: "UNKNOWN"},
		{name: "mergeability not returned at all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := prWithReviewersOfNode(
				userAuthorNode("alice"), pullRequestNode{Mergeable: tt.mergeable},
			)

			if pr.Conflicting != tt.expected {
				t.Errorf("Conflicting = %t, expected %t", pr.Conflicting, tt.expected)
			}
		})
	}
}

func TestPRWithReviewersDerivesNonApprovingReviews(t *testing.T) {
	tests := []struct {
		name     string
		author   *authorNode
		reviews  []reviewNode
		expected bool
	}{
		{
			name:    "no reviews at all",
			author:  userAuthorNode("alice"),
			reviews: nil,
		},
		{
			name:     "a reviewer commented",
			author:   userAuthorNode("alice"),
			reviews:  []reviewNode{{State: "COMMENTED", Author: userAuthorNode("bob")}},
			expected: true,
		},
		{
			name:     "a reviewer requested changes",
			author:   userAuthorNode("alice"),
			reviews:  []reviewNode{{State: "CHANGES_REQUESTED", Author: userAuthorNode("carol")}},
			expected: true,
		},
		{
			name:    "a reviewer approved",
			author:  userAuthorNode("alice"),
			reviews: []reviewNode{{State: "APPROVED", Author: userAuthorNode("bob")}},
		},
		{
			// A dismissed review has been withdrawn, so it asks the author for nothing.
			name:    "a reviewer's changes request was dismissed",
			author:  userAuthorNode("alice"),
			reviews: []reviewNode{{State: "DISMISSED", Author: userAuthorNode("dave")}},
		},
		{
			name:    "a reviewer has an unsubmitted review",
			author:  userAuthorNode("alice"),
			reviews: []reviewNode{{State: "PENDING", Author: userAuthorNode("erin")}},
		},
		{
			// A bare inline comment on one's own diff arrives as a COMMENTED review.
			name:    "the author commented on their own diff",
			author:  userAuthorNode("alice"),
			reviews: []reviewNode{{State: "COMMENTED", Author: userAuthorNode("alice")}},
		},
		{
			// Same comparison as the thread cases: both sides carry the [bot] suffix or
			// neither does.
			name:    "a bot commented on its own PR",
			author:  botAuthorNode("dependabot"),
			reviews: []reviewNode{{State: "COMMENTED", Author: botAuthorNode("dependabot")}},
		},
		{
			// A review bot is not a person asking for changes, so it leaves the PR in the
			// review queue rather than handing it back to the author.
			name:    "a bot commented on someone else's PR",
			author:  userAuthorNode("alice"),
			reviews: []reviewNode{{State: "COMMENTED", Author: botAuthorNode("linter")}},
		},
		{
			name:   "one reviewer approved and another commented",
			author: userAuthorNode("alice"),
			reviews: []reviewNode{
				{State: "APPROVED", Author: userAuthorNode("bob")},
				{State: "COMMENTED", Author: userAuthorNode("carol")},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := prWithReviewersOfNode(tt.author, pullRequestNode{
				Reviews: connection[reviewNode]{Nodes: tt.reviews},
			})

			if pr.HasNonApprovingReview != tt.expected {
				t.Errorf(
					"HasNonApprovingReview = %t, expected %t", pr.HasNonApprovingReview, tt.expected,
				)
			}
		})
	}
}

// A PR whose enrichment failed reaches prWithReviewers with a zero node, and must not read as
// conflicting or as anyone's turn.
func TestPRWithReviewersLeavesFlagsUnsetForAnUnenrichedPR(t *testing.T) {
	pr := prWithReviewers(
		pullRequestFromNode(pullRequestNode{Author: userAuthorNode("alice")}),
		models.Repository{},
		pullRequestNode{},
	)

	if pr.HasThreadWaitingForAuthor || pr.Conflicting || pr.HasNonApprovingReview {
		t.Errorf(
			"expected all three flags unset, got threads: %t, conflicting: %t, non-approving: %t",
			pr.HasThreadWaitingForAuthor, pr.Conflicting, pr.HasNonApprovingReview,
		)
	}
}

// The three unit tests above build nodes in Go, which cannot see a JSON tag. A mistagged
// mergeable, reviewThreads or isResolved decodes to its zero value and fails silently: no
// error, and every flag it feeds reads as nobody's turn.
func TestPRWithReviewersDerivesFlagsFromDecodedJSON(t *testing.T) {
	tests := []struct {
		name               string
		nodeJSON           string
		expectedConflicts  bool
		expectedUnresolved bool
	}{
		{
			name: "conflicting PR with one resolved and one unresolved thread",
			nodeJSON: `{
				"author":{"login":"alice","__typename":"User","name":"Alice"},
				"mergeable":"CONFLICTING",
				"reviewThreads":{"nodes":[
					{"isResolved":true,"comments":{"nodes":[
						{"author":{"login":"bob","__typename":"User"}}
					]}},
					{"isResolved":false,"comments":{"nodes":[
						{"author":{"login":"carol","__typename":"User"}}
					]}}
				]}
			}`,
			expectedConflicts:  true,
			expectedUnresolved: true,
		},
		{
			// The nested comments and author: losing either reads the thread as one nobody
			// has answered, which blocks.
			name: "PR whose author answered the only unresolved thread",
			nodeJSON: `{
				"author":{"login":"alice","__typename":"User","name":"Alice"},
				"mergeable":"MERGEABLE",
				"reviewThreads":{"nodes":[
					{"isResolved":false,"comments":{"nodes":[
						{"author":{"login":"alice","__typename":"User"}}
					]}}
				]}
			}`,
		},
		{
			// A mistagged isResolved reads this thread as unresolved, so the flag flips
			// even though the blocking case above stays true.
			name: "mergeable PR whose only thread is resolved",
			nodeJSON: `{
				"author":{"login":"alice","__typename":"User","name":"Alice"},
				"mergeable":"MERGEABLE",
				"reviewThreads":{"nodes":[
					{"isResolved":true,"comments":{"nodes":[
						{"author":{"login":"bob","__typename":"User"}}
					]}}
				]}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node pullRequestNode
			if err := json.Unmarshal([]byte(tt.nodeJSON), &node); err != nil {
				t.Fatalf("unable to decode pull request node: %v", err)
			}

			pr := prWithReviewers(pullRequestFromNode(node), models.Repository{}, node)

			if pr.Conflicting != tt.expectedConflicts {
				t.Errorf("Conflicting = %t, expected %t", pr.Conflicting, tt.expectedConflicts)
			}
			if pr.HasThreadWaitingForAuthor != tt.expectedUnresolved {
				t.Errorf(
					"HasThreadWaitingForAuthor = %t, expected %t",
					pr.HasThreadWaitingForAuthor, tt.expectedUnresolved,
				)
			}
		})
	}
}
