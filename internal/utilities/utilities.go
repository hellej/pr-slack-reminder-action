package utilities

import (
	"log"
	"os"
	"strconv"
	"strings"
)

func inputNameAsEnv(name string) string {
	e := strings.ReplaceAll(name, " ", "_")
	e = strings.ToUpper(e)
	return "INPUT_" + e
}

func raiseIfEmpty(value string, name string) string {
	if value == "" {
		log.Fatalf("Required input %s is not set", name)
	}
	return value
}

func GetEnv(name string) string {
	return os.Getenv(name)
}

func GetEnvRequired(name string) string {
	return raiseIfEmpty(os.Getenv(name), name)
}

func GetInput(name string) string {
	return strings.TrimSpace(GetEnv((inputNameAsEnv(name))))
}

func GetInputRequired(name string) string {
	return raiseIfEmpty(GetInput(name), name)
}

// Retrieves the value of the input, attempts to parse it as an integer,
// and returns a pointer to the parsed value.
// Returns nil if the environment variable is not set.
func GetInputInt(name string) *int {
	val := GetInput(name)
	if val == "" {
		return nil
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		log.Fatalf("Error parsing environment variable %s: %v", name, err)
	}
	return &parsed
}

func GetStringMapping(inputName string) *map[string]string {
	name := inputNameAsEnv(inputName)
	mapping := make(map[string]string)
	val := os.Getenv(name)
	if val == "" {
		return &mapping
	}

	lines := strings.Split(val, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			log.Fatalf("Invalid mapping format for %s: %s", name, line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" || value == "" {
			log.Fatalf("Invalid mapping key or value for %s: %s", name, line)
		}
		mapping[key] = value
	}

	return &mapping
}
