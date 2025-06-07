package utilities_test

import (
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/config/utilities"
)

func TestReadInput(t *testing.T) {
	t.Setenv("INPUT_TEST", "test_value")
	value := utilities.GetInput("test")
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", value)
	}

	nonSetInputValue := utilities.GetInput("notSet")
	if nonSetInputValue != "" {
		t.Errorf("Expected '', got '%s'", value)
	}
}

func TestReadInputRequired(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Log("Test passed, panic was caught!")
		}
	}()
	utilities.GetInputRequired("test")
	t.Errorf("Test failed, panic was expected")
}

func TestReadInputIntOk(t *testing.T) {
	t.Setenv("INPUT_TEST", "1")
	value := utilities.GetInputInt("test")
	expected := 1
	if *value != expected {
		t.Errorf("Expected '%d', got '%v'", expected, value)
	}
}

func TestReadInputIntInvalid(t *testing.T) {
	t.Setenv("INPUT_TEST", "a")

	defer func() {
		if r := recover(); r != nil {
			t.Log("Test passed, panic was caught!")
		}
	}()

	utilities.GetInputInt("test")
	t.Errorf("Test failed, panic was expected")
}

func TestReadStringMapping(t *testing.T) {
	t.Setenv("INPUT_TEST", "a:b;c:d")
	mapping := utilities.GetInputMapping("test")
	expected := map[string]string{"a": "b", "c": "d"}

	for key, expected := range expected {
		value, exists := (*mapping)[key]
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

	defer func() {
		if r := recover(); r != nil {
			t.Log("Test passed, panic was caught!")
		}
	}()

	utilities.GetInputMapping("test")
	t.Errorf("Test failed, panic was expected")
}

func TestReadInputMappingInvalid2(t *testing.T) {
	t.Setenv("INPUT_TEST", " ;a:b;c: ")

	defer func() {
		if r := recover(); r != nil {
			t.Log("Test passed, panic was caught!")
		}
	}()

	utilities.GetInputMapping("test")
	t.Errorf("Test failed, panic was expected")
}
