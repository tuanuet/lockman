package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func TestSubcommands_Output(t *testing.T) {
	snap := inspect.Snapshot{
		RuntimeLocks: []inspect.RuntimeLockInfo{
			{DefinitionID: "order", ResourceID: "order:1", OwnerID: "api-1", AcquiredAt: time.Now()},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/locks/inspect":
			writeJSON(w, snap)
		case "/locks/inspect/active":
			writeJSON(w, snap.RuntimeLocks)
		case "/locks/inspect/events":
			writeJSON(w, []observe.Event{})
		case "/locks/inspect/health":
			writeJSON(w, map[string]string{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tests := []struct {
		subcommand string
		args       []string
		check      func(output string) error
	}{
		{
			subcommand: "snapshot",
			args:       []string{"--url", srv.URL + "/locks/inspect"},
			check: func(output string) error {
				if !strings.Contains(output, "order:1") {
					return fmt.Errorf("expected order:1 in output")
				}
				return nil
			},
		},
		{
			subcommand: "active",
			args:       []string{"--url", srv.URL + "/locks/inspect"},
			check: func(output string) error {
				if !strings.Contains(output, "api-1") {
					return fmt.Errorf("expected api-1 in output")
				}
				return nil
			},
		},
		{
			subcommand: "health",
			args:       []string{"--url", srv.URL + "/locks/inspect"},
			check: func(output string) error {
				if !strings.Contains(output, "ok") {
					return fmt.Errorf("expected ok in health output")
				}
				return nil
			},
		},
		{
			subcommand: "events",
			args:       []string{"--url", srv.URL + "/locks/inspect", "--kind", "contention"},
			check: func(output string) error {
				if !strings.Contains(output, "[]") {
					return fmt.Errorf("expected empty array in events output")
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.subcommand, func(t *testing.T) {
			args := append([]string{"run", ".", tt.subcommand}, tt.args...)
			cmd := exec.Command("go", args...)
			cmd.Dir = "."
			cmd.Env = append(cmd.Environ(), "GONOSUMCHECK=*", "GONOSUMDB=*")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("command failed: %v\n%s", err, out)
			}
			if err := tt.check(string(out)); err != nil {
				t.Errorf("output check: %v\noutput: %s", err, out)
			}
		})
	}
}
