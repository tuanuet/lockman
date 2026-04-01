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
	name     string
	kind     useCaseKind
	config   useCaseConfig
	registry *Registry
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

type registeredUseCase interface {
	sdkUseCase() *useCaseCore
}

// Registry holds centrally registered SDK use cases.
type Registry struct {
	byName map[string]*useCaseCore
	link   sdk.RegistryLink
}

// NewRegistry creates an empty use-case registry.
func NewRegistry() *Registry {
	return &Registry{
		byName: make(map[string]*useCaseCore),
		link:   sdk.NewRegistryLink(),
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
		seen[core.name] = struct{}{}
		planned = append(planned, core)
	}

	for _, core := range planned {
		r.byName[core.name] = core
		core.registry = r
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
