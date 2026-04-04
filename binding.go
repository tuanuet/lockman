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
	lineageParent string
	composite     []compositeMemberConfig
	definitionRef *definitionRef
}

type compositeMemberConfig struct {
	name           string
	rank           int
	definitionID   string
	memberIsStrict bool
	failIfHeld     bool
	build          func(any) (map[string]string, error)
}

// CompositeMember describes one member of a composite run use case.
type CompositeMember[T any] struct {
	name         string
	definitionID string
	isStrict     bool
	failIfHeld   bool
	build        func(T) (definitionID string, resourceKey string, err error)
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

// Member declares one composite member backed by a shared LockDefinition.
// The project function transforms the composite input into the member's typed input.
func Member[TInput any, TMember any](name string, def LockDefinition[TMember], project func(TInput) TMember) CompositeMember[TInput] {
	return MemberWithFlags(name, def, def.Config().Strict, def.Config().FailIfHeld, project)
}

// MemberWithStrict declares one composite member with explicit strictness.
func MemberWithStrict[TInput any, TMember any](name string, def LockDefinition[TMember], isStrict bool, project func(TInput) TMember) CompositeMember[TInput] {
	return MemberWithFlags(name, def, isStrict, def.Config().FailIfHeld, project)
}

// MemberWithFlags declares one composite member with explicit strictness and fail-if-held flags.
func MemberWithFlags[TInput any, TMember any](name string, def LockDefinition[TMember], isStrict bool, failIfHeld bool, project func(TInput) TMember) CompositeMember[TInput] {
	if project == nil {
		panic("lockman: member projection function is required")
	}
	if def.ref == nil || def.binding.build == nil {
		panic("lockman: member definition and binding are required")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		panic("lockman: member name is required")
	}
	defID := def.ref.id
	return CompositeMember[TInput]{
		name:         name,
		definitionID: defID,
		isStrict:     isStrict,
		failIfHeld:   failIfHeld,
		build: func(input TInput) (string, string, error) {
			memberInput := project(input)
			resourceKey, err := def.binding.build(memberInput)
			if err != nil {
				return "", "", err
			}
			return defID, resourceKey, nil
		},
	}
}

// DefineCompositeRun declares a composite synchronous run use case with shared-definition members.
func DefineCompositeRun[T any](name string, members ...CompositeMember[T]) RunUseCase[T] {
	return DefineCompositeRunWithOptions(name, nil, members...)
}

// DefineCompositeRunWithOptions declares a composite synchronous run use case with shared-definition members and run options.
func DefineCompositeRunWithOptions[T any](name string, opts []UseCaseOption, members ...CompositeMember[T]) RunUseCase[T] {
	return RunUseCase[T]{
		core:    newUseCaseCoreWithComposite(name, members, opts...),
		binding: Binding[T]{},
	}
}

// OwnerID overrides the owner identity for one call.
func OwnerID(id string) CallOption {
	return func(cfg *callConfig) {
		cfg.ownerIDSet = true
		cfg.ownerID = strings.TrimSpace(id)
	}
}
