package lockman

import "time"

// Lease describes a synchronous lock lease handed to a Run callback.
type Lease struct {
	UseCase      string
	ResourceKey  string
	ResourceKeys []string
	LeaseTTL     time.Duration
	Deadline     time.Time
	FencingToken uint64
}

// Claim describes a worker claim handed to a Claim callback.
type Claim struct {
	UseCase        string
	ResourceKey    string
	LeaseTTL       time.Duration
	Deadline       time.Time
	FencingToken   uint64
	IdempotencyKey string
}

// Delivery carries queue delivery metadata for claim-based use cases.
type Delivery struct {
	MessageID     string
	ConsumerGroup string
	Attempt       int
}
