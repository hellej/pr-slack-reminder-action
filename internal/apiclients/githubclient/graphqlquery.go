package githubclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
)

const rateLimitField = "rateLimit"

const enrichBatchSize = 25

type graphqlQuery struct {
	text              string
	variables         map[string]any
	aliases           []string
	repositoryByAlias map[string]models.Repository
	referenceByAlias  map[string]models.PullRequestRef // PR queries only
}

func assembleQuery(variableDeclarations, aliasSelections []string, fragment string) string {
	return fmt.Sprintf(
		"query(%s){\n  %s { cost remaining limit }\n%s\n}\n%s",
		strings.Join(variableDeclarations, ","),
		rateLimitField,
		strings.Join(aliasSelections, "\n"),
		fragment,
	)
}

type rateLimitNode struct {
	Cost      int `json:"cost"`
	Remaining int `json:"remaining"`
	Limit     int `json:"limit"`
}

// The aliases are generated, so they are decoded by name rather than by struct field.
type aliasedData[T any] struct {
	RateLimit rateLimitNode
	ByAlias   map[string]*T
}

func (d *aliasedData[T]) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	d.ByAlias = map[string]*T{}
	for field, value := range fields {
		if field == rateLimitField {
			if err := json.Unmarshal(value, &d.RateLimit); err != nil {
				return err
			}
			continue
		}
		var node *T
		if err := json.Unmarshal(value, &node); err != nil {
			return err
		}
		d.ByAlias[field] = node
	}
	return nil
}

func logRateLimit(rateLimit rateLimitNode) {
	log.Printf(
		"GraphQL rate limit: cost %d, remaining %d of %d",
		rateLimit.Cost, rateLimit.Remaining, rateLimit.Limit,
	)
}

// The spread name and the definition are built together so that they cannot drift apart.
type pullRequestFragment struct {
	name string
	text string
}

func newPullRequestFragment(name, selection string) pullRequestFragment {
	return pullRequestFragment{
		name: name,
		text: fmt.Sprintf("fragment %s on PullRequest {\n%s\n}", name, selection),
	}
}

func buildPullRequestsQuery(
	references []models.PullRequestRef, fragment pullRequestFragment,
) graphqlQuery {
	query := graphqlQuery{
		variables:         map[string]any{},
		aliases:           make([]string, len(references)),
		repositoryByAlias: map[string]models.Repository{},
		referenceByAlias:  map[string]models.PullRequestRef{},
	}
	variableDeclarations := make([]string, 0, 3*len(references))
	pullRequestSelections := make([]string, len(references))

	for index, reference := range references {
		alias := fmt.Sprintf("p%d", index)
		ownerVariable := fmt.Sprintf("owner%d", index)
		nameVariable := fmt.Sprintf("name%d", index)
		numberVariable := fmt.Sprintf("num%d", index)

		query.aliases[index] = alias
		query.repositoryByAlias[alias] = reference.Repository
		query.referenceByAlias[alias] = reference
		query.variables[ownerVariable] = reference.Repository.Owner
		query.variables[nameVariable] = reference.Repository.Name
		query.variables[numberVariable] = reference.Number
		variableDeclarations = append(
			variableDeclarations,
			"$"+ownerVariable+":String!", "$"+nameVariable+":String!", "$"+numberVariable+":Int!",
		)
		pullRequestSelections[index] = fmt.Sprintf(
			"  %s: repository(owner:$%s,name:$%s){ pullRequest(number:$%s){ ...%s } }",
			alias, ownerVariable, nameVariable, numberVariable, fragment.name,
		)
	}

	query.text = assembleQuery(variableDeclarations, pullRequestSelections, fragment.text)
	return query
}

type pullRequestWrapperNode struct {
	PullRequest *pullRequestNode `json:"pullRequest"`
}

func errorsForAlias(alias string, hardError error, fieldErrors []fieldError) error {
	aliasErrors := []error{}

	var prError pullRequestError
	if errors.As(hardError, &prError) && prError.alias == alias {
		aliasErrors = append(aliasErrors, prError)
	}
	for _, err := range fieldErrors {
		if err.alias == alias {
			aliasErrors = append(aliasErrors, err)
		}
	}
	return errors.Join(aliasErrors...)
}

var errNoPullRequestReturned = errors.New("no pull request returned")

func logEnrichment(repository models.Repository, number int, node pullRequestNode, err error) {
	if err != nil {
		log.Printf("Unable to fetch reviews/comments for PR #%d: %v", number, err)
		return
	}
	log.Printf(
		"Found %d reviews and %d timeline comments for PR %v/%d",
		len(node.Reviews.Nodes), len(node.Comments.Nodes), repository, number,
	)
}
