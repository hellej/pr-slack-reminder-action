package githubclient

import (
	"slices"
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

const (
	closedNodeState = "CLOSED"
	mergedNodeState = "MERGED"
)

const approvedReviewState = "APPROVED"

const changesRequestedReviewState = "CHANGES_REQUESTED"

const conflictingMergeableState = "CONFLICTING"

const pendingReviewState = "PENDING"

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

// A review thread carries no author of its own, so its last comment supplies the one that
// decides whose turn the thread is.
type reviewThreadNode struct {
	IsResolved bool                    `json:"isResolved"`
	Comments   connection[commentNode] `json:"comments"`
}

// A requestedReviewer can be null. See githubclient.spec.md § Behaviour
type reviewRequestNode struct {
	RequestedReviewer *authorNode `json:"requestedReviewer"`
}

type readyForReviewEventNode struct {
	CreatedAt time.Time `json:"createdAt"`
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

	Mergeable      string                        `json:"mergeable"`
	ReviewThreads  connection[reviewThreadNode]  `json:"reviewThreads"`
	ReviewRequests connection[reviewRequestNode] `json:"reviewRequests"`

	// Holds the earliest event only, read with first: 1.
	// See docs/third-party-facts.md § `timelineItems` filters `itemTypes` before paging and returns events oldest first, measured only
	ReadyForReviewEvents connection[readyForReviewEventNode] `json:"timelineItems"`
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

func hasKnownNonBotAuthorNode(author *authorNode) bool {
	return !isUnknownAuthorNode(author) && author.Typename != botTypename
}

func isUnknownAuthorNode(author *authorNode) bool {
	return author == nil || author.Login == ""
}

func isBotAuthorNode(author *authorNode) bool {
	return !isUnknownAuthorNode(author) && author.Typename == botTypename
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
		HasThreadWaitingForAuthor: hasThreadWaitingForPRAuthor(
			node.ReviewThreads.Nodes, pullRequest.Author,
		),
		Conflicting: node.Mergeable == conflictingMergeableState,
		HasOutstandingChangesRequest: hasOutstandingChangesRequest(
			submittedReviews, node.ReviewRequests.Nodes, pullRequest.Author,
		),
		FirstReadyForReviewEventAt: firstReadyForReviewEventAt(node),
	}
}

func firstReadyForReviewEventAt(node pullRequestNode) *time.Time {
	if len(node.ReadyForReviewEvents.Nodes) == 0 {
		return nil
	}
	return &node.ReadyForReviewEvents.Nodes[0].CreatedAt
}

func hasThreadWaitingForPRAuthor(threads []reviewThreadNode, prAuthor Collaborator) bool {
	return slices.ContainsFunc(threads, func(thread reviewThreadNode) bool {
		return !thread.IsResolved && isWaitingForPRAuthor(thread, prAuthor)
	})
}

func isWaitingForPRAuthor(thread reviewThreadNode, prAuthor Collaborator) bool {
	comments := thread.Comments.Nodes
	if len(comments) == 0 {
		return true
	}

	lastCommentAuthor := comments[len(comments)-1].Author
	if isUnknownAuthorNode(lastCommentAuthor) {
		return true
	}
	if isBotAuthorNode(lastCommentAuthor) {
		return false
	}
	// The PR author's login came through collaboratorFromAuthorNode as well, so both sides
	// spell the same account the same way.
	return collaboratorFromAuthorNode(lastCommentAuthor).Login != prAuthor.Login
}

// Compares logins: the selection reads a name off a review's author, not off a requested reviewer.
func hasOutstandingChangesRequest(
	submittedReviews []reviewNode, reviewRequests []reviewRequestNode, prAuthor Collaborator,
) bool {
	requestedLogins := utilities.Map(reviewRequests, requestedReviewerLogin)
	return slices.ContainsFunc(submittedReviews, func(review reviewNode) bool {
		reviewerLogin := reviewAuthor(review).Login
		return review.State == changesRequestedReviewState &&
			reviewerLogin != prAuthor.Login &&
			!slices.Contains(requestedLogins, reviewerLogin)
	})
}

func isSubmittedUserReview(review reviewNode) bool {
	return review.State != pendingReviewState && hasKnownNonBotAuthorNode(review.Author)
}

func isApprovingReviewNode(review reviewNode) bool {
	return review.State == approvedReviewState
}

func reviewAuthor(review reviewNode) Collaborator {
	return collaboratorFromAuthorNode(review.Author)
}

func requestedReviewerLogin(request reviewRequestNode) string {
	return collaboratorFromAuthorNode(request.RequestedReviewer).Login
}

func hasValidCommentAuthor(comment commentNode) bool {
	return hasKnownNonBotAuthorNode(comment.Author)
}

func commentAuthor(comment commentNode) Collaborator {
	return collaboratorFromAuthorNode(comment.Author)
}
