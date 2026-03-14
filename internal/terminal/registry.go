package terminal

import (
	"fmt"
	"sync"
)

// Registry manages all active terminals.
type Registry struct {
	mu     sync.Mutex
	terms  map[string]*Terminal
	nextID int
}

// NewRegistry creates an empty terminal registry.
func NewRegistry() *Registry {
	return &Registry{
		terms:  make(map[string]*Terminal),
		nextID: 1,
	}
}

// NextID generates the next terminal ID.
func (r *Registry) NextID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := fmt.Sprintf("t%d", r.nextID)
	r.nextID++
	return id
}

// Add registers a terminal.
func (r *Registry) Add(t *Terminal) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.terms[t.ID] = t
}

// Get returns a terminal by ID, or nil if not found.
func (r *Registry) Get(id string) *Terminal {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.terms[id]
}

// Remove deletes a terminal from the registry.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.terms, id)
}

// List returns all terminals, optionally filtered by owner.
func (r *Registry) List(owner string) []*Terminal {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make([]*Terminal, 0)
	for _, t := range r.terms {
		if owner == "" || t.Owner == owner {
			result = append(result, t)
		}
	}
	return result
}

// Count returns the number of terminals.
func (r *Registry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.terms)
}
