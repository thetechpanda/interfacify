package nested

// Leaf exposes leaf behavior.
type Leaf struct{}

// LeafMethod reports leaf behavior.
func (Leaf) LeafMethod() bool { return true }

// Middle exposes middle behavior.
type Middle struct{ Leaf }

// MiddleMethod reports middle behavior.
func (Middle) MiddleMethod() bool { return true }

// Top exposes top behavior.
type Top struct{ Middle }

// TopMethod reports top behavior.
func (Top) TopMethod() bool { return true }
