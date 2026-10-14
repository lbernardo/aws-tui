// Package service defines how AWS services plug into aws-tui.
//
// To add a new service, create a package under internal/services/<name>,
// call Register from its init function and blank-import it in
// internal/services/services.go.
package service

import (
	"fmt"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/lbernardo/aws-tui/internal/ui"
)

// Context carries what a service needs to talk to AWS.
type Context struct {
	AWS     aws.Config
	Profile string
	Region  string
}

// Service is an AWS service exposed in the TUI.
type Service struct {
	// ID is the short identifier used on the command line (e.g. "s3").
	ID          string
	Name        string
	Description string
	// New builds the root view of the service.
	New func(ctx *Context) ui.View
}

var (
	mu       sync.RWMutex
	registry = map[string]Service{}
)

// Register adds a service to the registry. It panics on duplicated IDs.
func Register(s Service) {
	mu.Lock()
	defer mu.Unlock()
	if s.ID == "" || s.New == nil {
		panic("service: ID and New are required")
	}
	if _, dup := registry[s.ID]; dup {
		panic(fmt.Sprintf("service: %q registered twice", s.ID))
	}
	registry[s.ID] = s
}

// Get returns a registered service by ID.
func Get(id string) (Service, bool) {
	mu.RLock()
	defer mu.RUnlock()
	s, ok := registry[id]
	return s, ok
}

// All returns the registered services sorted by name.
func All() []Service {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Service, 0, len(registry))
	for _, s := range registry {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
