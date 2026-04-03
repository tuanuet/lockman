package lockman

import (
	"context"
	"fmt"
	"strings"
)

// DefinitionOption configures a lock definition.
type DefinitionOption func(*definitionConfig)

type definitionConfig struct {
	strict bool
}

type definitionRef struct {
	name   string
	id     string
	binder any
	config definitionConfig
}

// LockDefinition owns stable identity, binding, and definition-level strictness.
type LockDefinition[T any] struct {
	ref     *definitionRef
	binding Binding[T]
}

// DefineLock creates a lock definition with a stable ID derived from its name.
func DefineLock[T any](name string, binding Binding[T], opts ...DefinitionOption) LockDefinition[T] {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("lockman: definition name is required")
	}
	if binding.build == nil {
		panic("lockman: definition binding is required")
	}

	cfg := definitionConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	ref := &definitionRef{
		name:   name,
		id:     stableDefinitionID(name),
		binder: binding,
		config: cfg,
	}

	return LockDefinition[T]{
		ref:     ref,
		binding: binding,
	}
}

// StrictDef marks a lock definition as requiring strict fenced execution.
func StrictDef() DefinitionOption {
	return func(cfg *definitionConfig) {
		cfg.strict = true
	}
}

// stableDefinitionID returns a name-based stable identifier.
func stableDefinitionID(name string) string {
	return name
}

// ForceRelease forcibly releases a lock held under this definition.
func (d LockDefinition[T]) ForceRelease(ctx context.Context, client *Client, resourceKey string) error {
	if client == nil {
		return fmt.Errorf("lockman: client is required: %w", ErrNotImplemented)
	}
	return fmt.Errorf("lockman: force release not yet implemented: %w", ErrNotImplemented)
}

func (d LockDefinition[T]) stableID() string {
	return d.ref.id
}
