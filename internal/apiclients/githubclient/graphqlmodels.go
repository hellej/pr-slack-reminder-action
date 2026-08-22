package githubclient

import "time"

const (
	botTypename  = "Bot"
	userTypename = "User"
)

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

type commitNode struct {
	Commit struct {
		OID           string    `json:"oid"`
		CommittedDate time.Time `json:"committedDate"`
	} `json:"commit"`
}

type connection[T any] struct {
	Nodes []T `json:"nodes"`
}

type pullRequestNode struct {
	Number     int                     `json:"number"`
	Title      string                  `json:"title"`
	URL        string                  `json:"url"`
	IsDraft    bool                    `json:"isDraft"`
	CreatedAt  time.Time               `json:"createdAt"`
	UpdatedAt  time.Time               `json:"updatedAt"`
	MergedAt   *time.Time              `json:"mergedAt"`
	HeadRefOID string                  `json:"headRefOid"`
	State      string                  `json:"state"`
	Merged     bool                    `json:"merged"`
	Author     *authorNode             `json:"author"`
	Labels     connection[labelNode]   `json:"labels"`
	Commits    connection[commitNode]  `json:"commits"`
	Reviews    connection[reviewNode]  `json:"reviews"`
	Comments   connection[commentNode] `json:"comments"`
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
