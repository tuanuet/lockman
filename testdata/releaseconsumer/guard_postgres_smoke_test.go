package lockmanreleaseconsumer

import (
	"testing"

	"github.com/tuanuet/lockman"
	guardpostgres "github.com/tuanuet/lockman/guard/postgres"
)

func TestReleasedModuleCompiles(t *testing.T) {
	var _ lockman.Identity
	var _ guardpostgres.ExistingRowStatus
}
