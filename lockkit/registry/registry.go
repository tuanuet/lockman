package registry

import (
	"fmt"
	"sync"

	"lockman/lockkit/definitions"
)

// Reader provides read-only access to registered lock definitions.
type Reader interface {
	MustGet(id string) definitions.LockDefinition
}

// Registry holds lock definitions in Phase 1.
type Registry struct {
	mu          sync.RWMutex
	definitions map[string]definitions.LockDefinition
}

// New creates an empty lock registry.
func New() *Registry {
	return &Registry{
		definitions: make(map[string]definitions.LockDefinition),
	}
}

// Register stores a lock definition.
func (r *Registry) Register(def definitions.LockDefinition) error {
	if err := requireDefinitionID(def); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.definitions[def.ID]; exists {
		return fmt.Errorf("lock definition %q already registered", def.ID)
	}

	r.definitions[def.ID] = cloneDefinition(def)
	return nil
}

// Validate verifies every registered definition.
func (r *Registry) Validate() error {
	r.mu.RLock()
	defs := make([]definitions.LockDefinition, 0, len(r.definitions))
	for _, def := range r.definitions {
		defs = append(defs, def)
	}
	r.mu.RUnlock()

	for _, def := range defs {
		if err := ValidateDefinition(def); err != nil {
			return fmt.Errorf("definition %q: %w", def.ID, err)
		}
	}
	return nil
}

// MustGet returns the stored definition or panics if it is unknown.
func (r *Registry) MustGet(id string) definitions.LockDefinition {
	r.mu.RLock()
	def, exists := r.definitions[id]
	r.mu.RUnlock()
	if !exists {
		panic(fmt.Sprintf("lock definition %q not found", id))
	}
	return cloneDefinition(def)
}

func cloneDefinition(def definitions.LockDefinition) definitions.LockDefinition {
	if def.Tags == nil {
		return def
	}
	cloned := make(map[string]string, len(def.Tags))
	for key, value := range def.Tags {
		cloned[key] = value
	}
	def.Tags = cloned
	return def
}
