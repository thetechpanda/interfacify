package service

import "example.com/interfacify-externalpkg/dep/v3"

// Runner runs tasks with an external client config.
type Runner struct{}

// Run executes one task.
func (Runner) Run(clientv3.Config) error { return nil }
