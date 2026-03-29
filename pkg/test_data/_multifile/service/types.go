package service

import "net/url"

// Query looks up a remote resource.
type Query struct {
	Path string
}

// Worker combines direct and embedded behavior.
type Worker struct {
	Base
	Streamer
}

// Streamer streams resources.
type Streamer interface {
	// Stream emits remote resources.
	Stream(...Query) <-chan *url.URL
}
