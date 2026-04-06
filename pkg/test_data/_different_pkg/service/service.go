package service

import "context"

type RunnerNested struct {
	IsNested bool
}

// Runner runs tasks.
type Runner struct{}

// Run handles a task.
func (Runner) Run(context.Context, string) error { return nil }

// GetNested returns a nested struct from the current package
func (Runner) GetNested() RunnerNested { return RunnerNested{} }
