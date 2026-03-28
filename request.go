package lockman

type callConfig struct {
	ownerID string
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
	useCaseName string
	resourceKey string
	ownerID     string
	useCaseCore *useCaseCore
}

// ClaimRequest is an opaque request produced by ClaimUseCase.With.
type ClaimRequest struct {
	useCaseName string
	resourceKey string
	ownerID     string
	delivery    Delivery
	useCaseCore *useCaseCore
}
