package service

import (
	"context"

	"example.com/interfacify-differentpkgnested/nested"
)

type Nested struct {
	IsNested bool
}

// Runner runs tasks.
type Runner struct{}

// Run handles a task.
func (Runner) Run(context.Context, string) error { return nil }

// GetNested returns a nested struct from a foreign package
func (Runner) GetNested() nested.Nested { return nested.Nested{} }

// GetLocal returns a local struct from the current package
func (Runner) GetLocal() Nested { return Nested{} }
