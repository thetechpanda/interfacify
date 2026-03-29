package service

import "context"

// Result carries one typed value.
type Result[T any] struct {
	Value T
}

// Reader exposes typed reads.
type Reader[T any] interface {
	// Read loads one typed result.
	Read(context.Context) (Result[T], error)
}

// Loader combines direct and embedded generic behavior.
type Loader[T any] struct {
	Reader[T]
}

// Load stores one typed result.
func (Loader[T]) Load(context.Context, Result[T]) error { return nil }
