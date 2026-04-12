package service

import "example.com/interfacify-foreignembedded/dep"

// Service combines local and imported embedded behavior.
type Service struct {
	dep.Base
	dep.Runner
}

// Local reports service-local behavior.
func (Service) Local() bool { return true }
