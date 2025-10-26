package utilities

import (
	"iter"
	"slices"
)

func Filter[T any](items []T, filter func(e T) bool) []T {
	return slices.Collect(FilterToIter(items, filter))
}

func FilterToIter[T any](items []T, filter func(e T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, item := range items {

			if !filter(item) {
				continue
			}

			if !yield(item) {
				return
			}

		}
	}
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
	return slices.Collect(MapToIter(items, mapper))
}

func MapToIter[T any, V any](items []T, mapper func(T) V) iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, item := range items {
			if !yield(mapper(item)) {
				return
			}
		}
	}
}

// exits early on error (and returns it)
func MapWithError[T any, V any](items []T, mapper func(T) (V, error)) ([]V, error) {
	var result []V
	var firstError error

	for mapped, err := range MapWithErrorToIter(items, mapper) {
		if err != nil {
			firstError = err
			break
		}
		result = append(result, mapped)
	}

	return result, firstError
}

// exits early on error (and returns it)
func MapWithErrorToIter[T any, V any](items []T, mapper func(T) (V, error)) iter.Seq2[V, error] {
	return func(yield func(V, error) bool) {
		for _, item := range items {
			mapped, err := mapper(item)
			if !yield(mapped, err) {
				return
			}
			if err != nil {
				return
			}
		}
	}
}

// FlatMap flattens a slice of slices into a single slice, preserving order.
func FlatMap[T any](items [][]T) []T {
	return slices.Collect(FlatMapToIter(items))
}

// FlatMapToIter flattens a slice of slices into an iterator that yields each element.
func FlatMapToIter[T any](items [][]T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, slice := range items {
			for _, item := range slice {
				if !yield(item) {
					return
				}
			}
		}
	}
}

// Unique returns a new slice with duplicate elements removed, preserving the order of first occurrence.
// For comparable types, it uses a map for O(n) performance.
func Unique[T comparable](items []T) []T {
	if len(items) == 0 {
		return nil
	}

	seen := make(map[T]struct{}, len(items))
	result := make([]T, 0, len(items))

	for _, item := range items {
		if _, exists := seen[item]; !exists {
			seen[item] = struct{}{}
			result = append(result, item)
		}
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
