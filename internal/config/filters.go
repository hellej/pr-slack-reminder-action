package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/hellej/pr-slack-reminder-action/internal/config/inputhelpers"
)

type Filters struct {
	Authors        []string `json:"authors,omitempty"`
	IgnoredAuthors []string `json:"ignored-authors,omitempty"`
	Labels         []string `json:"labels,omitempty"`
	IgnoredLabels  []string `json:"ignored-labels,omitempty"`
	IgnoredTerms   []string `json:"ignored-terms,omitempty"`
}

func GetGlobalFiltersFromInput(input string) (Filters, error) {
	filters, err := parseFilters(inputhelpers.GetInput(input))
	if err != nil {
		return Filters{}, fmt.Errorf("error reading input %s: %w", input, err)
	}
	return filters, nil
}

func GetRepositoryFiltersFromInput(input string) (map[string]Filters, error) {
	rawFiltersByRepo, err := inputhelpers.GetInputMapping(input)
	if err != nil {
		return nil, fmt.Errorf("error reading input %s: %w", input, err)
	}
	if len(rawFiltersByRepo) == 0 {
		return make(map[string]Filters), nil
	}

	filtersByRepo := make(map[string]Filters, len(rawFiltersByRepo))
	for repo, rawFilters := range rawFiltersByRepo {
		filters, err := parseFilters(rawFilters)
		if err != nil {
			return nil, fmt.Errorf("error parsing filters for repository %s: %w", repo, err)
		}
		filtersByRepo[repo] = filters
	}

	return filtersByRepo, nil
}

func parseFilters(rawFilters string) (Filters, error) {
	if rawFilters == "" {
		return Filters{}, nil
	}

	dec := json.NewDecoder(bytes.NewReader([]byte(rawFilters)))
	dec.DisallowUnknownFields()
	var filters Filters
	err := dec.Decode(&filters)
	if err != nil {
		return Filters{}, fmt.Errorf("unable to parse filters from %v: %w", rawFilters, err)
	}
	err = filters.validate()
	if err != nil {
		return Filters{}, fmt.Errorf("invalid filters: %v, error: %w", rawFilters, err)
	}

	return filters, nil
}

func (f Filters) validate() error {
	if len(f.Authors) > 0 && len(f.IgnoredAuthors) > 0 {
		return fmt.Errorf("cannot use both authors and ignored-authors filters at the same time")
	}

	if slices.ContainsFunc(f.Labels, func(label string) bool {
		return slices.Contains(f.IgnoredLabels, label)
	}) {
		return fmt.Errorf("labels filter cannot contain labels that are in ignored-labels filter")
	}

	if slices.Contains(f.IgnoredTerms, "") {
		return fmt.Errorf("ignored-terms cannot contain empty strings")
	}

	return nil
}
