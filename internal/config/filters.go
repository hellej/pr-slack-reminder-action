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
	err = filters.validate()
	if err != nil {
		return Filters{}, fmt.Errorf("invalid value in input: %v, error: %v", input, err)
	}

	return filters, nil
}
