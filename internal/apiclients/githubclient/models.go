package githubclient

import (
	"cmp"
	"log"
	"slices"
	"time"

	"github.com/google/go-github/v78/github"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type PullRequest struct {
	Number    int
	Title     string
	HTMLURL   string
	CreatedAt time.Time
	UpdatedAt time.Time
	State     string
	Merged    bool
	Draft     bool
	Labels    []string
	Author    Collaborator
	HeadSHA   string
}

func (p *PullRequest) GetNumber() int {
	if p == nil {
		return 0
	}
	return p.Number
}

func (p *PullRequest) GetTitle() string {
	if p == nil {
		return ""
	}
	return p.Title
}

func (p *PullRequest) GetHTMLURL() string {
	if p == nil {
		return ""
	}
	return p.HTMLURL
}

func (p *PullRequest) GetCreatedAt() time.Time {
	if p == nil {
		return time.Time{}
	}
	return p.CreatedAt
}

func (p *PullRequest) GetUpdatedAt() time.Time {
	if p == nil {
		return time.Time{}
	}
	return p.UpdatedAt
}

func (p *PullRequest) GetState() string {
	if p == nil {
		return ""
	}
	return p.State
}

func (p *PullRequest) GetMerged() bool {
	if p == nil {
		return false
	}
	return p.Merged
}

func (p *PullRequest) GetDraft() bool {
	if p == nil {
		return false
	}
	return p.Draft
}

func newPullRequestFromGitHubPR(pr *github.PullRequest) *PullRequest {
	if pr == nil {
		return &PullRequest{}
	}
	return &PullRequest{
		Number:    pr.GetNumber(),
		Title:     pr.GetTitle(),
		HTMLURL:   pr.GetHTMLURL(),
		CreatedAt: pr.GetCreatedAt().Time,
		UpdatedAt: pr.GetUpdatedAt().Time,
		State:     pr.GetState(),
		Merged:    pr.GetMerged(),
		Draft:     pr.GetDraft(),
		Labels:    utilities.Map(pr.Labels, func(l *github.Label) string { return l.GetName() }),
		Author:    newCollaboratorFromUser(pr.GetUser()),
		HeadSHA:   pr.GetHead().GetSHA(),
	}
}

type PR struct {
	*PullRequest
	Repository       models.Repository
	Author           Collaborator
	ApprovedByUsers  []Collaborator
	CommentedByUsers []Collaborator // reviewers who commented the PR but did not approve it
	SnoozedUntil     *time.Time
}

type PRResult struct {
	pr         *PullRequest
	repository models.Repository
}

// Filtering, capping and logging run over listed PRs in "post" run mode and over fetched ones in
// "update" run mode.
type repositoryPullRequest interface {
	getPullRequest() *PullRequest
	getRepository() models.Repository
}

func (r PRResult) getPullRequest() *PullRequest { return r.pr }

func (r PRResult) getRepository() models.Repository { return r.repository }

func (p PR) getPullRequest() *PullRequest { return p.PullRequest }

func (p PR) getRepository() models.Repository { return p.Repository }

type FetchReviewsResult struct {
	pr               *PullRequest
	reviews          []*github.PullRequestReview
	comments         []*github.PullRequestComment
	timelineComments []TimelineComment
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

type TimelineComment struct {
	Body        string
	CreatedAt   time.Time
	Author      Collaborator
	AuthorIsBot bool
}

func newTimelineCommentFromIssueComment(comment *github.IssueComment) TimelineComment {
	return TimelineComment{
		Body:        comment.GetBody(),
		CreatedAt:   comment.GetCreatedAt().Time,
		Author:      newCollaboratorFromUser(comment.GetUser()),
		AuthorIsBot: isBot(comment.GetUser()),
	}
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
	reviewsWithValidUser := utilities.Filter(r.reviews, hasValidUserData)
	commentsWithValidUser := utilities.Filter(r.comments, hasValidUserData)
	approvingReviews := utilities.Filter(reviewsWithValidUser, isApprovingReview)

	approvedByUsers, commentedByUsers := deriveReviewers(
		r.pr.Author.Login,
		extractUniqueCollaborators(approvingReviews),
		extractUniqueCollaborators(reviewsWithValidUser),
		extractUniqueCollaborators(commentsWithValidUser),
		extractTimelineCommenters(r.timelineComments),
	)

	return PR{
		PullRequest:      r.pr,
		Repository:       r.repository,
		Author:           r.pr.Author,
		ApprovedByUsers:  approvedByUsers,
		CommentedByUsers: commentedByUsers,
		SnoozedUntil:     findActiveSnooze(r.timelineComments),
	}
}

func deriveReviewers(
	authorLogin string,
	approvers, reviewAuthors, reviewCommentAuthors, timelineCommenters []Collaborator,
) (approvedBy, commentedBy []Collaborator) {
	approvedBy = utilities.UniqueFunc(approvers, isUniqueCollaborator)

	allCommenters := slices.Concat(reviewAuthors, reviewCommentAuthors, timelineCommenters)
	commentedBy = utilities.Filter(
		utilities.UniqueFunc(allCommenters, isUniqueCollaborator),
		getFilterForCommenters(authorLogin, approvedBy),
	)
	return approvedBy, commentedBy
}

func extractTimelineCommenters(comments []TimelineComment) []Collaborator {
	commentsWithValidAuthor := utilities.Filter(comments, hasValidTimelineCommentAuthor)
	return utilities.Map(commentsWithValidAuthor, func(c TimelineComment) Collaborator { return c.Author })
}

func hasValidTimelineCommentAuthor(comment TimelineComment) bool {
	return !comment.AuthorIsBot && comment.Author.Login != ""
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
