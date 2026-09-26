package testhelpers

import (
	"bytes"
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

func AsPointer[T any](v T) *T {
	return &v
}

func RandomPositiveInt() int {
	return seededRand.Intn(100_000) + 1 // Ensures a positive integer
}

func RandomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

const charset = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

func LoadJSONFromFile[T any](filePath string, target *T) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, target); err != nil {
		return err
	}

	return nil
}

// CaptureLog sends the standard logger's output to the returned buffer, with main's flags, until
// the test ends.
func CaptureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	originalOutput, originalFlags := log.Writer(), log.Flags()
	t.Cleanup(func() {
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
	})
	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	log.SetFlags(0)
	return &logOutput
}

func LogLinesStartingWith(logOutput *bytes.Buffer, prefix string) []string {
	return utilities.Filter(strings.Split(logOutput.String(), "\n"), func(line string) bool {
		return strings.HasPrefix(line, prefix)
	})
}
