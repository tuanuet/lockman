package sdk

import "sync/atomic"

type registryLink uint64

var registryLinkCounter atomic.Uint64

// RegistryLink identifies the registry instance a normalized use case/request belongs to.
type RegistryLink = registryLink

func newRegistryLink() registryLink {
	return registryLink(registryLinkCounter.Add(1))
}

func registryLinkMismatch(left registryLink, right registryLink) bool {
	return left != right
}

// NewRegistryLink creates a unique registry identity token.
func NewRegistryLink() RegistryLink {
	return newRegistryLink()
}

// RegistryLinkMismatch reports whether two registry links belong to different registries.
func RegistryLinkMismatch(left RegistryLink, right RegistryLink) bool {
	return registryLinkMismatch(left, right)
}
