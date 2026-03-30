package policy

import (
	"fmt"
	"sort"

	"github.com/tuanuet/lockman/lockkit/definitions"
)

// MemberLeasePlan describes one composite member acquire operation after canonical sorting.
type MemberLeasePlan struct {
	Definition  definitions.LockDefinition
	ResourceKey string
}

// CanonicalizeMembers resolves member acquire order by rank, resource, and normalized resource key.
func CanonicalizeMembers(defs []definitions.LockDefinition, keys []string) ([]MemberLeasePlan, error) {
	if len(defs) != len(keys) {
		return nil, fmt.Errorf("composite member definitions and keys length mismatch")
	}

	plan := make([]MemberLeasePlan, len(defs))
	for i := range defs {
		plan[i] = MemberLeasePlan{
			Definition:  defs[i],
			ResourceKey: keys[i],
		}
	}

	sort.SliceStable(plan, func(i, j int) bool {
		left := plan[i]
		right := plan[j]

		if left.Definition.Rank != right.Definition.Rank {
			return left.Definition.Rank < right.Definition.Rank
		}
		if left.Definition.Resource != right.Definition.Resource {
			return left.Definition.Resource < right.Definition.Resource
		}
		return left.ResourceKey < right.ResourceKey
	})

	return plan, nil
}
