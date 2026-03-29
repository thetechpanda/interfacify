package service

import "context"

// Runner runs tasks.
type Runner struct{}

// Run handles a task.
func (Runner) Run(context.Context, string) error { return nil }
