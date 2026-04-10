package service

// Deep exposes a deeply promoted method.
type Deep struct{}

// Depth reports the deep path.
func (Deep) Depth() int { return 0 }

// ViaDeep exposes deeper promoted methods.
type ViaDeep struct{ Deep }

// Shallow exposes a shallower method with the same name.
type Shallow struct{}

// Depth reports the shallow path.
func (Shallow) Depth() string { return "" }

// Left exposes one conflicting promoted method.
type Left struct{}

// LeftOnly reports left-only behavior.
func (Left) LeftOnly() bool { return true }

// Ping reports left ping behavior.
func (Left) Ping() int { return 0 }

// Right exposes another conflicting promoted method.
type Right struct{}

// Ping reports right ping behavior.
func (Right) Ping() string { return "" }

// RightOnly reports right-only behavior.
func (Right) RightOnly() error { return nil }

// Runner combines direct, shallow, deep, and conflicting promoted methods.
type Runner struct {
	ViaDeep
	Shallow
	Left
	Right
}

// Run reports runner behavior.
func (Runner) Run() error { return nil }
