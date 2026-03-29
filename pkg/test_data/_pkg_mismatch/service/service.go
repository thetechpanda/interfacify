package service

// Request describes work.
type Request struct{}

// Runner runs tasks.
type Runner struct{}

// Run handles a request.
func (Runner) Run(Request) error { return nil }
