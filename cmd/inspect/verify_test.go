package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/cmd/inspect/sse"
	"github.com/tuanuet/lockman/cmd/inspect/tui/components"
	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
)

func TestVerify_ModuleSetup(t *testing.T) {
	t.Run("go.mod exists", func(t *testing.T) {
		if _, err := os.Stat("go.mod"); err == nil {
			return // running from cmd/inspect
		}
		if _, err := os.Stat("cmd/inspect/go.mod"); err != nil {
			t.Fatal("go.mod not found")
		}
	})
	t.Run("go.work includes cmd/inspect", func(t *testing.T) {
		data, err := os.ReadFile("go.work")
		if err != nil {
			data, err = os.ReadFile("../../go.work")
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "./cmd/inspect") {
			t.Fatal("go.work does not include ./cmd/inspect")
		}
	})
	t.Run("all packages compile", func(t *testing.T) {
		cmd := exec.Command("go", "build", "./...")
		cmd.Dir = "."
		cmd.Env = append(os.Environ(), "GONOSUMCHECK=*", "GONOSUMDB=*")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("compilation failed: %v\n%s", err, out)
		}
	})
}

func TestVerify_SSEParser(t *testing.T) {
	t.Run("single line events", func(t *testing.T) {
		input := "data: {\"kind\":\"acquire_succeeded\"}\n\ndata: {\"kind\":\"contention\"}\n"
		events := make(chan json.RawMessage, 10)
		errors := make(chan error, 10)

		go sse.ParseEvents(strings.NewReader(input), events, errors)

		var got []json.RawMessage
		for e := range events {
			got = append(got, e)
		}

		if len(got) != 2 {
			t.Fatalf("expected 2 events, got %d", len(got))
		}
	})

	t.Run("multi-line data fields", func(t *testing.T) {
		input := "data: {\"kind\":\"acquire_succeeded\",\ndata: \"definition_id\":\"order\"}\n\n"
		events := make(chan json.RawMessage, 10)
		errors := make(chan error, 10)

		go sse.ParseEvents(strings.NewReader(input), events, errors)

		var got []json.RawMessage
		for e := range events {
			got = append(got, e)
		}

		if len(got) != 1 {
			t.Fatalf("expected 1 event, got %d", len(got))
		}
	})

	t.Run("malformed JSON goes to errors", func(t *testing.T) {
		input := "data: not-json\n"
		events := make(chan json.RawMessage, 10)
		errors := make(chan error, 10)

		go sse.ParseEvents(strings.NewReader(input), events, errors)

		for range events {
		}

		select {
		case err := <-errors:
			if err == nil {
				t.Error("expected non-nil error")
			}
		default:
			t.Error("expected error for malformed JSON")
		}
	})
}

func TestVerify_HTTPClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/", "":
			json.NewEncoder(w).Encode(inspect.Snapshot{
				RuntimeLocks: []inspect.RuntimeLockInfo{
					{DefinitionID: "order", ResourceID: "order:1", OwnerID: "api-1"},
				},
			})
		case "/active":
			json.NewEncoder(w).Encode([]inspect.RuntimeLockInfo{
				{DefinitionID: "order", ResourceID: "order:1", OwnerID: "api-1"},
			})
		case "/events":
			json.NewEncoder(w).Encode([]observe.Event{})
		case "/health":
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL)

	t.Run("Snapshot", func(t *testing.T) {
		snap, err := c.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(snap.RuntimeLocks) != 1 {
			t.Fatalf("expected 1 lock, got %d", len(snap.RuntimeLocks))
		}
	})

	t.Run("Active", func(t *testing.T) {
		locks, err := c.Active(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(locks) != 1 {
			t.Fatalf("expected 1 lock, got %d", len(locks))
		}
	})

	t.Run("Events", func(t *testing.T) {
		events, err := c.Events(context.Background(), client.Filter{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if events == nil {
			t.Fatal("expected events slice")
		}
	})

	t.Run("Health", func(t *testing.T) {
		status, err := c.Health(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if status["status"] != "ok" {
			t.Fatalf("status = %q, want ok", status["status"])
		}
	})

	t.Run("ParseEventKind maps all 14 kinds", func(t *testing.T) {
		tests := []struct {
			input string
			want  observe.EventKind
		}{
			{"acquire_started", observe.EventAcquireStarted},
			{"acquire_succeeded", observe.EventAcquireSucceeded},
			{"acquire_failed", observe.EventAcquireFailed},
			{"released", observe.EventReleased},
			{"contention", observe.EventContention},
			{"overlap", observe.EventOverlap},
			{"overlap_rejected", observe.EventOverlapRejected},
			{"lease_lost", observe.EventLeaseLost},
			{"renewal_succeeded", observe.EventRenewalSucceeded},
			{"renewal_failed", observe.EventRenewalFailed},
			{"shutdown_started", observe.EventShutdownStarted},
			{"shutdown_completed", observe.EventShutdownCompleted},
			{"client_started", observe.EventClientStarted},
			{"presence_checked", observe.EventPresenceChecked},
			{"invalid", 0},
			{"", 0},
		}
		for _, tt := range tests {
			if got := client.ParseEventKind(tt.input); got != tt.want {
				t.Errorf("ParseEventKind(%q) = %v, want %v", tt.input, got, tt.want)
			}
		}
	})
}

func TestVerify_Components(t *testing.T) {
	t.Run("Table renders headers and rows", func(t *testing.T) {
		columns := []components.Column{
			{Title: "Name", Width: 10},
			{Title: "Value", Width: 10},
		}
		rows := [][]string{
			{"key1", "val1"},
			{"key2", "val2"},
		}
		output := components.Table(columns, rows, 0)
		if !strings.Contains(output, "Name") {
			t.Error("missing Name header")
		}
		if !strings.Contains(output, "key1") {
			t.Error("missing key1 row")
		}
		if !strings.Contains(output, "▸") {
			t.Error("missing selection indicator")
		}
	})

	t.Run("Table empty state", func(t *testing.T) {
		columns := []components.Column{{Title: "Empty", Width: 10}}
		output := components.Table(columns, nil, -1)
		if !strings.Contains(output, "Empty") {
			t.Error("missing header in empty table")
		}
	})

	t.Run("TabBar highlights active", func(t *testing.T) {
		names := []string{"Dash", "Active", "Events", "Stream", "Health"}
		output := components.RenderTabBar(names, 1, 40)
		if !strings.Contains(output, "● Active") {
			t.Error("active tab not highlighted")
		}
	})

	t.Run("FilterModal visibility", func(t *testing.T) {
		fm := components.NewFilterModal()
		fm.Show()
		if !fm.Visible() {
			t.Error("filter not visible after Show")
		}
		fm.Hide()
		if fm.Visible() {
			t.Error("filter still visible after Hide")
		}
	})

	t.Run("StatusBar renders hints", func(t *testing.T) {
		hints := []string{
			components.Hint("Tab", "Navigate"),
			components.Hint("R", "Refresh"),
		}
		output := components.RenderStatusBar(hints, 40)
		if !strings.Contains(output, "Tab") {
			t.Error("missing Tab hint")
		}
	})
}

func TestVerify_Subcommands(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/locks/inspect":
			json.NewEncoder(w).Encode(inspect.Snapshot{
				RuntimeLocks: []inspect.RuntimeLockInfo{
					{DefinitionID: "order", ResourceID: "order:1"},
				},
			})
		case "/locks/inspect/active":
			json.NewEncoder(w).Encode([]inspect.RuntimeLockInfo{
				{DefinitionID: "order", ResourceID: "order:1"},
			})
		case "/locks/inspect/events":
			json.NewEncoder(w).Encode([]observe.Event{})
		case "/locks/inspect/health":
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}
	}))
	defer srv.Close()

	tests := []struct {
		subcmd string
		args   []string
		check  func(string) error
	}{
		{"snapshot", []string{"--url", srv.URL + "/locks/inspect"}, func(o string) error {
			if !strings.Contains(o, "order:1") {
				return fmt.Errorf("missing order:1")
			}
			return nil
		}},
		{"active", []string{"--url", srv.URL + "/locks/inspect"}, func(o string) error {
			if !strings.Contains(o, "order:1") {
				return fmt.Errorf("missing order:1")
			}
			return nil
		}},
		{"events", []string{"--url", srv.URL + "/locks/inspect", "--kind", "contention"}, func(o string) error {
			if !strings.Contains(o, "[]") {
				return fmt.Errorf("missing []")
			}
			return nil
		}},
		{"health", []string{"--url", srv.URL + "/locks/inspect"}, func(o string) error {
			if !strings.Contains(o, "ok") {
				return fmt.Errorf("missing ok")
			}
			return nil
		}},
	}

	for _, tt := range tests {
		t.Run(tt.subcmd, func(t *testing.T) {
			// Determine run path based on current directory
			runPath := "."
			if _, err := os.Stat("go.work"); err == nil {
				runPath = "./cmd/inspect"
			}
			args := append([]string{"run", runPath, tt.subcmd}, tt.args...)
			cmd := exec.Command("go", args...)
			cmd.Dir = "."
			cmd.Env = append(os.Environ(), "GONOSUMCHECK=*", "GONOSUMDB=*")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("failed: %v\n%s", err, out)
			}
			if err := tt.check(string(out)); err != nil {
				t.Errorf("check: %v\noutput: %s", err, out)
			}
		})
	}
}

func TestVerify_IntegrationEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/locks/inspect" && r.URL.RawQuery == "":
			json.NewEncoder(w).Encode(inspect.Snapshot{
				RuntimeLocks: []inspect.RuntimeLockInfo{
					{DefinitionID: "order", ResourceID: "order:1", OwnerID: "api-1", AcquiredAt: time.Now()},
				},
				WorkerClaims: []inspect.WorkerClaimInfo{
					{DefinitionID: "order", ResourceID: "order:2", OwnerID: "api-2", ClaimedAt: time.Now()},
				},
				Renewals: []inspect.RenewalInfo{
					{DefinitionID: "order", ResourceID: "order:1", OwnerID: "api-1", LastRenewed: time.Now()},
				},
				Pipeline: inspect.PipelineState{
					BufferSize:           1024,
					DroppedCount:         3,
					SinkFailureCount:     1,
					ExporterFailureCount: 0,
				},
				Shutdown: inspect.ShutdownInfo{Started: false, Completed: false},
			})
		case r.URL.Path == "/locks/inspect/active":
			json.NewEncoder(w).Encode([]inspect.RuntimeLockInfo{
				{DefinitionID: "order", ResourceID: "order:1", OwnerID: "api-1", AcquiredAt: time.Now()},
			})
		case r.URL.Path == "/locks/inspect/events":
			json.NewEncoder(w).Encode([]observe.Event{
				{Kind: observe.EventAcquireSucceeded, DefinitionID: "order", ResourceID: "order:1", OwnerID: "api-1", Timestamp: time.Now()},
			})
		case r.URL.Path == "/locks/inspect/health":
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	baseURL := srv.URL + "/locks/inspect"

	t.Run("full snapshot roundtrip", func(t *testing.T) {
		c := client.New(baseURL)
		snap, err := c.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(snap.RuntimeLocks) != 1 {
			t.Fatalf("expected 1 lock, got %d", len(snap.RuntimeLocks))
		}
		if len(snap.WorkerClaims) != 1 {
			t.Fatalf("expected 1 claim, got %d", len(snap.WorkerClaims))
		}
		if snap.Pipeline.DroppedCount != 3 {
			t.Errorf("expected 3 dropped, got %d", snap.Pipeline.DroppedCount)
		}
	})

	t.Run("events with kind filter", func(t *testing.T) {
		c := client.New(baseURL)
		events, err := c.Events(context.Background(), client.Filter{
			Kind:  observe.EventAcquireSucceeded,
			Limit: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != observe.EventAcquireSucceeded {
			t.Errorf("wrong kind: %v", events[0].Kind)
		}
	})
}

func TestVerify_BinaryBuilds(t *testing.T) {
	t.Run("binary builds and shows help", func(t *testing.T) {
		buildPath := "."
		if _, err := os.Stat("go.work"); err == nil {
			buildPath = "./cmd/inspect"
		}
		cmd := exec.Command("go", "build", "-o", "/tmp/lockman-inspect-test", buildPath)
		cmd.Dir = "."
		cmd.Env = append(os.Environ(), "GONOSUMCHECK=*", "GONOSUMDB=*")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("build failed: %v\n%s", err, out)
		}

		helpCmd := exec.Command("/tmp/lockman-inspect-test", "--help")
		helpOut, err := helpCmd.Output()
		if err != nil {
			t.Fatalf("help failed: %v", err)
		}

		help := string(helpOut)
		for _, expected := range []string{"snapshot", "active", "events", "health", "--url"} {
			if !strings.Contains(help, expected) {
				t.Errorf("help missing %q", expected)
			}
		}
	})

	t.Run("events subcommand shows filter flags", func(t *testing.T) {
		cmd := exec.Command("/tmp/lockman-inspect-test", "events", "--help")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("events help failed: %v", err)
		}

		help := string(out)
		for _, expected := range []string{"--kind", "--limit", "--definition-id"} {
			if !strings.Contains(help, expected) {
				t.Errorf("events help missing %q", expected)
			}
		}
	})
}

type mockModel struct{}

func (m *mockModel) Init() tea.Cmd {
	return nil
}

func (m *mockModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m *mockModel) View() string {
	return "mock"
}
