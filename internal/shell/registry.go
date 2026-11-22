package shell

import (
	"maps"
	"slices"
	"sync"

	"learn-linux/internal/contract"
)

type Registry struct {
	mu       sync.RWMutex
	commands map[string]contract.CommandHandler
}

var _ contract.CommandRegistry = (*Registry)(nil)

func NewRegistry() *Registry {
	return &Registry{commands: make(map[string]contract.CommandHandler)}
}

func (r *Registry) Register(name string, fn contract.CommandHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[name] = fn
}

func (r *Registry) Lookup(name string) (contract.CommandHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.commands[name]
	return fn, ok
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Sorted(maps.Keys(r.commands))
}
