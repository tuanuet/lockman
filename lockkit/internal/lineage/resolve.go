package lineage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
)

var (
	errLeaseIDGenerationFailed = errors.New("lineage: lease id generation failed")
	leaseIDReader              = rand.Read
)

type AcquirePlan struct {
	DefinitionID string
	ResourceKey  string
	Kind         definitions.LockKind
	AncestorKeys []drivers.AncestorKey
	LeaseID      string
}

func (p AcquirePlan) LeaseMeta() drivers.LineageLeaseMeta {
	return drivers.LineageLeaseMeta{
		LeaseID:      p.LeaseID,
		Kind:         p.Kind,
		AncestorKeys: cloneAncestorKeys(p.AncestorKeys),
	}
}

func ResolveAcquirePlan(
	def definitions.LockDefinition,
	definitionsByID map[string]definitions.LockDefinition,
	input map[string]string,
) (AcquirePlan, error) {
	resourceKey, err := def.KeyBuilder.Build(input)
	if err != nil {
		return AcquirePlan{}, err
	}

	ancestors, err := resolveAncestors(def, definitionsByID, input)
	if err != nil {
		return AcquirePlan{}, err
	}
	leaseID, err := newLeaseID()
	if err != nil {
		return AcquirePlan{}, err
	}

	return AcquirePlan{
		DefinitionID: def.ID,
		ResourceKey:  resourceKey,
		Kind:         def.Kind,
		// Ancestors are ordered root-first so script key assembly, renew, and release stay stable.
		AncestorKeys: ancestors,
		LeaseID:      leaseID,
	}, nil
}

func resolveAncestors(
	def definitions.LockDefinition,
	definitionsByID map[string]definitions.LockDefinition,
	input map[string]string,
) ([]drivers.AncestorKey, error) {
	if def.ParentRef != "" && def.Kind != definitions.KindChild {
		return nil, fmt.Errorf("lineage: non-child definition %q must not set parent ref", def.ID)
	}
	if def.ParentRef == "" {
		if def.Kind == definitions.KindChild {
			return nil, fmt.Errorf("lineage: child definition %q missing parent ref", def.ID)
		}
		return nil, nil
	}

	visited := map[string]struct{}{
		def.ID: {},
	}

	stack := make([]drivers.AncestorKey, 0, 4)
	parentID := def.ParentRef
	for parentID != "" {
		if _, seen := visited[parentID]; seen {
			return nil, fmt.Errorf("lineage: cyclic parent ref at %q", parentID)
		}
		visited[parentID] = struct{}{}

		parent, ok := definitionsByID[parentID]
		if !ok {
			return nil, fmt.Errorf("lineage: missing parent definition %q for %q", parentID, def.ID)
		}

		key, err := parent.KeyBuilder.Build(input)
		if err != nil {
			return nil, err
		}
		stack = append(stack, drivers.AncestorKey{
			DefinitionID: parent.ID,
			ResourceKey:  key,
		})

		parentID = parent.ParentRef
	}

	// stack currently holds immediate-parent-first; reverse to root-first.
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}

	return stack, nil
}

func cloneAncestorKeys(input []drivers.AncestorKey) []drivers.AncestorKey {
	if len(input) == 0 {
		return nil
	}
	out := make([]drivers.AncestorKey, len(input))
	copy(out, input)
	return out
}

func newLeaseID() (string, error) {
	var b [16]byte
	if _, err := leaseIDReader(b[:]); err != nil {
		return "", fmt.Errorf("%w: %v", errLeaseIDGenerationFailed, err)
	}
	return hex.EncodeToString(b[:]), nil
}
