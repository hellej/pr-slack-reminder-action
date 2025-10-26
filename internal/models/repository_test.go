package models_test

import (
	"strings"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
)

func TestRepositoryString(t *testing.T) {
	repo := models.Repository{
		Owner: "test-org",
		Name:  "test-repo",
	}

	expected := "test-org/test-repo"
	if repo.String() != expected {
		t.Errorf("Expected repository string '%s', got '%s'", expected, repo.String())
	}
}

func TestParseRepository_Valid(t *testing.T) {
	testCases := []struct {
		name           string
		repositoryPath string
		expectedRepo   models.Repository
	}{
		{
			name:           "standard repository path",
			repositoryPath: "octocat/Hello-World",
			expectedRepo: models.Repository{
				Owner: "octocat",
				Name:  "Hello-World",
			},
		},
		{
			name:           "repository with numbers",
			repositoryPath: "org123/repo456",
			expectedRepo: models.Repository{
				Owner: "org123",
				Name:  "repo456",
			},
		},
		{
			name:           "repository with hyphens and underscores",
			repositoryPath: "my-org/my_repo-name",
			expectedRepo: models.Repository{
				Owner: "my-org",
				Name:  "my_repo-name",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo, err := models.ParseRepository(tc.repositoryPath)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
			if repo.GetPath() != tc.expectedRepo.GetPath() {
				t.Errorf("Expected repository path '%s', got '%s'", tc.expectedRepo.GetPath(), repo.GetPath())
			}
			if repo.Owner != tc.expectedRepo.Owner {
				t.Errorf("Expected repository owner '%s', got '%s'", tc.expectedRepo.Owner, repo.Owner)
			}
			if repo.Name != tc.expectedRepo.Name {
				t.Errorf("Expected repository name '%s', got '%s'", tc.expectedRepo.Name, repo.Name)
			}
		})
	}
}

func TestParseRepository_Invalid(t *testing.T) {
	testCases := []struct {
		name           string
		repositoryPath string
		expectedErrMsg string
	}{
		{
			name:           "no slash",
			repositoryPath: "invalid-repo-format",
			expectedErrMsg: "invalid owner/repository format: invalid-repo-format",
		},
		{
			name:           "too many slashes",
			repositoryPath: "org/repo/extra",
			expectedErrMsg: "invalid owner/repository format: org/repo/extra",
		},
		{
			name:           "empty owner",
			repositoryPath: "/repo-name",
			expectedErrMsg: "owner or repository name cannot be empty in: /repo-name",
		},
		{
			name:           "empty repository name",
			repositoryPath: "org-name/",
			expectedErrMsg: "owner or repository name cannot be empty in: org-name/",
		},
		{
			name:           "both empty",
			repositoryPath: "/",
			expectedErrMsg: "owner or repository name cannot be empty in: /",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := models.ParseRepository(tc.repositoryPath)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.expectedErrMsg) {
				t.Errorf("Expected error to contain '%s', got '%s'", tc.expectedErrMsg, err.Error())
			}
		})
	}
}
