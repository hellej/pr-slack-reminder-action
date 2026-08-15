package githubclient

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/go-github/v78/github"
)

func TestPullRequestGetters(t *testing.T) {
	createdAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 5, 2, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		pr       *PullRequest
		expected PullRequest
	}{
		{
			name:     "nil pull request",
			pr:       nil,
			expected: PullRequest{},
		},
		{
			name:     "zero values",
			pr:       &PullRequest{},
			expected: PullRequest{},
		},
		{
			name: "all fields set",
			pr: &PullRequest{
				Number:    7,
				Title:     "Add feature",
				HTMLURL:   "https://github.com/owner/repo/pull/7",
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
				State:     "closed",
				Merged:    true,
				Draft:     true,
			},
			expected: PullRequest{
				Number:    7,
				Title:     "Add feature",
				HTMLURL:   "https://github.com/owner/repo/pull/7",
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
				State:     "closed",
				Merged:    true,
				Draft:     true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pr.GetNumber(); got != tt.expected.Number {
				t.Errorf("GetNumber() = %d, expected %d", got, tt.expected.Number)
			}
			if got := tt.pr.GetTitle(); got != tt.expected.Title {
				t.Errorf("GetTitle() = %s, expected %s", got, tt.expected.Title)
			}
			if got := tt.pr.GetHTMLURL(); got != tt.expected.HTMLURL {
				t.Errorf("GetHTMLURL() = %s, expected %s", got, tt.expected.HTMLURL)
			}
			if got := tt.pr.GetCreatedAt(); !got.Equal(tt.expected.CreatedAt) {
				t.Errorf("GetCreatedAt() = %v, expected %v", got, tt.expected.CreatedAt)
			}
			if got := tt.pr.GetUpdatedAt(); !got.Equal(tt.expected.UpdatedAt) {
				t.Errorf("GetUpdatedAt() = %v, expected %v", got, tt.expected.UpdatedAt)
			}
			if got := tt.pr.GetState(); got != tt.expected.State {
				t.Errorf("GetState() = %s, expected %s", got, tt.expected.State)
			}
			if got := tt.pr.GetMerged(); got != tt.expected.Merged {
				t.Errorf("GetMerged() = %t, expected %t", got, tt.expected.Merged)
			}
			if got := tt.pr.GetDraft(); got != tt.expected.Draft {
				t.Errorf("GetDraft() = %t, expected %t", got, tt.expected.Draft)
			}
		})
	}
}

func TestNewPullRequestFromGitHubPR(t *testing.T) {
	createdAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 5, 2, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		gitHubPR *github.PullRequest
		expected PullRequest
	}{
		{
			name:     "nil pull request",
			gitHubPR: nil,
			expected: PullRequest{},
		},
		{
			name:     "all fields unset",
			gitHubPR: &github.PullRequest{},
			expected: PullRequest{},
		},
		{
			name: "all fields set",
			gitHubPR: &github.PullRequest{
				Number:    github.Ptr(7),
				Title:     github.Ptr("Add feature"),
				HTMLURL:   github.Ptr("https://github.com/owner/repo/pull/7"),
				CreatedAt: &github.Timestamp{Time: createdAt},
				UpdatedAt: &github.Timestamp{Time: updatedAt},
				State:     github.Ptr("closed"),
				Merged:    github.Ptr(true),
				Draft:     github.Ptr(true),
				Labels: []*github.Label{
					{Name: github.Ptr("bug")},
					{Name: github.Ptr("urgent")},
				},
				User: &github.User{
					Login: github.Ptr("author1"),
					Name:  github.Ptr("Author One"),
				},
				Head: &github.PullRequestBranch{SHA: github.Ptr("abc123")},
			},
			expected: PullRequest{
				Number:    7,
				Title:     "Add feature",
				HTMLURL:   "https://github.com/owner/repo/pull/7",
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
				State:     "closed",
				Merged:    true,
				Draft:     true,
				Labels:    []string{"bug", "urgent"},
				Author:    Collaborator{Login: "author1", Name: "Author One"},
				HeadSHA:   "abc123",
			},
		},
		{
			name: "nil label",
			gitHubPR: &github.PullRequest{
				Labels: []*github.Label{nil, {Name: github.Ptr("bug")}},
			},
			expected: PullRequest{Labels: []string{"", "bug"}},
		},
		{
			name: "label with nil name",
			gitHubPR: &github.PullRequest{
				Labels: []*github.Label{{}},
			},
			expected: PullRequest{Labels: []string{""}},
		},
		{
			name: "empty labels",
			gitHubPR: &github.PullRequest{
				Labels: []*github.Label{},
			},
			expected: PullRequest{Labels: nil},
		},
		{
			name: "head without sha",
			gitHubPR: &github.PullRequest{
				Head: &github.PullRequestBranch{},
			},
			expected: PullRequest{HeadSHA: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newPullRequestFromGitHubPR(tt.gitHubPR)
			if !reflect.DeepEqual(*got, tt.expected) {
				t.Errorf("newPullRequestFromGitHubPR() = %+v, expected %+v", *got, tt.expected)
			}
		})
	}
}

func TestCollaboratorGetGitHubName(t *testing.T) {
	tests := []struct {
		name         string
		collaborator Collaborator
		expected     string
	}{
		{
			name:         "has both login and name",
			collaborator: Collaborator{Login: "user1", Name: "User One"},
			expected:     "User One",
		},
		{
			name:         "has login but no name",
			collaborator: Collaborator{Login: "user1", Name: ""},
			expected:     "user1",
		},
		{
			name:         "has login but nil name equivalent",
			collaborator: Collaborator{Login: "user1"},
			expected:     "user1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.collaborator.GetGitHubName()
			if result != tt.expected {
				t.Errorf("GetGitHubName() = %s, expected %s", result, tt.expected)
			}
		})
	}
}
