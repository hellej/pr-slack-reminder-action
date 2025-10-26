package utilities

import (
	"slices"
	"strconv"
	"testing"
)

func TestFilter(t *testing.T) {
	tests := []struct {
		name     string
		items    []int
		filter   func(int) bool
		expected []int
	}{
		{
			name:     "filter even numbers",
			items:    []int{1, 2, 3, 4, 5, 6},
			filter:   func(n int) bool { return n%2 == 0 },
			expected: []int{2, 4, 6},
		},
		{
			name:     "filter greater than 3",
			items:    []int{1, 2, 3, 4, 5},
			filter:   func(n int) bool { return n > 3 },
			expected: []int{4, 5},
		},
		{
			name:     "no items match filter",
			items:    []int{1, 3, 5},
			filter:   func(n int) bool { return n%2 == 0 },
			expected: []int{},
		},
		{
			name:     "all items match filter",
			items:    []int{2, 4, 6},
			filter:   func(n int) bool { return n%2 == 0 },
			expected: []int{2, 4, 6},
		},
		{
			name:     "empty slice",
			items:    []int{},
			filter:   func(n int) bool { return true },
			expected: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Filter(tt.items, tt.filter)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("Filter() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestFilterWithStrings(t *testing.T) {
	items := []string{"apple", "banana", "apricot", "cherry"}
	filter := func(s string) bool { return len(s) > 5 }

	result := Filter(items, filter)
	expected := []string{"banana", "apricot", "cherry"}

	if !slices.Equal(result, expected) {
		t.Errorf("Filter() = %v, expected %v", result, expected)
	}
}

func TestFind(t *testing.T) {
	items := []string{"apple", "banana", "apricot", "cherry"}
	predicate := func(s string) bool { return s == "apricot" }

	result, found := Find(items, predicate)
	expected := "apricot"

	if !found {
		t.Error("Find() did not find the item, but it should have")
	}

	if result != expected {
		t.Errorf("Find() = %v, expected %v", result, expected)
	}
}

func TestFindMissing(t *testing.T) {
	items := []string{"apple", "banana", "cherry"}
	predicate := func(s string) bool { return s == "apricot" }

	result, found := Find(items, predicate)
	expected := ""

	if found {
		t.Error("Find() found the item, but it should not have")
	}

	if result != expected {
		t.Errorf("Find() = %v, expected %v", result, expected)
	}
}

func TestMap(t *testing.T) {
	items := []string{"apple", "banana", "apricot", "cherry"}
	mapper := func(s string) int { return len(s) }

	result := Map(items, mapper)
	expected := []int{5, 6, 7, 6}

	if !slices.Equal(result, expected) {
		t.Errorf("Map() = %v, expected %v", result, expected)
	}
}

func TestMapWithError_Success(t *testing.T) {
	items := []string{"1", "2", "3", "4"}
	mapper := func(s string) (int, error) {
		return strconv.Atoi(s)
	}

	result, err := MapWithError(items, mapper)
	expected := []int{1, 2, 3, 4}

	if err != nil {
		t.Errorf("MapWithError() returned unexpected error: %v", err)
	}

	if !slices.Equal(result, expected) {
		t.Errorf("MapWithError() = %v, expected %v", result, expected)
	}
}

func TestMapWithError_WithError(t *testing.T) {
	items := []string{"1", "invalid", "3", "4"}
	mapper := func(s string) (int, error) {
		return strconv.Atoi(s)
	}

	result, err := MapWithError(items, mapper)
	expected := []int{1} // Should only have processed first item before error

	if err == nil {
		t.Error("MapWithError() should have returned an error")
	}

	if !slices.Equal(result, expected) {
		t.Errorf("MapWithError() = %v, expected %v", result, expected)
	}
}

func TestMapWithError_EmptySlice(t *testing.T) {
	items := []string{}
	mapper := func(s string) (int, error) {
		return strconv.Atoi(s)
	}

	result, err := MapWithError(items, mapper)
	expected := []int{}

	if err != nil {
		t.Errorf("MapWithError() returned unexpected error: %v", err)
	}

	if !slices.Equal(result, expected) {
		t.Errorf("MapWithError() = %v, expected %v", result, expected)
	}
}

func TestMapWithErrorToIter(t *testing.T) {
	items := []string{"1", "invalid", "3"}
	mapper := func(s string) (int, error) {
		return strconv.Atoi(s)
	}

	var results []int
	var errors []error

	for value, err := range MapWithErrorToIter(items, mapper) {
		results = append(results, value)
		errors = append(errors, err)

		// Iterator stops at first error by design
		if err != nil {
			break
		}
	}

	expectedResults := []int{1, 0} // 0 is zero value for int when error occurs
	if !slices.Equal(results, expectedResults) {
		t.Errorf("MapWithErrorToIter() values = %v, expected %v", results, expectedResults)
	}

	if len(errors) != 2 {
		t.Errorf("MapWithErrorToIter() should have yielded 2 items, got %d", len(errors))
	}

	if errors[0] != nil {
		t.Errorf("First error should be nil, got %v", errors[0])
	}

	if errors[1] == nil {
		t.Error("Second error should not be nil")
	}
}

func TestUnique(t *testing.T) {
	tests := []struct {
		name     string
		items    []int
		expected []int
	}{
		{
			name:     "remove duplicates",
			items:    []int{1, 2, 2, 3, 1, 4, 3},
			expected: []int{1, 2, 3, 4},
		},
		{
			name:     "no duplicates",
			items:    []int{1, 2, 3, 4},
			expected: []int{1, 2, 3, 4},
		},
		{
			name:     "all same",
			items:    []int{5, 5, 5, 5},
			expected: []int{5},
		},
		{
			name:     "empty slice",
			items:    []int{},
			expected: nil,
		},
		{
			name:     "single element",
			items:    []int{42},
			expected: []int{42},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Unique(tt.items)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("Unique() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestUniqueFunc(t *testing.T) {
	type Person struct {
		Name string
		Age  int
	}

	tests := []struct {
		name     string
		items    []Person
		equal    func(Person, Person) bool
		expected []Person
	}{
		{
			name: "unique by name",
			items: []Person{
				{"Alice", 25},
				{"Bob", 30},
				{"Alice", 26}, // different age, same name
				{"Charlie", 35},
			},
			equal: func(a, b Person) bool { return a.Name == b.Name },
			expected: []Person{
				{"Alice", 25}, // first occurrence kept
				{"Bob", 30},
				{"Charlie", 35},
			},
		},
		{
			name: "unique by age",
			items: []Person{
				{"Alice", 25},
				{"Bob", 30},
				{"Charlie", 25}, // same age as Alice
				{"David", 40},
			},
			equal: func(a, b Person) bool { return a.Age == b.Age },
			expected: []Person{
				{"Alice", 25}, // first occurrence kept
				{"Bob", 30},
				{"David", 40},
			},
		},
		{
			name:     "empty slice",
			items:    []Person{},
			equal:    func(a, b Person) bool { return a.Name == b.Name },
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UniqueFunc(tt.items, tt.equal)
			if len(result) != len(tt.expected) {
				t.Errorf("UniqueFunc() length = %d, expected %d", len(result), len(tt.expected))
				return
			}
			for i, person := range result {
				if person != tt.expected[i] {
					t.Errorf("UniqueFunc() result[%d] = %v, expected %v", i, person, tt.expected[i])
				}
			}
		})
	}
}

func TestFlatMap(t *testing.T) {
	tests := []struct {
		name     string
		items    [][]int
		expected []int
	}{
		{
			name:     "flatten multiple slices",
			items:    [][]int{{1, 2}, {3, 4}, {5, 6}},
			expected: []int{1, 2, 3, 4, 5, 6},
		},
		{
			name:     "flatten with empty slices",
			items:    [][]int{{1, 2}, {}, {3, 4}},
			expected: []int{1, 2, 3, 4},
		},
		{
			name:     "flatten single slice",
			items:    [][]int{{1, 2, 3}},
			expected: []int{1, 2, 3},
		},
		{
			name:     "flatten all empty slices",
			items:    [][]int{{}, {}, {}},
			expected: []int{},
		},
		{
			name:     "flatten empty input",
			items:    [][]int{},
			expected: []int{},
		},
		{
			name:     "flatten single empty slice",
			items:    [][]int{{}},
			expected: []int{},
		},
		{
			name:     "flatten with nil slice",
			items:    [][]int{{1, 2}, nil, {3, 4}},
			expected: []int{1, 2, 3, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FlatMap(tt.items)
			if !slices.Equal(result, tt.expected) {
				t.Errorf("FlatMap() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestFlatMapWithStrings(t *testing.T) {
	items := [][]string{
		{"hello", "world"},
		{"foo", "bar"},
		{},
		{"baz"},
	}
	expected := []string{"hello", "world", "foo", "bar", "baz"}

	result := FlatMap(items)
	if !slices.Equal(result, expected) {
		t.Errorf("FlatMap() = %v, expected %v", result, expected)
	}
}

func TestFlatMapToIter(t *testing.T) {
	items := [][]int{{1, 2}, {3, 4}, {5}}
	expected := []int{1, 2, 3, 4, 5}

	result := slices.Collect(FlatMapToIter(items))
	if !slices.Equal(result, expected) {
		t.Errorf("FlatMapToIter() collected = %v, expected %v", result, expected)
	}
}
