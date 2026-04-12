package dep

import "context"

// Config stores dependency configuration.
type Config struct{}

// Base exposes embedded struct behavior.
type Base struct{}

// Ping checks the dependency config.
func (Base) Ping(Config) error { return nil }

// Runner exposes embedded interface behavior.
type Runner interface {
	// Run executes the dependency runner.
	Run(context.Context) Config
}
