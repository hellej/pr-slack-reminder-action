package utilities

import (
	"log"
	"os"
	"strconv"
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
