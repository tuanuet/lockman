package lockman

import "lockman/internal/sdk"

type callConfig struct {
	ownerID    string
	ownerIDSet bool
}

func applyCallOptions(opts ...CallOption) callConfig {
	cfg := callConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// RunRequest is an opaque request produced by RunUseCase.With.
type RunRequest struct {
	useCaseName           string
	resourceKey           string
	compositeMemberInputs []map[string]string
	ownerID               string
	useCaseCore           *useCaseCore
	registryLink          sdk.RegistryLink
	boundToRegistry       bool
}

// ClaimRequest is an opaque request produced by ClaimUseCase.With.
type ClaimRequest struct {
	useCaseName     string
	resourceKey     string
	ownerID         string
	delivery        Delivery
	useCaseCore     *useCaseCore
	registryLink    sdk.RegistryLink
	boundToRegistry bool
}
