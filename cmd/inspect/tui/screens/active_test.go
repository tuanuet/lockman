package screens

import (
	"testing"
	"time"

	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/inspect"
)

func TestActive_Sort(t *testing.T) {
	locks := []inspect.RuntimeLockInfo{
		{DefinitionID: "b", OwnerID: "y", AcquiredAt: time.Now().Add(-time.Hour)},
		{DefinitionID: "a", OwnerID: "z", AcquiredAt: time.Now()},
		{DefinitionID: "c", OwnerID: "x", AcquiredAt: time.Now().Add(-2 * time.Hour)},
	}

	a := NewActive(client.New("http://localhost"))
	model, _ := a.Update(activeLocksMsg{Locks: locks})
	a = model.(*Active)

	a.sortBy = 0
	a.sortLocks()
	if a.locks[0].DefinitionID != "a" {
		t.Errorf("expected first definition 'a', got %q", a.locks[0].DefinitionID)
	}

	a.sortBy = 1
	a.sortLocks()
	if a.locks[0].OwnerID != "x" {
		t.Errorf("expected first owner 'x', got %q", a.locks[0].OwnerID)
	}
}
