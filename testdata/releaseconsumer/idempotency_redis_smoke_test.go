package lockmanreleaseconsumer

import (
	"testing"

	"github.com/tuanuet/lockman"
	idempotencyredis "github.com/tuanuet/lockman/idempotency/redis"
)

func TestReleasedModuleCompiles(t *testing.T) {
	var _ lockman.Identity
	var _ = idempotencyredis.New
}
