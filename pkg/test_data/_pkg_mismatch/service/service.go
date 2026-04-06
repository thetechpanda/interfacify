package service

// request describes work.
type request struct{}

// Runner runs tasks.
type Runner struct{}

// Run handles a request.
func (Runner) Run(request) error { return nil }
