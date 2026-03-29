package examples

// A handles A and C flags lookup
type A struct{ *C }

// HasA true if A is true
func (*A) HasA() bool { return true }

type iC interface{ HasC() bool }

// B handles B and C flags lookup
type B struct{ iC }

// HasB true if B is true
func (*B) HasB() bool { return true }

type C struct{}

// HasC true if C is true
func (*C) HasC() bool { return true }
