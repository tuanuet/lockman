// Package postgres is a temporary compatibility shim.
//
// Deprecated: new code should import the extracted adapter module at
// `lockman/guard/postgres`.
package postgres

import (
	"lockman/guard"
	pgguard "lockman/guard/postgres"
)

// ExistingRowStatus is a type alias to the extracted adapter status type.
type ExistingRowStatus = pgguard.ExistingRowStatus

// ScanExistingRowStatus decodes the guarded-row query result.
//
// Deprecated: use `lockman/guard/postgres.ScanExistingRowStatus` directly.
func ScanExistingRowStatus(scanner interface{ Scan(dest ...any) error }) (ExistingRowStatus, error) {
	return pgguard.ScanExistingRowStatus(scanner)
}

// ClassifyExistingRowUpdate classifies a guarded existing-row update result.
//
// Deprecated: use `lockman/guard/postgres.ClassifyExistingRowUpdate` directly.
func ClassifyExistingRowUpdate(g guard.Context, status ExistingRowStatus) (guard.Outcome, error) {
	return pgguard.ClassifyExistingRowUpdate(g, status)
}
