package lockmanreleaseconsumer

import (
	"testing"

	"github.com/tuanuet/lockman"
)

func TestReleasedModuleCompiles(t *testing.T) {
	var _ lockman.Identity
	var _ = lockman.NewRegistry
}
