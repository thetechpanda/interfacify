package beta

import foo "net/http"

// Beta runs beta work.
type Beta struct{}

// Write uses an HTTP writer.
func (Beta) Write(foo.ResponseWriter) error { return nil }
