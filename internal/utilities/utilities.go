package utilities

import (
	"log"
	"os"
	"strconv"
	"strings"
)

func GetEnv(name string, fallback string) string {
	val := os.Getenv(name)
	if val == "" {
		return fallback
	}
	return val
}

// Logs a fatal error if the environment variable named by 'name' is not set,
// and exits the program.
func GetRequiredEnv(name string) string {
	val := os.Getenv(name)
	if val == "" {
		log.Fatalf("Required environment variable not set: %s", name)
	}
	return val
}

// Retrieves the value of the environment variable named by 'name',
// attempts to parse it as an integer, and returns a pointer to the parsed value.
// If the environment variable is not set, it returns nil.
// If parsing fails, the function logs a fatal error and terminates the program.
func GetEnvInt(name string) *int {
	val := os.Getenv(name)
	if val == "" {
		return nil
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		log.Fatalf("Error parsing environment variable %s: %v", name, err)
	}
	return &parsed
}

func GetEnvIntOr(name string, fallBack int) int {
	val := os.Getenv(name)
	if val == "" {
		return fallBack
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		log.Fatalf("Error parsing environment variable %s: %v", name, err)
	}
	return parsed
}

func GetStringMapping(name string) *map[string]string {
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
