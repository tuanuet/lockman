package lockman

import "github.com/tuanuet/lockman/internal/sdk"

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
	cachedNormalized      sdk.UseCase
	registryLink          sdk.RegistryLink
	boundToRegistry       bool
}

// ResourceKey returns the bound resource key for this request.
func (r RunRequest) ResourceKey() string {
	return r.resourceKey
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

// HoldRequest is an opaque request produced by HoldUseCase.With.
type HoldRequest struct {
	useCaseName     string
	resourceKey     string
	ownerID         string
	useCaseCore     *useCaseCore
	registryLink    sdk.RegistryLink
	boundToRegistry bool
}

// ResourceKey returns the bound resource key for this request.
func (r HoldRequest) ResourceKey() string {
	return r.resourceKey
}

// ForfeitRequest is an opaque request produced by HoldUseCase.ForfeitWith.
type ForfeitRequest struct {
	useCaseName     string
	token           string
	useCaseCore     *useCaseCore
	registryLink    sdk.RegistryLink
	boundToRegistry bool
}
