package githubclient

import (
	"cmp"
	"slices"
	"time"

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
	// Head commit date when known, the update time as a fallback, nil when neither is known.
	LastActivityAt *time.Time
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

func (p *PullRequest) GetLastActivityAt() *time.Time {
	if p == nil {
		return nil
	}
	return p.LastActivityAt
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

type PR struct {
	*PullRequest
	Repository       models.Repository
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

type Collaborator struct {
	Login string // GitHub username
	Name  string // GitHub name if available
}

type TimelineComment struct {
	Body      string
	CreatedAt time.Time
}

// Returns the GitHub name if available, otherwise login.
func (c Collaborator) GetGitHubName() string {
	return cmp.Or(c.Name, c.Login)
}

func deriveReviewers(
	authorLogin string,
	approvers, reviewAuthors, timelineCommenters []Collaborator,
) (approvedBy, commentedBy []Collaborator) {
	approvedBy = utilities.UniqueFunc(approvers, isUniqueCollaborator)

	allCommenters := slices.Concat(reviewAuthors, timelineCommenters)
	commentedBy = utilities.Filter(
		utilities.UniqueFunc(allCommenters, isUniqueCollaborator),
		getFilterForCommenters(authorLogin, approvedBy),
	)
	return approvedBy, commentedBy
}

func isUniqueCollaborator(a, b Collaborator) bool {
	return a.Login == b.Login
}

func getFilterForCommenters(authorLogin string, approvedByUsers []Collaborator) func(c Collaborator) bool {
	return func(c Collaborator) bool {
		return c.Login != authorLogin &&
			!slices.ContainsFunc(approvedByUsers, func(approver Collaborator) bool {
				return c.Login == approver.Login
			})
	}
}
