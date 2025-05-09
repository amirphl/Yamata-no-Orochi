// Package utils provides utility functions for the application.
package utils

func ToPtr[T any](v T) *T {
	return &v
}

func IsTrue(b *bool) bool {
	return b != nil && *b
}
