package postgres

import (
	"fmt"

	"lockman/guard"
	lockerrors "lockman/lockkit/errors"
)

type ExistingRowStatus struct {
	Found              bool
	Applied            bool
	CurrentToken       uint64
	CurrentResourceKey string
	CurrentLockID      string
}

type rowScanner interface {
	Scan(dest ...any) error
}

func ScanExistingRowStatus(scanner rowScanner) (ExistingRowStatus, error) {
	var status ExistingRowStatus
	if err := scanner.Scan(
		&status.Found,
		&status.Applied,
		&status.CurrentToken,
		&status.CurrentResourceKey,
		&status.CurrentLockID,
	); err != nil {
		return ExistingRowStatus{}, err
	}

	return status, nil
}

func ClassifyExistingRowUpdate(g guard.Context, status ExistingRowStatus) (guard.Outcome, error) {
	switch {
	case !status.Found:
		return "", fmt.Errorf("%w: guarded row not found for %s", lockerrors.ErrInvariantRejected, g.ResourceKey)
	case status.Applied:
		return guard.OutcomeApplied, nil
	case status.CurrentLockID != g.LockID:
		return "", fmt.Errorf(
			"%w: guarded boundary mismatch want lock=%s got lock=%s",
			lockerrors.ErrInvariantRejected,
			g.LockID,
			status.CurrentLockID,
		)
	case status.CurrentResourceKey != g.ResourceKey:
		return "", fmt.Errorf(
			"%w: guarded boundary mismatch want=%s got=%s",
			lockerrors.ErrInvariantRejected,
			g.ResourceKey,
			status.CurrentResourceKey,
		)
	case status.CurrentToken >= g.FencingToken:
		return guard.OutcomeStaleRejected, nil
	default:
		return "", fmt.Errorf("%w: inconsistent guarded update state", lockerrors.ErrInvariantRejected)
	}
}
