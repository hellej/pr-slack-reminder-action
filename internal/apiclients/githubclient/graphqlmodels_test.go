package githubclient

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
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
			if valid := hasValidAuthorNode(author); valid != tt.expectedValid {
				t.Errorf("hasValidAuthorNode() = %t, expected %t", valid, tt.expectedValid)
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
