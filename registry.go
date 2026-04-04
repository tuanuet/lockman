package lockman

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tuanuet/lockman/internal/sdk"
)

type useCaseKind uint8

const (
	useCaseKindRun useCaseKind = iota + 1
	useCaseKindClaim
	useCaseKindHold
)

type useCaseCore struct {
	name       string
	kind       useCaseKind
	config     useCaseConfig
	definition *definitionRef
	registry   *Registry
}

func newUseCaseCore(name string, kind useCaseKind, opts ...UseCaseOption) *useCaseCore {
	cfg := useCaseConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return &useCaseCore{
		name:   strings.TrimSpace(name),
		kind:   kind,
		config: cfg,
	}
}

func newUseCaseCoreWithDefinition(name string, kind useCaseKind, def *definitionRef, opts ...UseCaseOption) *useCaseCore {
	cfg := useCaseConfig{
		definitionRef: def,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return &useCaseCore{
		name:       strings.TrimSpace(name),
		kind:       kind,
		config:     cfg,
		definition: def,
	}
}

func newUseCaseCoreWithComposite[T any](name string, members []CompositeMember[T], opts ...UseCaseOption) *useCaseCore {
	composite := make([]compositeMemberConfig, 0, len(members))
	for index, member := range members {
		member := member
		composite = append(composite, compositeMemberConfig{
			name:           strings.TrimSpace(member.name),
			rank:           index + 1,
			definitionID:   member.definitionID,
			memberIsStrict: member.isStrict,
			failIfHeld:     member.failIfHeld,
			build: func(input any) (map[string]string, error) {
				typed, ok := input.(T)
				if !ok {
					return nil, fmt.Errorf("lockman: composite member input type mismatch")
				}
				if member.build == nil {
					return nil, errBindingFunctionRequired
				}
				definitionID, resourceKey, err := member.build(typed)
				if err != nil {
					return nil, err
				}
				result := map[string]string{
					sdk.ResourceKeyInputKey: resourceKey,
				}
				if definitionID != "" {
					result[sdk.DefinitionIDInputKey] = definitionID
				}
				return result, nil
			},
		})
	}
	cfg := useCaseConfig{
		composite: composite,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return &useCaseCore{
		name:   strings.TrimSpace(name),
		kind:   useCaseKindRun,
		config: cfg,
	}
}

type registeredUseCase interface {
	sdkUseCase() *useCaseCore
}

// Registry holds centrally registered SDK use cases.
type Registry struct {
	byName    map[string]*useCaseCore
	byDefName map[string]*definitionRef
	byDefID   map[string]*definitionRef
	link      sdk.RegistryLink
}

// NewRegistry creates an empty use-case registry.
func NewRegistry() *Registry {
	return &Registry{
		byName:    make(map[string]*useCaseCore),
		byDefName: make(map[string]*definitionRef),
		byDefID:   make(map[string]*definitionRef),
		link:      sdk.NewRegistryLink(),
	}
}

// Register adds use cases and rejects duplicate use-case names.
func (r *Registry) Register(useCases ...registeredUseCase) error {
	if r == nil {
		return fmt.Errorf("lockman: registry is nil")
	}
	if r.byName == nil {
		r.byName = make(map[string]*useCaseCore)
	}

	planned := make([]*useCaseCore, 0, len(useCases))
	seen := make(map[string]struct{}, len(useCases))
	seenDefNames := make(map[string]*definitionRef, len(useCases))
	for _, entry := range useCases {
		if entry == nil {
			return fmt.Errorf("lockman: use case is nil")
		}
		core := entry.sdkUseCase()
		if core == nil {
			return fmt.Errorf("lockman: use case is nil")
		}
		if core.name == "" {
			return fmt.Errorf("lockman: use case name is required")
		}
		if core.registry != nil && core.registry != r {
			return fmt.Errorf("lockman: use case %q belongs to a different registry", core.name)
		}
		if _, exists := r.byName[core.name]; exists {
			return fmt.Errorf("lockman: duplicate use case name %q", core.name)
		}
		if _, exists := seen[core.name]; exists {
			return fmt.Errorf("lockman: duplicate use case name %q", core.name)
		}

		defRef := definitionRefForUseCase(core)
		if defRef != nil {
			if existingRef, exists := seenDefNames[defRef.name]; exists && existingRef != defRef {
				return fmt.Errorf("lockman: duplicate definition name %q", defRef.name)
			}
			seenDefNames[defRef.name] = defRef
		}

		seen[core.name] = struct{}{}
		planned = append(planned, core)
	}

	for _, core := range planned {
		r.byName[core.name] = core
		core.registry = r
		defRef := definitionRefForUseCase(core)
		if defRef != nil {
			if defRef.id != "" {
				if r.byDefID == nil {
					r.byDefID = make(map[string]*definitionRef)
				}
				r.byDefID[defRef.id] = defRef
			}
			if defRef.name != "" {
				if r.byDefName == nil {
					r.byDefName = make(map[string]*definitionRef)
				}
				r.byDefName[defRef.name] = defRef
			}
		}
	}
	return nil
}

func (r *Registry) registeredUseCases() []*useCaseCore {
	if r == nil || len(r.byName) == 0 {
		return nil
	}

	names := make([]string, 0, len(r.byName))
	for name := range r.byName {
		names = append(names, name)
	}
	sort.Strings(names)

	useCases := make([]*useCaseCore, 0, len(names))
	for _, name := range names {
		useCases = append(useCases, r.byName[name])
	}
	return useCases
}

func (r *Registry) findDefinition(id string) *definitionRef {
	if r == nil {
		return nil
	}
	if r.byDefID != nil {
		if def, exists := r.byDefID[id]; exists {
			return def
		}
	}
	if r.byDefName != nil {
		if def, exists := r.byDefName[id]; exists {
			return def
		}
	}
	for _, core := range r.byName {
		if core.definition != nil && (core.definition.id == id || core.definition.name == id) {
			return core.definition
		}
		if core.config.definitionRef != nil && (core.config.definitionRef.id == id || core.config.definitionRef.name == id) {
			return core.config.definitionRef
		}
	}
	return nil
}

func (r *Registry) findDefinitionRefs() []*definitionRef {
	if r == nil {
		return nil
	}
	refs := make([]*definitionRef, 0)
	seen := make(map[*definitionRef]bool)
	for _, core := range r.byName {
		if core.definition != nil && !seen[core.definition] {
			refs = append(refs, core.definition)
			seen[core.definition] = true
		}
		if core.config.definitionRef != nil && !seen[core.config.definitionRef] {
			refs = append(refs, core.config.definitionRef)
			seen[core.config.definitionRef] = true
		}
	}
	return refs
}

func definitionIDForUseCase(core *useCaseCore) string {
	if core.definition != nil {
		return core.definition.id
	}
	if core.config.definitionRef != nil {
		return core.config.definitionRef.id
	}
	return ""
}

func definitionRefForUseCase(core *useCaseCore) *definitionRef {
	if core.definition != nil {
		return core.definition
	}
	return core.config.definitionRef
}

func definitionNameForUseCase(core *useCaseCore) string {
	if core.definition != nil {
		return core.definition.name
	}
	if core.config.definitionRef != nil {
		return core.config.definitionRef.name
	}
	return ""
}
