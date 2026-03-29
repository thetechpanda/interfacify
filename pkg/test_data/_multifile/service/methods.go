package service

import (
	"context"
	"net/url"
)

// Base handles shared requests.
type Base struct{}

// Execute runs the query.
func (Worker) Execute(context.Context, Query) ([]*url.URL, error) { return nil, nil }

// BaseURL returns the shared base URL.
func (Base) BaseURL() *url.URL { return nil }
