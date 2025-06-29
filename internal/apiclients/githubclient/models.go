package githubclient

import (
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
	CommentedByUsers []string // reviewers who commented the PR but did not approve it
	ApprovedByUsers  []string
}

func (pr PR) GetUsername() string {
	if pr.GetUser() != nil {
		return pr.GetUser().GetLogin()
	}
	return ""
}

func (pr PR) GetAuthorNameOrUsername() string {
	if pr.GetUser() != nil {
		if pr.GetUser().GetName() != "" {
			return pr.GetUser().GetName()
		}
		return pr.GetUser().GetLogin()
	}
	return ""
}

func (pr PR) isMatch(filters config.Filters) bool {
	if len(filters.Labels) > 0 {
		if !slices.ContainsFunc(pr.Labels, func(l *github.Label) bool {
			return slices.Contains(filters.Labels, l.GetName())
		}) {
			return false
		}
	}
	if len(filters.Authors) > 0 {
		if !slices.Contains(filters.Authors, pr.GetUsername()) {
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
		log.Printf("Review by %s: %s", review.GetUser().GetLogin(), *review.State)
	}
}

func (r FetchReviewsResult) asPR() PR {
	approvedByUsers := []string{}
	commentedByUsers := []string{}

	for _, review := range r.reviews {
		user := review.GetUser().GetLogin()
		if user == "" {
			continue
		}
		if review.GetState() == "APPROVED" {
			if !slices.Contains(approvedByUsers, user) {
				approvedByUsers = append(approvedByUsers, user)
			}
		} else {
			if !slices.Contains(commentedByUsers, user) && !slices.Contains(approvedByUsers, user) {
				// Only add to commentedByUsers if the user has not already approved
				commentedByUsers = append(commentedByUsers, user)
			}
		}
	}

	return PR{
		PullRequest:      r.pr,
		Repository:       r.repository,
		CommentedByUsers: commentedByUsers,
		ApprovedByUsers:  approvedByUsers,
	}
}

type OwnerAndRepo struct {
	Owner string
	Repo  string
}
