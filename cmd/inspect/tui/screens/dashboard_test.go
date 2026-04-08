package screens

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/inspect"
)

func TestDashboard_View(t *testing.T) {
	snap := inspect.Snapshot{
		RuntimeLocks: []inspect.RuntimeLockInfo{
			{DefinitionID: "order", ResourceID: "order:1", OwnerID: "api-1"},
		},
	}

	c := client.New("http://localhost")
	d := NewDashboard(c)
	model, _ := d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	d = model.(*Dashboard)

	// Simulate receiving snapshot data
	d.Update(snapshotMsg{Snapshot: snap})

	view := d.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	if !strings.Contains(view, "order:1") {
		t.Errorf("view should contain lock data, got: %s", view)
	}
}
