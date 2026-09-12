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

// GraphQL states of a PR node. CLOSED and MERGED are both "closed" to the pipeline.
const (
	closedNodeState = "CLOSED"
	mergedNodeState = "MERGED"
)

const approvedReviewState = "APPROVED"

// The review states that ask the PR author for something. DISMISSED is left out: such a review
// has been withdrawn and no longer blocks.
const (
	commentedReviewState        = "COMMENTED"
	changesRequestedReviewState = "CHANGES_REQUESTED"
)

// The one mergeable state that cannot be merged. UNKNOWN is GitHub still computing
// mergeability, and reads as not conflicting.
const conflictingMergeableState = "CONFLICTING"

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

// A review thread carries no author of its own, so its last comment supplies the one that
// decides whose turn the thread is.
type reviewThreadNode struct {
	IsResolved bool                    `json:"isResolved"`
	Comments   connection[commentNode] `json:"comments"`
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

	Mergeable     string                       `json:"mergeable"`
	ReviewThreads connection[reviewThreadNode] `json:"reviewThreads"`
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
	return !isUnknownAuthorNode(author) && author.Typename != botTypename
}

// GitHub reports no author at all for a deleted account, and a node without a login says as
// little, so the two are one unknown to the pipeline.
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
		HasThreadWaitingForAuthor: hasThreadWaitingForPRAuthor(
			node.ReviewThreads.Nodes, pullRequest.Author,
		),
		Conflicting:           node.Mergeable == conflictingMergeableState,
		HasNonApprovingReview: hasNonApprovingReview(submittedReviews, pullRequest.Author),
	}
}

func hasThreadWaitingForPRAuthor(threads []reviewThreadNode, prAuthor Collaborator) bool {
	return slices.ContainsFunc(threads, func(thread reviewThreadNode) bool {
		return !thread.IsResolved && isWaitingForPRAuthor(thread, prAuthor)
	})
}

// An unresolved thread's last comment says whose turn it is. The author replying hands it back
// to the reviewer, and an author's own note on their own diff never waits on anyone. A bot's
// thread waits on nobody either: with a review bot enabled, every new PR would otherwise arrive
// already waiting on its author. A thread nobody has commented on, or one whose last commenter
// GitHub no longer reports, is left waiting rather than assumed answered.
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

// The author's own reviews are left out: a bare inline comment on one's own diff arrives as a
// COMMENTED review. The given reviews are the submitted ones, so bots are already out, keeping
// a review bot's comment from handing the PR back to its author.
func hasNonApprovingReview(submittedReviews []reviewNode, prAuthor Collaborator) bool {
	return slices.ContainsFunc(submittedReviews, func(review reviewNode) bool {
		return isNonApprovingReviewState(review.State) &&
			collaboratorFromAuthorNode(review.Author).Login != prAuthor.Login
	})
}

func isNonApprovingReviewState(state string) bool {
	return state == commentedReviewState || state == changesRequestedReviewState
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
