package inputhelpers_test

import (
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/config/inputhelpers"
)

func TestReadInput(t *testing.T) {
	t.Setenv("INPUT_TEST", "test_value")
	value := inputhelpers.GetInput("test")
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", value)
	}

	nonSetInputValue := inputhelpers.GetInput("notSet")
	if nonSetInputValue != "" {
		t.Errorf("Expected '', got '%s'", value)
	}
}

func TestReadInputRequired(t *testing.T) {
	_, err := inputhelpers.GetInputRequired("test")
	if err == nil {
		t.Errorf("Expected error for missing required input, got nil")
	}
}

func TestReadInputIntOk(t *testing.T) {
	t.Setenv("INPUT_TEST", "1")
	value, _ := inputhelpers.GetInputInt("test")
	expected := 1
	if value != expected {
		t.Errorf("Expected '%d', got '%v'", expected, value)
	}
}

func TestReadInputIntNotSet(t *testing.T) {
	value, err := inputhelpers.GetInputInt("notSet")
	if err != nil {
		t.Errorf("Expected no error for missing int input, got %v", err)
	}
	expected := 0
	if value != expected {
		t.Errorf("Expected '%d', got '%v'", expected, value)
	}
}

func TestReadInputIntInvalid(t *testing.T) {
	t.Setenv("INPUT_TEST", "a")
	_, err := inputhelpers.GetInputInt("test")
	if err == nil {
		t.Errorf("Expected error for invalid int input, got nil")
	}
	expectedError := "error parsing input test as integer: strconv.Atoi: parsing \"a\": invalid syntax"
	if err.Error() != expectedError {
		t.Errorf("Expected error %v, got '%v'", expectedError, err)
	}
}

func TestGetInputBool(t *testing.T) {
	testCases := []struct {
		name        string
		value       string
		expected    bool
		expectError bool
	}{
		{"lowercase true", "true", true, false},
		{"uppercase TRUE", "TRUE", true, false},
		{"mixed case True", "True", true, false},
		{"number 1", "1", true, false},
		{"lowercase false", "false", false, false},
		{"uppercase FALSE", "FALSE", false, false},
		{"mixed case False", "False", false, false},
		{"number 0", "0", false, false},
		{"not set returns false", "", false, false},
		{"invalid value", "invalid", false, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value != "" {
				t.Setenv("INPUT_TEST", tc.value)
			}
			result, err := inputhelpers.GetInputBool("test")

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
				if result != tc.expected {
					t.Errorf("Expected %v, got %v", tc.expected, result)
				}
			}
		})
	}
}

func TestReadStringMapping(t *testing.T) {
	t.Setenv("INPUT_TEST", "a:b;c:d")
	mapping, _ := inputhelpers.GetInputMapping("test")
	expected := map[string]string{"a": "b", "c": "d"}

	for key, expected := range expected {
		value, exists := (mapping)[key]
		if !exists {
			t.Errorf("Expected key '%s' to exist in mapping", key)
		}
		if value != expected {
			t.Errorf("Expected '%v', got '%v'", expected, value)
		}
	}
}
func TestReadInputMappingInvalid1(t *testing.T) {
	t.Setenv("INPUT_TEST", "a:b;c")
	_, err := inputhelpers.GetInputMapping("test")
	if err == nil {
		t.Errorf("Expected error for invalid mapping input, got nil")
	}
}

func TestReadInputMappingInvalid2(t *testing.T) {
	t.Setenv("INPUT_TEST", " ;a:b;c: ")
	_, err := inputhelpers.GetInputMapping("test")
	if err == nil {
		t.Errorf("Expected error for invalid mapping input, got nil")
	}
}

func TestGetInputOr_NotSetUsesDefault(t *testing.T) {
	value := inputhelpers.GetInputOr("missing", "default-val")
	if value != "default-val" {
		t.Errorf("Expected default value 'default-val', got '%s'", value)
	}
}

func TestGetInputOr_SetUsesValue(t *testing.T) {
	t.Setenv("INPUT_CUSTOM", "present")
	value := inputhelpers.GetInputOr("custom", "default")
	if value != "present" {
		t.Errorf("Expected 'present', got '%s'", value)
	}
}

func TestGetInputOr_ExplicitEmptyOverridesDefault(t *testing.T) {
	// When the env var exists but is empty, should return empty string, not default
	t.Setenv("INPUT_EMPTY", "")
	value := inputhelpers.GetInputOr("empty", "default")
	if value != "" {
		t.Errorf("Expected empty string, got '%s'", value)
	}
}

func TestUnquoteValues(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]string
	}{
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: map[string]string{},
		},
		{
			name: "no quotes",
			input: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "mixed quotes",
			input: map[string]string{
				"repo1": `"prefix1"`,
				"repo2": `'prefix2'`,
				"repo3": "no-quotes",
				"repo4": `"with spaces"`,
			},
			expected: map[string]string{
				"repo1": "prefix1",
				"repo2": "prefix2",
				"repo3": "no-quotes",
				"repo4": "with spaces",
			},
		},
		{
			name: "partial quotes only",
			input: map[string]string{
				"repo1": `"partial`,
				"repo2": `partial"`,
				"repo3": `'partial`,
			},
			expected: map[string]string{
				"repo1": `"partial`,
				"repo2": `partial"`,
				"repo3": `'partial`,
			},
		},
		{
			name: "empty quoted values",
			input: map[string]string{
				"repo1": `""`,
				"repo2": `''`,
			},
			expected: map[string]string{
				"repo1": "",
				"repo2": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inputhelpers.UnquoteValues(tt.input)

			for key, expectedValue := range tt.expected {
				if actualValue, exists := result[key]; !exists {
					t.Errorf("Expected key %q to exist in result", key)
				} else if actualValue != expectedValue {
					t.Errorf("UnquoteValues()[%q] = %q, expected %q", key, actualValue, expectedValue)
				}
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected result to have %d keys, got %d", len(tt.expected), len(result))
			}
		})
	}
}
