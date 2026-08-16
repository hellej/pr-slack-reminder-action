package githubclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const openPullRequestState = "open"

const rateLimitField = "rateLimit"

// labels are capped at GitHub's maximum page size; REST returned them unpaginated.
const openPullRequestsFragment = `fragment prs on Repository {
  pullRequests(states: OPEN, first: 100, orderBy: {field: CREATED_AT, direction: DESC}) {
    nodes {
      number title url isDraft createdAt updatedAt headRefOid
      author { login __typename ... on User { name } }
      labels(first: 100) { nodes { name } }
    }
  }
}`

type graphqlQuery struct {
	text              string
	variables         map[string]any
	aliases           []string
	repositoryByAlias map[string]models.Repository
}

// Aliases cannot be variables, so they are generated into the query text; owner and name are
// always passed as variables.
func buildListOpenPRsQuery(repositories []models.Repository) graphqlQuery {
	query := graphqlQuery{
		variables:         map[string]any{},
		aliases:           make([]string, len(repositories)),
		repositoryByAlias: map[string]models.Repository{},
	}
	variableDeclarations := make([]string, 0, 2*len(repositories))
	repositorySelections := make([]string, len(repositories))

	for index, repository := range repositories {
		alias := fmt.Sprintf("r%d", index)
		ownerVariable := fmt.Sprintf("owner%d", index)
		nameVariable := fmt.Sprintf("name%d", index)

		query.aliases[index] = alias
		query.repositoryByAlias[alias] = repository
		query.variables[ownerVariable] = repository.Owner
		query.variables[nameVariable] = repository.Name
		variableDeclarations = append(
			variableDeclarations, "$"+ownerVariable+":String!", "$"+nameVariable+":String!",
		)
		repositorySelections[index] = fmt.Sprintf(
			"  %s: repository(owner:$%s,name:$%s){ ...prs }", alias, ownerVariable, nameVariable,
		)
	}

	query.text = fmt.Sprintf(
		"query(%s){\n  %s { cost remaining limit }\n%s\n}\n%s",
		strings.Join(variableDeclarations, ","),
		rateLimitField,
		strings.Join(repositorySelections, "\n"),
		openPullRequestsFragment,
	)
	return query
}

type rateLimitNode struct {
	Cost      int `json:"cost"`
	Remaining int `json:"remaining"`
	Limit     int `json:"limit"`
}

type repositoryPullRequestsNode struct {
	PullRequests connection[pullRequestNode] `json:"pullRequests"`
}

// The repository aliases are generated, so they are decoded by name rather than by struct field.
type listOpenPRsData struct {
	RateLimit         rateLimitNode
	RepositoryByAlias map[string]*repositoryPullRequestsNode
}

func (d *listOpenPRsData) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	d.RepositoryByAlias = map[string]*repositoryPullRequestsNode{}
	for field, value := range fields {
		if field == rateLimitField {
			if err := json.Unmarshal(value, &d.RateLimit); err != nil {
				return err
			}
			continue
		}
		var repository *repositoryPullRequestsNode
		if err := json.Unmarshal(value, &repository); err != nil {
			return err
		}
		d.RepositoryByAlias[field] = repository
	}
	return nil
}

func (c *client) listOpenPRs(
	ctx context.Context, repositories []models.Repository,
) ([]PRResult, error) {
	query := buildListOpenPRsQuery(repositories)

	callCtx, cancel := context.WithTimeout(ctx, PullRequestListTimeout)
	defer cancel()

	var data listOpenPRsData
	fieldErrors, err := c.graphql.Do(callCtx, query.text, query.variables, query.aliases, &data)
	if err != nil {
		return nil, listOpenPRsError(err, query.repositoryByAlias)
	}
	// A field error here loses every PR of one repository, which has always failed the whole list.
	if len(fieldErrors) > 0 {
		return nil, repositoryFetchError(query.repositoryByAlias[fieldErrors[0].alias], fieldErrors[0])
	}
	logRateLimit(data.RateLimit)

	prResults := []PRResult{}
	for _, alias := range query.aliases {
		repository := query.repositoryByAlias[alias]
		node := data.RepositoryByAlias[alias]
		if node == nil {
			return nil, repositoryFetchError(repository, errors.New("no data returned"))
		}
		prResults = append(
			prResults,
			utilities.Map(node.PullRequests.Nodes, openPRResultMapper(repository))...,
		)
	}
	return prResults, nil
}

func listOpenPRsError(err error, repositoryByAlias map[string]models.Repository) error {
	var repoError repositoryError
	if errors.As(err, &repoError) {
		if repository, isKnownAlias := repositoryByAlias[repoError.alias]; isKnownAlias {
			return fmt.Errorf(
				"repository %s/%s not found - check the repository name and permissions",
				repository.Owner,
				repository.Name,
			)
		}
	}
	return fmt.Errorf("error fetching pull requests: %w", err)
}

func repositoryFetchError(repository models.Repository, err error) error {
	return fmt.Errorf(
		"error fetching pull requests from %s/%s: %w", repository.Owner, repository.Name, err,
	)
}

func logRateLimit(rateLimit rateLimitNode) {
	log.Printf(
		"GraphQL rate limit: cost %d, remaining %d of %d",
		rateLimit.Cost, rateLimit.Remaining, rateLimit.Limit,
	)
}

func openPRResultMapper(repository models.Repository) func(node pullRequestNode) PRResult {
	return func(node pullRequestNode) PRResult {
		return PRResult{pr: openPullRequestFromNode(node), repository: repository}
	}
}

// states: OPEN guarantees the state, which is therefore not selected.
func openPullRequestFromNode(node pullRequestNode) *PullRequest {
	return &PullRequest{
		Number:    node.Number,
		Title:     node.Title,
		HTMLURL:   node.URL,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
		State:     openPullRequestState,
		Merged:    false,
		Draft:     node.IsDraft,
		Labels:    utilities.Map(node.Labels.Nodes, func(label labelNode) string { return label.Name }),
		Author:    collaboratorFromAuthorNode(node.Author),
		HeadSHA:   node.HeadRefOID,
	}
}
