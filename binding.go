package lockman

import (
	"errors"
	"fmt"
	"strings"
	"time"
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
	strict        bool
	lineageParent string
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

// OwnerID overrides the owner identity for one call.
func OwnerID(id string) CallOption {
	return func(cfg *callConfig) {
		cfg.ownerIDSet = true
		cfg.ownerID = strings.TrimSpace(id)
	}
}
