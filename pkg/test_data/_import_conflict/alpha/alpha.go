package alpha

import foo "encoding/json"

// Alpha runs alpha work.
type Alpha struct{}

// Marshal uses a JSON value.
func (Alpha) Marshal(foo.RawMessage) error { return nil }
