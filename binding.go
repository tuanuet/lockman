package lockman

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tuanuet/lockman/internal/sdk"
)

var (
	errEmptyBindingValue       = errors.New("lockman: bound value is required")
	errBindingFunctionRequired = errors.New("lockman: binding function is required")
	errResourcePrefixInvalid   = errors.New("lockman: resource prefix is invalid")
)

// Binding maps typed input into a lock resource key.
type Binding[T any] struct {
	build func(T) (string, error)
}

// UseCaseOption configures a use case definition.
type UseCaseOption func(*useCaseConfig)

type useCaseConfig struct {
	ttl           time.Duration
	wait          time.Duration
	idempotent    bool
	lineageParent string
	composite     []compositeMemberConfig
	definitionRef *definitionRef
}

type compositeMemberConfig struct {
	name  string
	rank  int
	build func(any) (map[string]string, error)
}

// CompositeMember describes one member of a composite run use case.
type CompositeMember[T any] struct {
	name    string
	binding Binding[T]
}

// BindResourceID binds a single resource id and normalizes it to "resource:<id>".
func BindResourceID[T any](resource string, fn func(T) string) Binding[T] {
	resource = strings.TrimSpace(resource)
	return Binding[T]{
		build: func(input T) (string, error) {
			if fn == nil {
				return "", errBindingFunctionRequired
			}
			if resource == "" || strings.Contains(resource, ":") {
				return "", errResourcePrefixInvalid
			}
			id := strings.TrimSpace(fn(input))
			if id == "" {
				return "", errEmptyBindingValue
			}
			return fmt.Sprintf("%s:%s", resource, id), nil
		},
	}
}

// BindKey binds a caller-provided lock key directly.
func BindKey[T any](fn func(T) string) Binding[T] {
	return Binding[T]{
		build: func(input T) (string, error) {
			if fn == nil {
				return "", errBindingFunctionRequired
			}
			key := strings.TrimSpace(fn(input))
			if key == "" {
				return "", errEmptyBindingValue
			}
			return key, nil
		},
	}
}

// TTL configures a lease TTL hint for the use case.
func TTL(ttl time.Duration) UseCaseOption {
	return func(cfg *useCaseConfig) {
		cfg.ttl = ttl
	}
}

// WaitTimeout configures how long run/claim acquisition may wait.
func WaitTimeout(timeout time.Duration) UseCaseOption {
	return func(cfg *useCaseConfig) {
		cfg.wait = timeout
	}
}

// Idempotent marks a claim use case as requiring idempotency behavior.
func Idempotent() UseCaseOption {
	return func(cfg *useCaseConfig) {
		cfg.idempotent = true
	}
}

// Strict marks a run use case as requiring strict fenced execution.
// Deprecated: use StrictDef() as a DefinitionOption instead.
func Strict() UseCaseOption {
	return func(cfg *useCaseConfig) {
		if cfg.definitionRef == nil {
			cfg.definitionRef = &definitionRef{
				config: definitionConfig{strict: true},
			}
		}
		cfg.definitionRef.config.strict = true
	}
}

// DefineCompositeMember declares one typed member for a composite run use case.
func DefineCompositeMember[T any](name string, binding Binding[T]) CompositeMember[T] {
	return CompositeMember[T]{
		name:    strings.TrimSpace(name),
		binding: binding,
	}
}

// Composite marks a run use case as a composite run made of ordered members.
func Composite[T any](members ...CompositeMember[T]) UseCaseOption {
	return func(cfg *useCaseConfig) {
		composite := make([]compositeMemberConfig, 0, len(members))
		for index, member := range members {
			member := member
			composite = append(composite, compositeMemberConfig{
				name: strings.TrimSpace(member.name),
				rank: index + 1,
				build: func(input any) (map[string]string, error) {
					typed, ok := input.(T)
					if !ok {
						return nil, fmt.Errorf("lockman: composite member input type mismatch")
					}
					if member.binding.build == nil {
						return nil, errBindingFunctionRequired
					}
					resourceKey, err := member.binding.build(typed)
					if err != nil {
						return nil, err
					}
					return map[string]string{
						sdk.ResourceKeyInputKey: resourceKey,
					}, nil
				},
			})
		}
		cfg.composite = composite
	}
}

// OwnerID overrides the owner identity for one call.
func OwnerID(id string) CallOption {
	return func(cfg *callConfig) {
		cfg.ownerIDSet = true
		cfg.ownerID = strings.TrimSpace(id)
	}
}
