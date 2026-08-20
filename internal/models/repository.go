package models

import (
	"fmt"
	"strings"
)

type Repository struct {
	Owner string
	Name  string
}

// GetPath returns the full path to the repository, e.g. "owner/repo"
func (r Repository) GetPath() string {
	return fmt.Sprintf("%s/%s", r.Owner, r.Name)
}

// GetPullsURL returns the repository's GitHub pull requests page.
func (r Repository) GetPullsURL() string {
	return fmt.Sprintf("https://github.com/%s/pulls", r.GetPath())
}

func (r Repository) String() string {
	return r.GetPath()
}

func ParseRepository(repository string) (Repository, error) {
	repoParts := strings.Split(repository, "/")
	if len(repoParts) != 2 {
		return Repository{}, fmt.Errorf("invalid owner/repository format: %s", repository)
	}
	repoOwner := repoParts[0]
	repoName := repoParts[1]

	if repoOwner == "" || repoName == "" {
		return Repository{}, fmt.Errorf("owner or repository name cannot be empty in: %s", repository)
	}

	return Repository{
		Owner: repoOwner,
		Name:  repoName,
	}, nil
}
