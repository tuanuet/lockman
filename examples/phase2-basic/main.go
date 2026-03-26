package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	redisdriver "lockman/lockkit/drivers/redis"
	lockerrors "lockman/lockkit/errors"
	"lockman/lockkit/idempotency"
	redisstore "lockman/lockkit/idempotency/redis"
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

	prefix := fmt.Sprintf("lockman:example:phase2:%d", time.Now().UnixNano())
	driver := redisdriver.NewDriver(client, prefix+":lease")
	store := redisstore.NewStore(client, prefix+":idempotency")

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                  "OrderClaim",
		Kind:                definitions.KindParent,
		Resource:            "order",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionAsync,
		LeaseTTL:            5 * time.Second,
		IdempotencyRequired: true,
		KeyBuilder:          definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		return err
	}

	mgr, err := workers.NewManager(reg, driver, store)
	if err != nil {
		return err
	}

	req := definitions.MessageClaimRequest{
		DefinitionID: "OrderClaim",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "example-worker",
			MessageID:     "message-123",
			Attempt:       1,
			ConsumerGroup: "examples",
			HandlerName:   "Phase2Basic",
		},
		IdempotencyKey: "msg:123",
	}

	if err := mgr.ExecuteClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
		if _, err := fmt.Fprintf(out, "execute: callback running for %s\n", claim.ResourceKey); err != nil {
			return err
		}

		presence, err := driver.CheckPresence(ctx, drivers.PresenceRequest{
			DefinitionID: req.DefinitionID,
			ResourceKeys: []string{claim.ResourceKey},
		})
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(out, "presence while held: %s\n", presenceLabel(presence.Present))
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

	err = mgr.ExecuteClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
		return errors.New("duplicate callback should not execute")
	})
	switch {
	case errors.Is(err, lockerrors.ErrDuplicateIgnored):
		if _, err := fmt.Fprintln(out, "duplicate outcome: ignored"); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		return fmt.Errorf("expected duplicate ignored outcome")
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

func presenceLabel(present bool) string {
	if present {
		return "held"
	}
	return "not_held"
}

var _ idempotency.Store = (*redisstore.Store)(nil)
