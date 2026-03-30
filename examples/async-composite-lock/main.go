//go:build lockman_examples

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	redisstore "lockman/idempotency/redis"
	"lockman/lockkit/definitions"
	"lockman/lockkit/registry"
	"lockman/lockkit/workers"
	lockredis "lockman/backend/redis"
)

const defaultRedisURL = "redis://localhost:6379/0"

func main() {
	redisURL := strings.TrimSpace(os.Getenv("LOCKMAN_REDIS_URL"))
	if redisURL == "" {
		redisURL = defaultRedisURL
	}

	if err := run(os.Stdout, redisURL); err != nil {
		fmt.Fprintf(os.Stderr, "example failed: %v\n", err)
		os.Exit(1)
	}
}

func run(out io.Writer, redisURL string) error {
	client, err := newRedisClient(redisURL)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	prefix := fmt.Sprintf("lockman:example:phase2:composite:%d", time.Now().UnixNano())
	driver := lockredis.NewDriver(client, prefix+":lease")
	store := redisstore.New(client, prefix+":idempotency")

	reg := registry.New()
	register := func(def definitions.LockDefinition) error { return reg.Register(def) }

	if err := register(definitions.LockDefinition{
		ID:                  "LedgerMember",
		Kind:                definitions.KindParent,
		Resource:            "ledger",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionAsync,
		LeaseTTL:            5 * time.Second,
		Rank:                20,
		IdempotencyRequired: true,
		KeyBuilder:          definitions.MustTemplateKeyBuilder("ledger:{ledger_id}", []string{"ledger_id"}),
	}); err != nil {
		return err
	}
	if err := register(definitions.LockDefinition{
		ID:                  "AccountMember",
		Kind:                definitions.KindParent,
		Resource:            "account",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionAsync,
		LeaseTTL:            5 * time.Second,
		Rank:                10,
		IdempotencyRequired: true,
		KeyBuilder:          definitions.MustTemplateKeyBuilder("account:{account_id}", []string{"account_id"}),
	}); err != nil {
		return err
	}
	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "TransferComposite",
		Members:          []string{"LedgerMember", "AccountMember"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   2,
		ExecutionKind:    definitions.ExecutionAsync,
	}); err != nil {
		return err
	}

	mgr, err := workers.NewManager(reg, driver, store)
	if err != nil {
		return err
	}

	req := definitions.CompositeClaimRequest{
		DefinitionID:   "TransferComposite",
		IdempotencyKey: "transfer:123",
		MemberInputs: []map[string]string{
			{"ledger_id": "ledger-456"},
			{"account_id": "acct-123"},
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "example-composite-worker",
			MessageID:     "message-123",
			Attempt:       1,
			ConsumerGroup: "examples",
			HandlerName:   "Phase2CompositeWorker",
		},
	}

	if err := mgr.ExecuteCompositeClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
		_, err := fmt.Fprintf(out, "composite callback: %s\n", strings.Join(claim.ResourceKeys, ","))
		return err
	}); err != nil {
		return err
	}

	record, err := store.Get(context.Background(), req.IdempotencyKey)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "composite idempotency after ack: %s\n", record.Status); err != nil {
		return err
	}

	if err := mgr.Shutdown(context.Background()); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}

func newRedisClient(redisURL string) (goredis.UniversalClient, error) {
	if strings.TrimSpace(redisURL) == "" {
		return nil, fmt.Errorf("redis url is required")
	}
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return goredis.NewClient(opts), nil
}
