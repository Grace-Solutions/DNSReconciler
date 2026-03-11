// Package provider implements the provider registry and shared utilities (§9).
package provider

import (
	"fmt"
	"sync"

	"github.com/gracesolutions/dns-automatic-updater/internal/core"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
)

// Factory creates a provider instance given its provider-level config and a logger.
type Factory func(providerConfig map[string]any, logger *logging.Logger) (core.Provider, error)

// Registry holds named provider factories.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: map[string]Factory{}}
}

// Register adds a factory under the given name.
func (r *Registry) Register(name string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// Build creates a provider instance for the given name.
func (r *Registry) Build(name string, providerConfig map[string]any, logger *logging.Logger) (core.Provider, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	return factory(providerConfig, logger)
}

// Names returns the list of registered provider names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for n := range r.factories {
		names = append(names, n)
	}
	return names
}

