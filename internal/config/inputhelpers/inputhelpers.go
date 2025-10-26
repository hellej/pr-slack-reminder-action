// Package utilities provides helper functions for parsing GitHub Action inputs
// from environment variables. It handles input name conversion, type parsing,
// and structured data extraction from string inputs.
package inputhelpers

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func inputNameAsEnv(name string) string {
	e := strings.ReplaceAll(name, " ", "_")
	e = strings.ToUpper(e)
	return "INPUT_" + e
}

func withErrorIfEmpty(value string, name string) (string, error) {
	if value != "" {
		return value, nil
	}
	return value, fmt.Errorf("required input %s is not set", name)
}

func GetEnv(name string) string {
	return os.Getenv(name)
}

func GetEnvRequired(name string) (string, error) {
	return withErrorIfEmpty(os.Getenv(name), name)
}

func GetInput(name string) string {
	return strings.TrimSpace(GetEnv((inputNameAsEnv(name))))
}

// GetInputOr returns the input value if set, otherwise returns the provided default.
// An explicitly set empty string overrides the default (returns empty).
func GetInputOr(name string, defaultValue string) string {
	val := GetInput(name)
	if val == "" {
		// Distinguish between unset and intentionally empty:
		// If the variable name exists in the environment but is empty, empty it is.
		if _, exists := os.LookupEnv(inputNameAsEnv(name)); exists {
			return ""
		}
		return defaultValue
	}
	return val
}

func GetInputRequired(name string) (string, error) {
	return withErrorIfEmpty(GetInput(name), name)
}

// Retrieves the value of the input, attempts to parse it as an integer,
// and returns the parsed value.
// Returns 0 if the environment variable is not set.
func GetInputInt(name string) (int, error) {
	val := GetInput(name)
	if val == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("error parsing input %s as integer: %v", name, err)
	}
	return parsed, nil
}

// Retrieves the value of the input, attempts to parse it as a boolean,
// and returns the parsed value.
// Returns false if the environment variable is not set.
func GetInputBool(name string) (bool, error) {
	val := GetInput(name)
	if val == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return false, fmt.Errorf("error parsing input %s as boolean: %v", name, err)
	}
	return parsed, nil
}

func GetInputList(name string) []string {
	val := GetInput(name)
	if val == "" {
		return []string{}
	}
	separator := "\n"
	if strings.Contains(val, ";") {
		separator = ";" // for more convenient local testing
	}
	lines := strings.Split(val, separator)
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return lines
}

func GetInputMapping(inputName string) (map[string]string, error) {
	name := inputNameAsEnv(inputName)
	mapping := make(map[string]string)
	val := os.Getenv(name)
	if val == "" {
		return mapping, nil
	}
	separator := "\n"
	if strings.Contains(val, ";") {
		// for more convenient local testing
		separator = ";"
	}
	lines := strings.Split(val, separator)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid mapping format for %s: '%s'", inputName, line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" || value == "" {
			return nil, fmt.Errorf("invalid mapping key or value for %s: '%s'", inputName, line)
		}
		mapping[key] = value
	}

	return mapping, nil
}

func removeLeadingAndTrailingQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// removes leading and trailing quotes from all values in the provided map.
func UnquoteValues(m map[string]string) map[string]string {
	result := make(map[string]string, len(m))
	for key, value := range m {
		result[key] = removeLeadingAndTrailingQuotes(value)
	}
	return result
}
