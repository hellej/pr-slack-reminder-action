package testhelpers

func AsPointer[T any](v T) *T {
	return &v
}
