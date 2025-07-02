package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/hellej/pr-slack-reminder-action/internal/config/utilities"
)

type Filters struct {
	Authors       []string `json:"authors,omitempty"`
	AuthorsIgnore []string `json:"authors-ignore,omitempty"`
	Labels        []string `json:"labels,omitempty"`
	LabelsIgnore  []string `json:"labels-ignore,omitempty"`
}

func (f Filters) validate() error {
	if len(f.Authors) > 0 && len(f.AuthorsIgnore) > 0 {
		return fmt.Errorf("cannot use both authors and authors-ignore filters at the same time")
	}
	return nil
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
		return Filters{}, fmt.Errorf("unable to parse filters from %v: %v", rawFilters, err)
	}
	err = filters.validate()
	if err != nil {
		return Filters{}, fmt.Errorf("invalid filters: %v, error: %v", rawFilters, err)
	}

	return filters, nil
}

func GetGlobalFiltersFromInput(input string) (Filters, error) {
	filters, err := parseFilters(utilities.GetInput(input))
	if err != nil {
		return Filters{}, fmt.Errorf("error reading input %s: %w", input, err)
	}
	return filters, nil
}

func GetRepositoryFiltersFromInput(input string) (map[string]Filters, error) {
	rawFiltersByRepo, err := utilities.GetInputMapping(input)
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
