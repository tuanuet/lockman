package guard_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strings"
	"testing"

	"lockman/guard"
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

func TestGuardDoesNotExportLeaseOrClaimMappingHelpers(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info fs.FileInfo) bool {
		name := info.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse guard dir: %v", err)
	}

	pkg := pkgs["guard"]
	if pkg == nil {
		t.Fatalf("expected non-test guard package files present")
	}

	for _, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			decl, ok := n.(*ast.FuncDecl)
			if !ok || decl.Name == nil {
				return true
			}

			switch decl.Name.Name {
			case "ContextFromLease", "ContextFromClaim":
				t.Fatalf("guard must not export %s(...)", decl.Name.Name)
				return false
			default:
				return true
			}
		})
	}
}
