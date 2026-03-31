package lockmanreleaseconsumer

import (
	"testing"

	"github.com/tuanuet/lockman"
	backendredis "github.com/tuanuet/lockman/backend/redis"
)

func TestReleasedModuleCompiles(t *testing.T) {
	var _ lockman.Identity
	var _ = backendredis.New
}
