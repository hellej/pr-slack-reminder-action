package githubclient

import (
	"testing"
)

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
