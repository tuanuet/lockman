package postgres_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/tuanuet/lockman/guard"
	pgguard "github.com/tuanuet/lockman/guard/postgres"
)

func TestClassifyExistingRowUpdateReturnsApplied(t *testing.T) {
	outcome, err := pgguard.ClassifyExistingRowUpdate(
		guard.Context{LockID: "StrictOrderClaim", ResourceKey: "order:123", FencingToken: 5},
		pgguard.ExistingRowStatus{
			Found:              true,
			Applied:            true,
			CurrentToken:       5,
			CurrentResourceKey: "order:123",
			CurrentLockID:      "StrictOrderClaim",
		},
	)
	if err != nil {
		t.Fatalf("ClassifyExistingRowUpdate returned error: %v", err)
	}
	if outcome != guard.OutcomeApplied {
		t.Fatalf("expected applied, got %q", outcome)
	}
}

func TestClassifyExistingRowUpdateTreatsEqualTokenAsStale(t *testing.T) {
	outcome, err := pgguard.ClassifyExistingRowUpdate(
		guard.Context{LockID: "StrictOrderClaim", ResourceKey: "order:123", FencingToken: 5},
		pgguard.ExistingRowStatus{
			Found:              true,
			Applied:            false,
			CurrentToken:       5,
			CurrentResourceKey: "order:123",
			CurrentLockID:      "StrictOrderClaim",
		},
	)
	if err != nil {
		t.Fatalf("ClassifyExistingRowUpdate returned error: %v", err)
	}
	if outcome != guard.OutcomeStaleRejected {
		t.Fatalf("expected stale, got %q", outcome)
	}
}

func TestClassifyExistingRowUpdateRejectsMissingRowAsInvariant(t *testing.T) {
	_, err := pgguard.ClassifyExistingRowUpdate(
		guard.Context{LockID: "StrictOrderClaim", ResourceKey: "order:123", FencingToken: 5},
		pgguard.ExistingRowStatus{Found: false},
	)
	if !errors.Is(err, guard.ErrInvariantRejected) {
		t.Fatalf("expected invariant rejection, got %v", err)
	}
}

func TestClassifyExistingRowUpdateRejectsBoundaryMismatchAsInvariant(t *testing.T) {
	_, err := pgguard.ClassifyExistingRowUpdate(
		guard.Context{LockID: "StrictOrderClaim", ResourceKey: "order:123", FencingToken: 5},
		pgguard.ExistingRowStatus{
			Found:              true,
			Applied:            false,
			CurrentToken:       1,
			CurrentResourceKey: "order:456",
			CurrentLockID:      "StrictOrderClaim",
		},
	)
	if !errors.Is(err, guard.ErrInvariantRejected) {
		t.Fatalf("expected invariant rejection, got %v", err)
	}
}

func TestClassifyExistingRowUpdateRejectsLockIDMismatchAsInvariant(t *testing.T) {
	_, err := pgguard.ClassifyExistingRowUpdate(
		guard.Context{LockID: "StrictOrderClaim", ResourceKey: "order:123", FencingToken: 5},
		pgguard.ExistingRowStatus{
			Found:              true,
			Applied:            false,
			CurrentToken:       1,
			CurrentResourceKey: "order:123",
			CurrentLockID:      "OtherStrictOrderClaim",
		},
	)
	if !errors.Is(err, guard.ErrInvariantRejected) {
		t.Fatalf("expected invariant rejection, got %v", err)
	}
}

func TestClassifyExistingRowUpdateRejectsInconsistentStateAsInvariant(t *testing.T) {
	_, err := pgguard.ClassifyExistingRowUpdate(
		guard.Context{LockID: "StrictOrderClaim", ResourceKey: "order:123", FencingToken: 5},
		pgguard.ExistingRowStatus{
			Found:              true,
			Applied:            false,
			CurrentToken:       1,
			CurrentResourceKey: "order:123",
			CurrentLockID:      "StrictOrderClaim",
		},
	)
	if !errors.Is(err, guard.ErrInvariantRejected) {
		t.Fatalf("expected invariant rejection, got %v", err)
	}
}

type stubScanner struct {
	values []any
	err    error
}

func (s stubScanner) Scan(dest ...any) error {
	if s.err != nil {
		return s.err
	}
	if len(dest) != 5 {
		return fmt.Errorf("expected 5 scan destinations, got %d", len(dest))
	}

	found, ok := dest[0].(*bool)
	if !ok {
		return fmt.Errorf("dest[0] must be *bool")
	}
	applied, ok := dest[1].(*bool)
	if !ok {
		return fmt.Errorf("dest[1] must be *bool")
	}
	currentToken, ok := dest[2].(*uint64)
	if !ok {
		return fmt.Errorf("dest[2] must be *uint64")
	}
	currentResourceKey, ok := dest[3].(*string)
	if !ok {
		return fmt.Errorf("dest[3] must be *string")
	}
	currentLockID, ok := dest[4].(*string)
	if !ok {
		return fmt.Errorf("dest[4] must be *string")
	}

	*found = s.values[0].(bool)
	*applied = s.values[1].(bool)
	*currentToken = s.values[2].(uint64)
	*currentResourceKey = s.values[3].(string)
	*currentLockID = s.values[4].(string)

	return nil
}

func TestScanExistingRowStatusDecodesExpectedColumnsInOrder(t *testing.T) {
	status, err := pgguard.ScanExistingRowStatus(stubScanner{
		values: []any{true, false, uint64(42), "order:123", "StrictOrderClaim"},
	})
	if err != nil {
		t.Fatalf("ScanExistingRowStatus returned error: %v", err)
	}

	want := pgguard.ExistingRowStatus{
		Found:              true,
		Applied:            false,
		CurrentToken:       42,
		CurrentResourceKey: "order:123",
		CurrentLockID:      "StrictOrderClaim",
	}
	if status != want {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestScanExistingRowStatusPropagatesScanError(t *testing.T) {
	wantErr := errors.New("scan failed")

	_, err := pgguard.ScanExistingRowStatus(stubScanner{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected scan error %v, got %v", wantErr, err)
	}
}
