package registry

import (
	"fmt"
	"strings"
	"sync"

	"github.com/tuanuet/lockman/lockkit/definitions"
)

// Reader provides read-only access to registered lock definitions.
type Reader interface {
	MustGet(id string) definitions.LockDefinition
	MustGetComposite(id string) definitions.CompositeDefinition
	Definitions() []definitions.LockDefinition
}

// Registry holds lock and composite definitions.
type Registry struct {
	mu          sync.RWMutex
	definitions map[string]definitions.LockDefinition
	composites  map[string]definitions.CompositeDefinition
}

// New creates an empty lock registry.
func New() *Registry {
	return &Registry{
		definitions: make(map[string]definitions.LockDefinition),
		composites:  make(map[string]definitions.CompositeDefinition),
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
	if _, exists := r.composites[def.ID]; exists {
		return fmt.Errorf("lock definition %q collides with registered composite definition", def.ID)
	}

	r.definitions[def.ID] = cloneDefinition(def)
	return nil
}

// RegisterComposite stores a composite definition.
func (r *Registry) RegisterComposite(def definitions.CompositeDefinition) error {
	if err := requireCompositeDefinitionID(def); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.composites[def.ID]; exists {
		return fmt.Errorf("composite definition %q already registered", def.ID)
	}
	if _, exists := r.definitions[def.ID]; exists {
		return fmt.Errorf("composite definition %q collides with lock definition ID", def.ID)
	}

	r.composites[def.ID] = cloneCompositeDefinition(def)
	return nil
}

// Validate verifies every registered definition.
func (r *Registry) Validate() error {
	r.mu.RLock()
	defs := make([]definitions.LockDefinition, 0, len(r.definitions))
	defByID := make(map[string]definitions.LockDefinition, len(r.definitions))
	for _, def := range r.definitions {
		cloned := cloneDefinition(def)
		defs = append(defs, cloned)
		defByID[cloned.ID] = cloned
	}
	composites := make([]definitions.CompositeDefinition, 0, len(r.composites))
	for _, def := range r.composites {
		composites = append(composites, cloneCompositeDefinition(def))
	}
	r.mu.RUnlock()

	for _, def := range defs {
		if err := ValidateDefinition(def); err != nil {
			return fmt.Errorf("definition %q: %w", def.ID, err)
		}
		if err := ValidateDefinitionAgainstRegistry(def, defByID); err != nil {
			return fmt.Errorf("definition %q: %w", def.ID, err)
		}
	}
	for _, composite := range composites {
		if err := ValidateCompositeDefinition(composite, defByID); err != nil {
			return fmt.Errorf("composite definition %q: %w", composite.ID, err)
		}
	}
	return nil
}

// RequiresLineageDriver returns true when the registry contains definitions that
// participate in lineage semantics (either as a child or as a parent with descendants).
//
// Callers can use this to gate manager construction when a backend lacks drivers.LineageDriver.
func RequiresLineageDriver(reg Reader) bool {
	defs := reg.Definitions()
	childrenByParent := indexChildrenByParent(defs)
	for _, def := range defs {
		if definitionUsesLineage(def, childrenByParent) {
			return true
		}
	}
	return false
}

// RequiresStrictRuntimeDriver returns true when the registry contains strict
// definitions that are executable via runtime sync flows.
func RequiresStrictRuntimeDriver(reg Reader) bool {
	for _, def := range reg.Definitions() {
		if def.Mode != definitions.ModeStrict {
			continue
		}
		if def.ExecutionKind == definitions.ExecutionSync || def.ExecutionKind == definitions.ExecutionBoth {
			return true
		}
	}
	return false
}

// RequiresStrictWorkerDriver returns true when the registry contains strict
// definitions that are executable via worker async flows.
func RequiresStrictWorkerDriver(reg Reader) bool {
	for _, def := range reg.Definitions() {
		if def.Mode != definitions.ModeStrict {
			continue
		}
		if def.ExecutionKind == definitions.ExecutionAsync || def.ExecutionKind == definitions.ExecutionBoth {
			return true
		}
	}
	return false
}

func indexChildrenByParent(defs []definitions.LockDefinition) map[string][]string {
	out := make(map[string][]string, len(defs))
	for _, def := range defs {
		parentID := strings.TrimSpace(def.ParentRef)
		if parentID == "" {
			continue
		}
		out[parentID] = append(out[parentID], def.ID)
	}
	return out
}

func definitionUsesLineage(def definitions.LockDefinition, childrenByParent map[string][]string) bool {
	return strings.TrimSpace(def.ParentRef) != "" || len(childrenByParent[def.ID]) > 0
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

// MustGetComposite returns the stored composite definition or panics if it is unknown.
func (r *Registry) MustGetComposite(id string) definitions.CompositeDefinition {
	r.mu.RLock()
	def, exists := r.composites[id]
	r.mu.RUnlock()
	if !exists {
		panic(fmt.Sprintf("composite definition %q not found", id))
	}
	return cloneCompositeDefinition(def)
}

// Definitions returns a cloned snapshot of registered lock definitions.
func (r *Registry) Definitions() []definitions.LockDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]definitions.LockDefinition, 0, len(r.definitions))
	for _, def := range r.definitions {
		defs = append(defs, cloneDefinition(def))
	}
	return defs
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

func cloneCompositeDefinition(def definitions.CompositeDefinition) definitions.CompositeDefinition {
	if def.Members == nil {
		return def
	}
	def.Members = append([]string(nil), def.Members...)
	return def
}
