package externalconsumer

import (
	"testing"

	"github.com/tuanuet/lockman"
	backendredis "github.com/tuanuet/lockman/backend/redis"
	guardpostgres "github.com/tuanuet/lockman/guard/postgres"
	idempotencyredis "github.com/tuanuet/lockman/idempotency/redis"
)

func TestReleasedModulesCompileTogether(t *testing.T) {
	var _ lockman.Identity
	var _ = lockman.NewRegistry
	var _ = backendredis.New
	var _ = idempotencyredis.New
	var _ guardpostgres.ExistingRowStatus
}
