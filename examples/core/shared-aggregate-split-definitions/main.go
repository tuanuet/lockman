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

	"github.com/tuanuet/lockman/backend"
	lockredis "github.com/tuanuet/lockman/backend/redis"
	redisstore "github.com/tuanuet/lockman/idempotency/redis"
	"github.com/tuanuet/lockman/lockkit/definitions"
	"github.com/tuanuet/lockman/lockkit/observe"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/lockkit/runtime"
	"github.com/tuanuet/lockman/lockkit/workers"
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

	prefix := fmt.Sprintf("lockman:example:phase2:shared-boundary:%d", time.Now().UnixNano())
	driver := lockredis.NewDriver(client, prefix+":lease")
	store := redisstore.New(client, prefix+":idempotency")

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderApprovalSync",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      5 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		return err
	}
	if err := reg.Register(definitions.LockDefinition{
		ID:                  "OrderApprovalAsync",
		Kind:                backend.KindParent,
		Resource:            "order",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionAsync,
		LeaseTTL:            5 * time.Second,
		IdempotencyRequired: true,
		KeyBuilder:          definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		return err
	}

	runtimeMgr, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		return err
	}
	workerMgr, err := workers.NewManager(reg, driver, store)
	if err != nil {
		return err
	}
	defer func() {
		_ = workerMgr.Shutdown(context.Background())
		_ = runtimeMgr.Shutdown(context.Background())
	}()

	runtimeReq := definitions.SyncLockRequest{
		DefinitionID: "OrderApprovalSync",
		KeyInput:     map[string]string{"order_id": "123"},
		Ownership: definitions.OwnershipMeta{
			OwnerID:     "example:runtime-user",
			ServiceName: "example",
			HandlerName: "OrderApprovalSync",
		},
	}
	if err := runtimeMgr.ExecuteExclusive(context.Background(), runtimeReq, func(ctx context.Context, lease definitions.LeaseContext) error {
		if _, err := fmt.Fprintf(out, "runtime path: acquired %s\n", lease.ResourceKey); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "runtime definition: OrderApprovalSync")
		return err
	}); err != nil {
		return err
	}

	workerReq := definitions.MessageClaimRequest{
		DefinitionID: "OrderApprovalAsync",
		KeyInput:     map[string]string{"order_id": "123"},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "example:worker",
			MessageID:     "message-order-123",
			Attempt:       1,
			ConsumerGroup: "examples",
			HandlerName:   "OrderApprovalAsync",
		},
		IdempotencyKey: "order-approval:123",
	}
	if err := workerMgr.ExecuteClaimed(context.Background(), workerReq, func(ctx context.Context, claim definitions.ClaimContext) error {
		if _, err := fmt.Fprintf(out, "worker path: claimed %s\n", claim.ResourceKey); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "worker definition: OrderApprovalAsync")
		return err
	}); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "shared aggregate key: order:123"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "teaching point: split sync and async definitions can still share one aggregate boundary"); err != nil {
		return err
	}

	if err := workerMgr.Shutdown(context.Background()); err != nil {
		return err
	}
	if err := runtimeMgr.Shutdown(context.Background()); err != nil {
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
