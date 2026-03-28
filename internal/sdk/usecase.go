package sdk

import (
	"hash/fnv"
	"strings"
)

type useCaseKind uint8

const (
	useCaseKindRun useCaseKind = iota + 1
	useCaseKindClaim
)

type useCaseID string

type capabilityRequirements struct {
	requiresIdempotency bool
	requiresStrict      bool
	requiresLineage     bool
}

// UseCaseKind identifies whether a use case is run-based or claim-based.
type UseCaseKind uint8

const (
	UseCaseKindRun   UseCaseKind = UseCaseKind(useCaseKindRun)
	UseCaseKindClaim UseCaseKind = UseCaseKind(useCaseKindClaim)
)

// CapabilityRequirements describes backend prerequisites for a use case.
type CapabilityRequirements struct {
	RequiresIdempotency bool
	RequiresStrict      bool
	RequiresLineage     bool
}

type useCase struct {
	id           useCaseID
	publicName   string
	kind         useCaseKind
	requirements capabilityRequirements
	registryLink registryLink
}

// UseCase is an internal-sdk normalized use case consumed by package lockman.
type UseCase struct {
	internal useCase
}

func newUseCase(name string, kind useCaseKind, requirements capabilityRequirements, link registryLink) useCase {
	trimmedName := strings.TrimSpace(name)
	return useCase{
		id:           stableUseCaseID(trimmedName, kind),
		publicName:   trimmedName,
		kind:         kind,
		requirements: requirements,
		registryLink: link,
	}
}

// NewUseCase creates a normalized internal SDK use case.
func NewUseCase(name string, kind UseCaseKind, requirements CapabilityRequirements, link RegistryLink) UseCase {
	return UseCase{
		internal: newUseCase(name, useCaseKind(kind), capabilityRequirements{
			requiresIdempotency: requirements.RequiresIdempotency,
			requiresStrict:      requirements.RequiresStrict,
			requiresLineage:     requirements.RequiresLineage,
		}, registryLink(link)),
	}
}

func stableUseCaseID(name string, kind useCaseKind) useCaseID {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte{nameDelimiter(kind)})
	_, _ = hash.Write([]byte(name))
	return useCaseID("sdk_uc_" + toHex(hash.Sum64()))
}

func nameDelimiter(kind useCaseKind) byte {
	switch kind {
	case useCaseKindRun:
		return 'r'
	case useCaseKindClaim:
		return 'c'
	default:
		return 'u'
	}
}

func toHex(v uint64) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 16)
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = hex[v&0xF]
		v >>= 4
	}
	return string(out)
}

func internalUseCases(in []UseCase) []useCase {
	out := make([]useCase, 0, len(in))
	for _, item := range in {
		out = append(out, item.internal)
	}
	return out
}
