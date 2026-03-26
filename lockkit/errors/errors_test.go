package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrReentrantAcquireMatchesWithErrorsIs(t *testing.T) {
	err := fmt.Errorf("runtime rejected acquire: %w", ErrReentrantAcquire)
	if !errors.Is(err, ErrReentrantAcquire) {
		t.Fatal("expected ErrReentrantAcquire to match with errors.Is")
	}
}
