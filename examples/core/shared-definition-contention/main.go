//go:build lockman_examples

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

	lockredis "github.com/tuanuet/lockman/backend/redis"
	redisstore "github.com/tuanuet/lockman/idempotency/redis"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
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

	prefix := fmt.Sprintf("lockman:example:phase2:shared-definition:%d", time.Now().UnixNano())
	driver := lockredis.NewDriver(client, prefix+":lease")
	store := redisstore.New(client, prefix+":idempotency")

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                  "OrderApprovalShared",
		Kind:                definitions.KindParent,
		Resource:            "order",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionBoth,
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
		DefinitionID: "OrderApprovalShared",
		KeyInput:     map[string]string{"order_id": "123"},
		Ownership: definitions.OwnershipMeta{
			OwnerID:     "example:runtime-user",
			ServiceName: "example",
			HandlerName: "OrderApprovalSharedRuntime",
		},
	}
	if err := runtimeMgr.ExecuteExclusive(context.Background(), runtimeReq, func(ctx context.Context, lease definitions.LeaseContext) error {
		if _, err := fmt.Fprintf(out, "runtime path: acquired %s\n", lease.ResourceKey); err != nil {
			return err
		}

		busyWorkerReq := definitions.MessageClaimRequest{
			DefinitionID: "OrderApprovalShared",
			KeyInput:     map[string]string{"order_id": "123"},
			Ownership: definitions.OwnershipMeta{
				OwnerID:       "example:worker-during-runtime",
				MessageID:     "message-order-123-during-runtime",
				Attempt:       1,
				ConsumerGroup: "examples",
				HandlerName:   "OrderApprovalSharedWorkerDuringRuntime",
			},
			IdempotencyKey: "order-approval-shared:123:during-runtime",
		}
		busyErr := workerMgr.ExecuteClaimed(ctx, busyWorkerReq, func(ctx context.Context, claim definitions.ClaimContext) error {
			return fmt.Errorf("worker should not claim while runtime holds %s", claim.ResourceKey)
		})
		switch {
		case errors.Is(busyErr, lockerrors.ErrLockBusy):
			if _, err := fmt.Fprintln(out, "worker path during runtime lock: lock busy"); err != nil {
				return err
			}
		case busyErr != nil:
			return busyErr
		default:
			return fmt.Errorf("expected worker claim to fail while runtime holds order:123")
		}

		return nil
	}); err != nil {
		return err
	}

	workerReq := definitions.MessageClaimRequest{
		DefinitionID: "OrderApprovalShared",
		KeyInput:     map[string]string{"order_id": "123"},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "example:worker",
			MessageID:     "message-order-123",
			Attempt:       1,
			ConsumerGroup: "examples",
			HandlerName:   "OrderApprovalSharedWorker",
		},
		IdempotencyKey: "order-approval-shared:123",
	}
	if err := workerMgr.ExecuteClaimed(context.Background(), workerReq, func(ctx context.Context, claim definitions.ClaimContext) error {
		if _, err := fmt.Fprintf(out, "worker path: claimed %s\n", claim.ResourceKey); err != nil {
			return err
		}

		busyRuntimeReq := definitions.SyncLockRequest{
			DefinitionID: "OrderApprovalShared",
			KeyInput:     map[string]string{"order_id": "123"},
			Ownership: definitions.OwnershipMeta{
				OwnerID:     "example:runtime-during-worker",
				ServiceName: "example",
				HandlerName: "OrderApprovalSharedRuntimeDuringWorker",
			},
		}
		busyErr := runtimeMgr.ExecuteExclusive(ctx, busyRuntimeReq, func(ctx context.Context, lease definitions.LeaseContext) error {
			return fmt.Errorf("runtime should not acquire while worker holds %s", lease.ResourceKey)
		})
		switch {
		case errors.Is(busyErr, lockerrors.ErrLockBusy):
			if _, err := fmt.Fprintln(out, "runtime path during worker claim: lock busy"); err != nil {
				return err
			}
		case busyErr != nil:
			return busyErr
		default:
			return fmt.Errorf("expected runtime acquire to fail while worker holds order:123")
		}

		return nil
	}); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "shared definition: OrderApprovalShared"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "shared aggregate key: order:123"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "teaching point: one ExecutionKind=both definition creates one shared contention boundary across runtime and workers"); err != nil {
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
