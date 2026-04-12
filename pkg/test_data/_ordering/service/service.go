package service

// Extra exposes embedded interface behavior.
type Extra interface {
	// Echo reports echo behavior.
	Echo() bool
	// Bravo reports bravo behavior.
	Bravo() bool
}

// Contract keeps interface methods intentionally unsorted.
type Contract interface {
	Extra
	// Zulu reports zulu behavior.
	Zulu() bool
	// Alpha reports alpha behavior.
	Alpha() bool
}

// Echoer exposes echo behavior.
type Echoer struct{}

// Echo reports echo behavior.
func (Echoer) Echo() bool { return true }

// Bravoer exposes bravo behavior.
type Bravoer struct{}

// Bravo reports bravo behavior.
func (Bravoer) Bravo() bool { return true }

// Worker keeps concrete methods intentionally unsorted.
type Worker struct {
	Echoer
	Bravoer
}

// Zulu reports zulu behavior.
func (Worker) Zulu() bool { return true }

// Alpha reports alpha behavior.
func (Worker) Alpha() bool { return true }
