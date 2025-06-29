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

func GetFiltersFromInput(input string) (Filters, error) {
	rawFilters := utilities.GetInput(input)
	if rawFilters == "" {
		return Filters{}, nil
	}

	dec := json.NewDecoder(bytes.NewReader([]byte(rawFilters)))
	dec.DisallowUnknownFields()
	var filters Filters
	err := dec.Decode(&filters)
	if err != nil {
		return Filters{}, fmt.Errorf("unable to parse %v from %v: %v", input, rawFilters, err)
	}

	return filters, nil
}
