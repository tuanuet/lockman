package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"lockman/lockkit/definitions"
	redisdriver "lockman/lockkit/drivers/redis"
	redisstore "lockman/idempotency/redis"
	"lockman/lockkit/registry"
	"lockman/lockkit/workers"
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
	defer func() {
		_ = client.Close()
	}()

	prefix := fmt.Sprintf("lockman:example:phase3a:strict-worker:%d", time.Now().UnixNano())
	driver := redisdriver.NewDriver(client, prefix+":lease")
	store := redisstore.New(client, prefix+":idempotency")

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "StrictOrderClaim",
		Kind:                 definitions.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionAsync,
		LeaseTTL:             5 * time.Second,
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  true,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		return err
	}

	mgr, err := workers.NewManager(reg, driver, store)
	if err != nil {
		return err
	}

	req := definitions.MessageClaimRequest{
		DefinitionID: "StrictOrderClaim",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "strict-worker-a",
			MessageID:     "strict-message-123",
			Attempt:       1,
			ConsumerGroup: "examples",
			HandlerName:   "Phase3aStrictWorker",
		},
		IdempotencyKey: "strict-msg:123",
	}

	if err := mgr.ExecuteClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
		if _, err := fmt.Fprintf(out, "strict worker claim: %s\n", claim.ResourceKey); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "fencing token: %d\n", claim.FencingToken)
		return err
	}); err != nil {
		return err
	}

	record, err := store.Get(context.Background(), req.IdempotencyKey)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "idempotency after ack: %s\n", record.Status); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "teaching point: strict worker exposes fencing tokens; guarded writes still arrive in phase3b"); err != nil {
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
