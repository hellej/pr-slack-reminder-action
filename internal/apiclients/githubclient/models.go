package githubclient

import (
	"cmp"
	"log"
	"slices"

	"github.com/google/go-github/v72/github"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type PR struct {
	*github.PullRequest
	Repository       models.Repository
	Author           Collaborator
	ApprovedByUsers  []Collaborator
	CommentedByUsers []Collaborator // reviewers who commented the PR but did not approve it
}

type PRResult struct {
	pr         *github.PullRequest
	repository models.Repository
}

type FetchReviewsResult struct {
	pr               *github.PullRequest
	reviews          []*github.PullRequestReview
	comments         []*github.PullRequestComment
	timelineComments []*github.IssueComment
	repository       models.Repository
	err              error
}

func (r FetchReviewsResult) printResult() {
	if r.err != nil {
		log.Printf("Unable to fetch reviews/comments for PR #%d: %v", r.pr.GetNumber(), r.err)
	} else {
		log.Printf("Found %d reviews, %d PR comments, and %d timeline comments for PR %v/%d", len(r.reviews), len(r.comments), len(r.timelineComments), r.repository, r.pr.GetNumber())
	}
}

type Collaborator struct {
	Login string // GitHub username
	Name  string // GitHub name if available
}

func newCollaboratorFromUser(user *github.User) Collaborator {
	return Collaborator{
		Login: user.GetLogin(),
		Name:  user.GetName(),
	}
}

func isBot(user *github.User) bool {
	userType := user.GetType()
	return userType == "Bot"
}

type GitHubUserProvider interface {
	GetUser() *github.User
}

// Returns the GitHub name if available, otherwise login.
func (c Collaborator) GetGitHubName() string {
	return cmp.Or(c.Name, c.Login)
}

func (r FetchReviewsResult) asPR() PR {
	authorLogin := r.pr.GetUser().GetLogin()

	reviewsWithValidUser := utilities.Filter(r.reviews, hasValidUserData)
	commentsWithValidUser := utilities.Filter(r.comments, hasValidUserData)
	timelineCommentsWithValidUser := utilities.Filter(r.timelineComments, hasValidUserData)

	approvingReviews := utilities.Filter(reviewsWithValidUser, isApprovingReview)
	approvedByUsers := extractUniqueCollaborators(approvingReviews)

	reviewCommenters := extractUniqueCollaborators(reviewsWithValidUser)
	standaloneCommenters := extractUniqueCollaborators(commentsWithValidUser)
	timelineCommenters := extractUniqueCollaborators(timelineCommentsWithValidUser)

	allCommenters := slices.Concat(reviewCommenters, standaloneCommenters, timelineCommenters)
	commentedByUsers := utilities.Filter(
		utilities.UniqueFunc(allCommenters, isUniqueCollaborator),
		getFilterForCommenters(authorLogin, approvedByUsers),
	)

	return PR{
		PullRequest:      r.pr,
		Repository:       r.repository,
		Author:           newCollaboratorFromUser(r.pr.GetUser()),
		ApprovedByUsers:  approvedByUsers,
		CommentedByUsers: commentedByUsers,
	}
}

func hasValidUserData[T GitHubUserProvider](item T) bool {
	user := item.GetUser()
	return user != nil && user.GetLogin() != "" && !isBot(user)
}

func extractUniqueCollaborators[T GitHubUserProvider](items []T) []Collaborator {
	return utilities.UniqueFunc(
		utilities.Map(items, getCollaborator[T]), isUniqueCollaborator,
	)
}

func getCollaborator[T GitHubUserProvider](item T) Collaborator {
	return newCollaboratorFromUser(item.GetUser())
}

func isUniqueCollaborator(a, b Collaborator) bool {
	return a.Login == b.Login
}

func isApprovingReview(review *github.PullRequestReview) bool {
	return review.GetState() == "APPROVED"
}

func getFilterForCommenters(authorLogin string, approvedByUsers []Collaborator) func(c Collaborator) bool {
	return func(c Collaborator) bool {
		return c.Login != authorLogin &&
			!slices.ContainsFunc(approvedByUsers, func(approver Collaborator) bool {
				return c.Login == approver.Login
			})
	}
}
