package policy

import (
	"fmt"
	"strings"

	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
)

// RejectOverlap enforces Phase 2 overlap rejection between a parent key and one of its children.
func RejectOverlap(parent definitions.LockDefinition, child definitions.LockDefinition, parentKey string, childKey string) error {
	if parent.Kind != definitions.KindParent {
		return nil
	}
	if child.Kind != definitions.KindChild {
		return nil
	}
	if child.ParentRef != parent.ID {
		return nil
	}

	if childKey == parentKey || strings.HasPrefix(childKey, parentKey+":") {
		return fmt.Errorf(
			"%w: child lock %q overlaps parent %q",
			lockerrors.ErrPolicyViolation,
			child.ID,
			parent.ID,
		)
	}

	return nil
}
