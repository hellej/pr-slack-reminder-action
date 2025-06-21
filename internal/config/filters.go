package config

import (
	"encoding/json"
	"fmt"

	"github.com/hellej/pr-slack-reminder-action/internal/config/utilities"
)

type Filters struct {
	Authors      []string `json:"authors,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	LabelsIgnore []string `json:"labels-ignore,omitempty"`
}

func GetFiltersFromInput(input string) (Filters, error) {
	rawFilters := utilities.GetInput(input)
	if rawFilters == "" {
		return Filters{}, nil
	}

	var filters Filters
	err := json.Unmarshal([]byte(rawFilters), &filters)
	if err != nil {
		return Filters{}, fmt.Errorf("unable to parse %v from %v: %v", input, rawFilters, err)
	}

	return filters, nil
}
