package service

// RunnerNested is returned from the source package.
type RunnerNested struct {
	IsNested bool
}

// Runner runs tasks.
type Runner struct{}

// GetNested returns a source-package struct.
func (Runner) GetNested() RunnerNested { return RunnerNested{} }
