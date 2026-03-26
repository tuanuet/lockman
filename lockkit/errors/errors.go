package lockerrors

import "errors"

var (
	ErrLockBusy           = errors.New("lock busy")
	ErrLockAcquireTimeout = errors.New("lock acquire timeout")
	ErrLeaseLost          = errors.New("lease lost")
	ErrRegistryViolation  = errors.New("registry violation")
	ErrPolicyViolation    = errors.New("policy violation")
	ErrReentrantAcquire   = errors.New("reentrant acquire")
)
