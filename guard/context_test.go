package guard_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tuanuet/lockman/guard"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
)

func TestContextContractFieldsAreStable(t *testing.T) {
	_ = guard.Context{
		LockID:         "StrictOrderLock",
		ResourceKey:    "order:123",
		FencingToken:   7,
		OwnerID:        "runtime-a",
		MessageID:      "msg-123",
		IdempotencyKey: "idem-123",
	}
}

func TestOutcomeStringsRemainStable(t *testing.T) {
	cases := map[guard.Outcome]string{
		guard.OutcomeApplied:           "applied",
		guard.OutcomeDuplicateIgnored:  "duplicate_ignored",
		guard.OutcomeStaleRejected:     "stale_rejected",
		guard.OutcomeVersionConflict:   "version_conflict",
		guard.OutcomeInvariantRejected: "invariant_rejected",
	}

	for outcome, want := range cases {
		if string(outcome) != want {
			t.Fatalf("outcome %q changed, want %q", outcome, want)
		}
	}
}

func TestInvariantRejectedSentinelRemainsStable(t *testing.T) {
	wrapped := fmt.Errorf("guard failed: %w", guard.ErrInvariantRejected)
	if !errors.Is(wrapped, guard.ErrInvariantRejected) {
		t.Fatalf("expected wrapped guard invariant error to match sentinel")
	}
	if !errors.Is(lockerrors.ErrInvariantRejected, guard.ErrInvariantRejected) {
		t.Fatalf("expected legacy lockkit invariant error to alias guard sentinel")
	}
}

func TestGuardExportsNoFuncs(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)

	// Use go/build so this test tracks the default build surface (build tags, GOOS/GOARCH).
	buildPkg, err := build.Default.ImportDir(dir, 0)
	if err != nil {
		t.Fatalf("ImportDir(%s): %v", dir, err)
	}
	if len(buildPkg.GoFiles) == 0 {
		t.Fatalf("expected guard package to have non-test files")
	}

	fset := token.NewFileSet()
	for _, name := range buildPkg.GoFiles {
		path := filepath.Join(dir, name)
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			decl, ok := n.(*ast.FuncDecl)
			if !ok || decl.Name == nil {
				return true
			}
			if decl.Name.IsExported() {
				t.Fatalf("guard must not export funcs; found %s at %s", decl.Name.Name, fset.Position(decl.Pos()))
			}
			return true
		})
	}
}
