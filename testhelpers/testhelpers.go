package testhelpers

import (
	"encoding/json"
	"math/rand"
	"os"
	"time"
)

func AsPointer[T any](v T) *T {
	return &v
}

func RandomPositiveInt() int {
	return seededRand.Intn(100_000) + 1 // Ensures a positive integer
}

func RandomStringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func RandomString(length int) string {
	return RandomStringWithCharset(length, charset)
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
