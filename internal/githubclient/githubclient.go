package githubclient

import "github.com/google/go-github/v72/github"

func GetClient(token string) *github.Client {
	return github.NewClient(nil).WithAuthToken(token)
}
