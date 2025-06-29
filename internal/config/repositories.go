package config

import (
	"fmt"
	"strings"
)

type Repository struct {
	// Path is the full path to the repository, e.g. "owner/repo"
	Path  string
	Owner string
	Name  string
}

func (r Repository) String() string {
	return r.Path
}

func parseRepository(repository string) (Repository, error) {
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
		Path:  repository,
		Owner: repoOwner,
		Name:  repoName,
	}, nil

}
