package githubclient

import (
	"cmp"
	"log"
	"slices"

	"github.com/google/go-github/v72/github"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
)

type PRsOfRepoResult struct {
	prs        []*github.PullRequest
	repository string
	owner      string
	err        error
}

func (r PRsOfRepoResult) GetPRCount() int {
	if r.prs != nil {
		return len(r.prs)
	}
	return 0
}

type PR struct {
	*github.PullRequest
	// Repository name (just the name, no owner)
	Repository       string
	Author           Collaborator
	CommentedByUsers []Collaborator // reviewers who commented the PR but did not approve it
	ApprovedByUsers  []Collaborator
}

func (pr PR) isMatch(filters config.Filters) bool {
	if len(filters.LabelsIgnore) > 0 {
		if slices.ContainsFunc(pr.Labels, func(l *github.Label) bool {
			return slices.Contains(filters.LabelsIgnore, l.GetName())
		}) {
			return false
		}
	}
	if len(filters.AuthorsIgnore) > 0 {
		if slices.Contains(filters.AuthorsIgnore, pr.Author.Login) {
			return false
		}
	}
	if len(filters.Labels) > 0 {
		if !slices.ContainsFunc(pr.Labels, func(l *github.Label) bool {
			return slices.Contains(filters.Labels, l.GetName())
		}) {
			return false
		}
	}
	if len(filters.Authors) > 0 {
		if !slices.Contains(filters.Authors, pr.Author.Login) {
			return false
		}
	}
	return true
}

type FetchReviewsResult struct {
	pr      *github.PullRequest
	reviews []*github.PullRequestReview
	// Repository name (just the name, no owner)
	repository string
	err        error
}

func (r FetchReviewsResult) printResult() {
	if r.err != nil {
		log.Printf("Unable to fetch reviews for PR #%d: %v", r.pr.GetNumber(), r.err)
	} else {
		log.Printf("Found %d reviews for PR %v/%d", len(r.reviews), r.repository, r.pr.GetNumber())
	}
	for _, review := range r.reviews {
		log.Printf(
			"Review by %s (name: %s): %s",
			review.GetUser().GetLogin(),
			review.GetUser().GetName(),
			*review.State,
		)
	}
}

type Collaborator struct {
	Login string // GitHub username
	Name  string // GitHub name if available
}

func NewCollaboratorFromUser(user *github.User) Collaborator {
	return Collaborator{
		Login: user.GetLogin(),
		Name:  user.GetName(),
	}
}

// Returns the GitHub name if available, otherwise login.
func (c Collaborator) GetGitHubName() string {
	return cmp.Or(c.Name, c.Login)
}

func (r FetchReviewsResult) asPR() PR {
	approvedByUsers := []Collaborator{}
	commentedByUsers := []Collaborator{}

	for _, review := range r.reviews {
		login := review.GetUser().GetLogin()
		if login == "" {
			continue
		}
		if review.GetState() == "APPROVED" {
			if !slices.ContainsFunc(approvedByUsers, func(c Collaborator) bool {
				return c.Login == login
			}) {
				approvedByUsers = append(
					approvedByUsers, NewCollaboratorFromUser(review.GetUser()),
				)
			}

		} else {
			if !slices.ContainsFunc(commentedByUsers, func(c Collaborator) bool {
				return c.Login == login
			}) && !slices.ContainsFunc(approvedByUsers, func(c Collaborator) bool {
				// Only add to commentedByUsers if the user has not already approved
				return c.Login == login
			}) {
				commentedByUsers = append(
					commentedByUsers, NewCollaboratorFromUser(review.GetUser()),
				)
			}
		}
	}

	return PR{
		PullRequest:      r.pr,
		Repository:       r.repository,
		Author:           NewCollaboratorFromUser(r.pr.GetUser()),
		CommentedByUsers: commentedByUsers,
		ApprovedByUsers:  approvedByUsers,
	}
}

type OwnerAndRepo struct {
	Owner string
	Repo  string
}
