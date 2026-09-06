package githubclient

import (
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const (
	botTypename  = "Bot"
	userTypename = "User"
)

const (
	openPullRequestState   = "open"
	closedPullRequestState = "closed"
)

// GraphQL states of a PR node. CLOSED and MERGED are both "closed" to the pipeline.
const (
	closedNodeState = "CLOSED"
	mergedNodeState = "MERGED"
)

const approvedReviewState = "APPROVED"

// A pending review is visible only to its own author, so it contributes no reviewer.
const pendingReviewState = "PENDING"

// Nullable Actor; name is selected through "... on User { name }" so it is set for users only.
type authorNode struct {
	Login    string `json:"login"`
	Typename string `json:"__typename"`
	Name     string `json:"name"`
}

type labelNode struct {
	Name string `json:"name"`
}

type reviewNode struct {
	State  string      `json:"state"`
	Author *authorNode `json:"author"`
}

type commentNode struct {
	CreatedAt time.Time   `json:"createdAt"`
	Body      string      `json:"body"`
	Author    *authorNode `json:"author"`
}

type connection[T any] struct {
	Nodes []T `json:"nodes"`
}

type pullRequestNode struct {
	Number    int                     `json:"number"`
	Title     string                  `json:"title"`
	URL       string                  `json:"url"`
	IsDraft   bool                    `json:"isDraft"`
	CreatedAt time.Time               `json:"createdAt"`
	UpdatedAt time.Time               `json:"updatedAt"`
	MergedAt  *time.Time              `json:"mergedAt"`
	State     string                  `json:"state"`
	Merged    bool                    `json:"merged"`
	Author    *authorNode             `json:"author"`
	Labels    connection[labelNode]   `json:"labels"`
	Reviews   connection[reviewNode]  `json:"reviews"`
	Comments  connection[commentNode] `json:"comments"`
}

func collaboratorFromAuthorNode(author *authorNode) Collaborator {
	if author == nil {
		return Collaborator{}
	}
	switch author.Typename {
	case botTypename:
		return Collaborator{Login: author.Login + "[bot]"}
	case userTypename:
		return Collaborator{Login: author.Login, Name: author.Name}
	default:
		return Collaborator{Login: author.Login}
	}
}

func hasValidAuthorNode(author *authorNode) bool {
	return author != nil && author.Login != "" && author.Typename != botTypename
}

func timelineCommentFromNode(comment commentNode) TimelineComment {
	return TimelineComment{
		Body:      comment.Body,
		CreatedAt: comment.CreatedAt,
	}
}

func pullRequestFromNode(node pullRequestNode) *PullRequest {
	return &PullRequest{
		Number:    node.Number,
		Title:     node.Title,
		HTMLURL:   node.URL,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
		State:     pullRequestStateFromNodeState(node.State),
		Merged:    node.Merged,
		Draft:     node.IsDraft,
		Labels:    utilities.Map(node.Labels.Nodes, func(label labelNode) string { return label.Name }),
		Author:    collaboratorFromAuthorNode(node.Author),
		MergedAt:  node.MergedAt,
	}
}

// Only the two closed states close a PR, so an unexpected or missing state renders as open
// rather than striking through every PR in the reminder.
func pullRequestStateFromNodeState(nodeState string) string {
	if nodeState == closedNodeState || nodeState == mergedNodeState {
		return closedPullRequestState
	}
	return openPullRequestState
}

func enrichedNode(aliasNode *pullRequestWrapperNode) (pullRequestNode, bool) {
	if aliasNode == nil || aliasNode.PullRequest == nil {
		return pullRequestNode{}, false
	}
	return *aliasNode.PullRequest, true
}

// Reads the reviewer lists and the snooze off a PR's reviews and comments connections.
func prWithReviewers(
	pullRequest *PullRequest, repository models.Repository, node pullRequestNode,
) PR {
	submittedReviews := utilities.Filter(node.Reviews.Nodes, isSubmittedUserReview)
	approvingReviews := utilities.Filter(submittedReviews, isApprovingReviewNode)
	commentsFromUsers := utilities.Filter(node.Comments.Nodes, hasValidCommentAuthor)
	timelineComments := utilities.Map(node.Comments.Nodes, timelineCommentFromNode)

	approvedByUsers, commentedByUsers := deriveReviewers(
		pullRequest.Author.Login,
		utilities.Map(approvingReviews, reviewAuthor),
		utilities.Map(submittedReviews, reviewAuthor),
		utilities.Map(commentsFromUsers, commentAuthor),
	)

	return PR{
		PullRequest:      pullRequest,
		Repository:       repository,
		ApprovedByUsers:  approvedByUsers,
		CommentedByUsers: commentedByUsers,
		SnoozedUntil:     findActiveSnooze(timelineComments),
	}
}

func isSubmittedUserReview(review reviewNode) bool {
	return review.State != pendingReviewState && hasValidAuthorNode(review.Author)
}

func isApprovingReviewNode(review reviewNode) bool {
	return review.State == approvedReviewState
}

func reviewAuthor(review reviewNode) Collaborator {
	return collaboratorFromAuthorNode(review.Author)
}

func hasValidCommentAuthor(comment commentNode) bool {
	return hasValidAuthorNode(comment.Author)
}

func commentAuthor(comment commentNode) Collaborator {
	return collaboratorFromAuthorNode(comment.Author)
}
