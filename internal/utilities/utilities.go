package utilities

import "slices"

func Filter[T any](items []T, filter func(e T) bool) []T {
	var kept []T
	for _, item := range items {
		if filter(item) {
			kept = append(kept, item)
		}
	}
	return kept
}

func Find[T any](items []T, predicate func(e T) bool) (T, bool) {
	index := slices.IndexFunc(items, predicate)
	if index >= 0 {
		return items[index], true
	}
	var zero T
	return zero, false
}

func Map[T any, V any](items []T, mapper func(T) V) []V {
	var mapped []V
	for _, item := range items {
		mapped = append(mapped, mapper(item))
	}
	return mapped
}

// exits early on error (and returns it)
func MapWithError[T any, V any](items []T, mapper func(T) (V, error)) ([]V, error) {
	var mappedBeforeError []V
	for _, item := range items {
		mappedItem, err := mapper(item)
		if err != nil {
			return mappedBeforeError, err
		}
		mappedBeforeError = append(mappedBeforeError, mappedItem)
	}
	return mappedBeforeError, nil
}

// FlatMap flattens a slice of slices into a single slice, preserving order.
func FlatMap[T any](items [][]T) []T {
	var flattened []T
	for _, slice := range items {
		flattened = append(flattened, slice...)
	}
	return flattened
}

// Intersperse returns the items with the separator placed between each adjacent pair.
func Intersperse[T any](items []T, separator T) []T {
	result := make([]T, 0, max(0, 2*len(items)-1))
	for index, item := range items {
		if index > 0 {
			result = append(result, separator)
		}
		result = append(result, item)
	}
	return result
}

// UniqueFunc returns a new slice with duplicate elements removed, using a custom equality function.
// For non-comparable types or custom equality logic.
func UniqueFunc[T any](items []T, equal func(T, T) bool) []T {
	if len(items) == 0 {
		return nil
	}

	result := make([]T, 0, len(items))

	for _, item := range items {
		found := false
		for _, existing := range result {
			if equal(item, existing) {
				found = true
				break
			}
		}
		if !found {
			result = append(result, item)
		}
	}

	return result
}
