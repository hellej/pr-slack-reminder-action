package utilities

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

func GetInputRequired(name string) (string, error) {
	return withErrorIfEmpty(GetInput(name), name)
}

// Retrieves the value of the input, attempts to parse it as an integer,
// and returns a pointer to the parsed value.
// Returns nil if the environment variable is not set.
func GetInputInt(name string) (*int, error) {
	val := GetInput(name)
	if val == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return nil, fmt.Errorf("error parsing input %s as integer: %v", name, err)
	}
	return &parsed, nil
}

func GetInputList(name string) []string {
	val := GetInput(name)
	if val == "" {
		return []string{}
	}
	separator := "\n"
	if strings.Contains(val, ";") {
		// for more convenient local testing
		separator = ";"
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
		parts := strings.SplitN(line, ":", -1)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid mapping format for %s: %s", inputName, line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" || value == "" {
			return nil, fmt.Errorf("invalid mapping key or value for %s: %s", inputName, line)
		}
		mapping[key] = value
	}

	return mapping, nil
}
