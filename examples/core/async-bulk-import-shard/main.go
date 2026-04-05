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
	"github.com/tuanuet/lockman/lockkit/registry"
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

	prefix := fmt.Sprintf("lockman:example:phase2:bulk-import-shard:%d", time.Now().UnixNano())
	driver := lockredis.NewDriver(client, prefix+":lease")
	store := redisstore.New(client, prefix+":idempotency")

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                  "BulkImportShard",
		Kind:                backend.KindParent,
		Resource:            "import-shard",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionAsync,
		LeaseTTL:            5 * time.Second,
		IdempotencyRequired: true,
		KeyBuilder:          definitions.MustTemplateKeyBuilder("import-shard:{shard_id}", []string{"shard_id"}),
	}); err != nil {
		return err
	}

	mgr, err := workers.NewManager(reg, driver, store)
	if err != nil {
		return err
	}
	defer func() { _ = mgr.Shutdown(context.Background()) }()

	req := definitions.MessageClaimRequest{
		DefinitionID: "BulkImportShard",
		KeyInput:     map[string]string{"shard_id": "07"},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "example:bulk-import-worker",
			MessageID:     "bulk-import:workers:shard-07",
			Attempt:       1,
			ConsumerGroup: "examples",
			HandlerName:   "BulkImportShard",
		},
		IdempotencyKey: "bulk-import:workers:shard-07",
	}

	if err := mgr.ExecuteClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
		if _, err := fmt.Fprintf(out, "shard lock: %s\n", claim.ResourceKey); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "package: workers")
		return err
	}); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "teaching point: shard ownership is the default boundary for bulk import"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "contrast: smaller batch locks only work when batches are independently safe and replayable"); err != nil {
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
