package service

// Pair exposes typed key/value access.
type Pair[K, V any] interface {
	// Key returns the key.
	Key() K
	// Value returns the value.
	Value() V
}

// Entry stores direct and embedded key/value behavior.
type Entry[K, V any] struct {
	Pair[K, V]
}

// Put stores a key and value.
func (Entry[K, V]) Put(K, V) error { return nil }

// Snapshot returns the current pair.
func (Entry[K, V]) Snapshot() Pair[K, V] { return nil }
