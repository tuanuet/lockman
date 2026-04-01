# DefineHold Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `DefineHold[T]` as a third use case kind that acquires a detached TTL-bounded lease and returns a serializable token; a separate `Forfeit` call releases it from any process.

**Architecture:** Three-layer stack matching the Run/Claim pattern: `HoldUseCase[T]` (SDK layer) → `holds.Manager` (engine layer, new `lockkit/holds` package) → `backend.Driver` (unchanged). Token encoding lives in `internal/sdk/token.go` and is self-contained. The Hold/Run distinction exists only in the SDK layer; engine registry entries are standard `LockDefinition` with `ExecutionSync`.

**Tech Stack:** Go 1.22, `encoding/binary` (big-endian token payload), `encoding/base64` (RawURLEncoding), existing `backend.Driver`, `lockkit/registry`, `lockkit/definitions`.

---

## File Map

### New files

| File                            | Responsibility                                                                |
| ------------------------------- | ----------------------------------------------------------------------------- |
| `internal/sdk/token.go`         | `EncodeHoldToken` / `DecodeHoldToken` — self-contained binary+base64url codec |
| `internal/sdk/token_test.go`    | Round-trip, unknown prefix, N-key payloads                                    |
| `lockkit/holds/manager.go`      | `holds.Manager` — `Acquire`, `Release`, `Shutdown`, shutdown-aware            |
| `lockkit/holds/manager_test.go` | Manager unit tests                                                            |
| `lockman/usecase_hold.go`       | `HoldUseCase[T]`, `HoldHandle`, `DefineHold`, `.With()`, `.ForfeitWith()`     |
| `lockman/usecase_hold_test.go`  | Binding validation tests                                                      |
| `lockman/client_hold.go`        | `client.Hold()`, `client.Forfeit()`, request validation, error mapping        |
| `lockman/client_hold_test.go`   | Integration tests for Hold/Forfeit                                            |

### Modified files

| File                           | Change                                                                                                                                                              |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/sdk/usecase.go`      | Add `useCaseKindHold`, `UseCaseKindHold`, `'h'` delimiter                                                                                                           |
| `lockkit/definitions/types.go` | Add `DetachedAcquireRequest`, `DetachedReleaseRequest`                                                                                                              |
| `lockman/registry.go`          | Add `useCaseKindHold` constant                                                                                                                                      |
| `lockman/request.go`           | Add `HoldRequest`, `ForfeitRequest`                                                                                                                                 |
| `lockman/errors.go`            | Add `ErrHoldTokenInvalid`, `ErrHoldExpired`                                                                                                                         |
| `lockman/client.go`            | Add `holds *holds.Manager` field, init in `New()`, shutdown in `Shutdown()`                                                                                         |
| `lockman/client_validation.go` | Add `hasHoldUseCases`, Handle Hold in `buildClientPlan`, `normalizeUseCase`, `toSDKUseCaseKind`, `toExecutionKind`, `validateHoldRequest`, `validateForfeitRequest` |

---

## Task 1: Token codec (`internal/sdk/token.go`)

**Files:**

- Create: `internal/sdk/token_test.go`
- Create: `internal/sdk/token.go`

Token format: `h1_<base64url(payload)>` where payload (big-endian) encodes:

```
uint16  count of resource keys
  for each key:
    uint16  len(key)
    []byte  key bytes
uint16  len(ownerID)
[]byte  ownerID bytes
```

- [ ] **Step 1: Write failing tests**

Create `internal/sdk/token_test.go`:

```go
package sdk

import (
	"testing"
)

func TestHoldTokenRoundTrip(t *testing.T) {
	keys := []string{"contract:abc"}
	ownerID := "owner-1"

	token, err := EncodeHoldToken(keys, ownerID)
	if err != nil {
		t.Fatalf("EncodeHoldToken error: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	gotKeys, gotOwner, err := DecodeHoldToken(token)
	if err != nil {
		t.Fatalf("DecodeHoldToken error: %v", err)
	}
	if len(gotKeys) != len(keys) || gotKeys[0] != keys[0] {
		t.Fatalf("expected keys %v, got %v", keys, gotKeys)
	}
	if gotOwner != ownerID {
		t.Fatalf("expected ownerID %q, got %q", ownerID, gotOwner)
	}
}

func TestHoldTokenMultipleKeys(t *testing.T) {
	keys := []string{"contract:abc", "order:xyz", "user:999"}
	ownerID := "svc-worker-2"

	token, err := EncodeHoldToken(keys, ownerID)
	if err != nil {
		t.Fatalf("EncodeHoldToken error: %v", err)
	}

	gotKeys, gotOwner, err := DecodeHoldToken(token)
	if err != nil {
		t.Fatalf("DecodeHoldToken error: %v", err)
	}
	if len(gotKeys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(gotKeys))
	}
	for i, k := range keys {
		if gotKeys[i] != k {
			t.Fatalf("key[%d]: expected %q, got %q", i, k, gotKeys[i])
		}
	}
	if gotOwner != ownerID {
		t.Fatalf("expected ownerID %q, got %q", ownerID, gotOwner)
	}
}

func TestHoldTokenUnknownVersionPrefix(t *testing.T) {
	_, _, err := DecodeHoldToken("h2_somepayload")
	if err == nil {
		t.Fatal("expected error for unknown version prefix")
	}
}

func TestHoldTokenMalformedBase64(t *testing.T) {
	_, _, err := DecodeHoldToken("h1_!!!notbase64!!!")
	if err == nil {
		t.Fatal("expected error for malformed base64")
	}
}

func TestHoldTokenEmptyString(t *testing.T) {
	_, _, err := DecodeHoldToken("")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestHoldTokenHasExpectedPrefix(t *testing.T) {
	token, err := EncodeHoldToken([]string{"x:1"}, "o")
	if err != nil {
		t.Fatalf("EncodeHoldToken error: %v", err)
	}
	if len(token) < 3 || token[:3] != "h1_" {
		t.Fatalf("expected token to start with h1_, got %q", token)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/sdk/... -run TestHoldToken -v`
Expected: FAIL — `EncodeHoldToken` and `DecodeHoldToken` undefined

- [ ] **Step 3: Implement `internal/sdk/token.go`**

Create `internal/sdk/token.go`:

```go
package sdk

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

const holdTokenVersion = "h1_"

var errHoldTokenUnknownVersion = errors.New("sdk: hold token has unknown version prefix")
var errHoldTokenMalformed = errors.New("sdk: hold token payload is malformed")

// EncodeHoldToken encodes resourceKeys and ownerID into an opaque h1_ token string.
func EncodeHoldToken(resourceKeys []string, ownerID string) (string, error) {
	// Calculate payload size
	size := 2 // uint16 key count
	for _, k := range resourceKeys {
		size += 2 + len(k) // uint16 len + bytes
	}
	size += 2 + len(ownerID) // uint16 len + bytes

	buf := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint16(buf[offset:], uint16(len(resourceKeys)))
	offset += 2

	for _, k := range resourceKeys {
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(k)))
		offset += 2
		copy(buf[offset:], k)
		offset += len(k)
	}

	binary.BigEndian.PutUint16(buf[offset:], uint16(len(ownerID)))
	offset += 2
	copy(buf[offset:], ownerID)

	encoded := base64.RawURLEncoding.EncodeToString(buf)
	return holdTokenVersion + encoded, nil
}

// DecodeHoldToken decodes an h1_ token back into resourceKeys and ownerID.
func DecodeHoldToken(token string) (resourceKeys []string, ownerID string, err error) {
	if token == "" {
		return nil, "", fmt.Errorf("%w: empty token", errHoldTokenMalformed)
	}
	if !strings.HasPrefix(token, holdTokenVersion) {
		return nil, "", fmt.Errorf("%w: got prefix %q", errHoldTokenUnknownVersion, tokenVersionPrefix(token))
	}

	encoded := token[len(holdTokenVersion):]
	buf, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", fmt.Errorf("%w: base64 decode: %v", errHoldTokenMalformed, err)
	}

	offset := 0

	if offset+2 > len(buf) {
		return nil, "", fmt.Errorf("%w: truncated key count", errHoldTokenMalformed)
	}
	keyCount := int(binary.BigEndian.Uint16(buf[offset:]))
	offset += 2

	keys := make([]string, 0, keyCount)
	for i := 0; i < keyCount; i++ {
		if offset+2 > len(buf) {
			return nil, "", fmt.Errorf("%w: truncated key length at index %d", errHoldTokenMalformed, i)
		}
		keyLen := int(binary.BigEndian.Uint16(buf[offset:]))
		offset += 2
		if offset+keyLen > len(buf) {
			return nil, "", fmt.Errorf("%w: truncated key bytes at index %d", errHoldTokenMalformed, i)
		}
		keys = append(keys, string(buf[offset:offset+keyLen]))
		offset += keyLen
	}

	if offset+2 > len(buf) {
		return nil, "", fmt.Errorf("%w: truncated owner length", errHoldTokenMalformed)
	}
	ownerLen := int(binary.BigEndian.Uint16(buf[offset:]))
	offset += 2
	if offset+ownerLen > len(buf) {
		return nil, "", fmt.Errorf("%w: truncated owner bytes", errHoldTokenMalformed)
	}
	owner := string(buf[offset : offset+ownerLen])

	return keys, owner, nil
}

func tokenVersionPrefix(token string) string {
	if idx := strings.Index(token, "_"); idx > 0 {
		return token[:idx+1]
	}
	return token
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/sdk/... -run TestHoldToken -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/sdk/token.go internal/sdk/token_test.go
git commit -m "feat(sdk): add hold token encode/decode (h1_ format)"
```

---

## Task 2: Extend `internal/sdk` + `lockkit/definitions` for Hold kind

**Files:**

- Modify: `internal/sdk/usecase.go`
- Modify: `lockkit/definitions/types.go`

- [ ] **Step 1: Add `UseCaseKindHold` to `internal/sdk/usecase.go`**

Three targeted edits — do not append new const blocks; replace the existing ones in place.

**Edit 1** — In the unexported iota block (currently `useCaseKindRun`, `useCaseKindClaim`), append `useCaseKindHold` as the third constant. Replace the block:

```go
// BEFORE (lines ~10-13)
const (
	useCaseKindRun useCaseKind = iota + 1
	useCaseKindClaim
)

// AFTER
const (
	useCaseKindRun useCaseKind = iota + 1
	useCaseKindClaim
	useCaseKindHold
)
```

**Edit 2** — In the exported `UseCaseKind` const block (currently `UseCaseKindRun`, `UseCaseKindClaim`), append `UseCaseKindHold`. Replace the block:

```go
// BEFORE (lines ~26-29)
const (
	UseCaseKindRun   UseCaseKind = UseCaseKind(useCaseKindRun)
	UseCaseKindClaim UseCaseKind = UseCaseKind(useCaseKindClaim)
)

// AFTER
const (
	UseCaseKindRun   UseCaseKind = UseCaseKind(useCaseKindRun)
	UseCaseKindClaim UseCaseKind = UseCaseKind(useCaseKindClaim)
	UseCaseKindHold  UseCaseKind = UseCaseKind(useCaseKindHold)
)
```

**Edit 3** — In `nameDelimiter`, add the `useCaseKindHold` case. Replace the function:

```go
// BEFORE
func nameDelimiter(kind useCaseKind) byte {
	switch kind {
	case useCaseKindRun:
		return 'r'
	case useCaseKindClaim:
		return 'c'
	default:
		return 'u'
	}
}

// AFTER
func nameDelimiter(kind useCaseKind) byte {
	switch kind {
	case useCaseKindRun:
		return 'r'
	case useCaseKindClaim:
		return 'c'
	case useCaseKindHold:
		return 'h'
	default:
		return 'u'
	}
}
```

- [ ] **Step 2: Add `DetachedAcquireRequest` and `DetachedReleaseRequest` to `lockkit/definitions/types.go`**

Append at the end of `lockkit/definitions/types.go`:

```go
// DetachedAcquireRequest carries inputs for acquiring a detached (hold) lease.
// LeaseTTL is resolved from the LockDefinition in the registry, not carried here.
type DetachedAcquireRequest struct {
	DefinitionID string
	ResourceKeys []string
	OwnerID      string
}

// DetachedReleaseRequest carries inputs for releasing a detached (hold) lease.
type DetachedReleaseRequest struct {
	DefinitionID string
	ResourceKeys []string
	OwnerID      string
}
```

- [ ] **Step 3: Run existing tests to verify no regressions**

Run: `go test ./internal/sdk/... ./lockkit/definitions/...`
Expected: all PASS (including existing SDK tests for ID hashing and capability checks)

- [ ] **Step 4: Commit**

```bash
git add internal/sdk/usecase.go lockkit/definitions/types.go
git commit -m "feat(sdk): add UseCaseKindHold and detached request types"
```

---

```

```

## Task 3: Sentinel errors

**Files:**

- Modify: `lockman/errors.go`

- [ ] **Step 1: Add `ErrHoldTokenInvalid` and `ErrHoldExpired` to `lockman/errors.go`**

Append to the `var (...)` block:

```go
ErrHoldTokenInvalid = errors.New("lockman: hold token is malformed or unrecognized")
ErrHoldExpired      = errors.New("lockman: hold lease has expired")
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add lockman/errors.go
git commit -m "feat: add ErrHoldTokenInvalid and ErrHoldExpired sentinel errors"
```

---

## Task 4: `lockkit/holds` Manager

**Files:**

- Create: `lockkit/holds/manager_test.go`
- Create: `lockkit/holds/manager.go`

The `holds.Manager` is simpler than `runtime.Manager` — `Acquire` is an instant backend call with no callback, so no in-flight drain is needed. `Shutdown()` sets an atomic flag; subsequent `Acquire` calls are rejected.

- [ ] **Step 1: Write failing tests**

Create `lockkit/holds/manager_test.go`:

```go
package holds

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	lockregistry "github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/lockkit/testkit"
)

func newTestRegistry(t *testing.T, defs ...definitions.LockDefinition) *lockregistry.Registry {
	t.Helper()
	reg := lockregistry.New()
	for _, def := range defs {
		if err := reg.Register(def); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	return reg
}

func testHoldDefinition(id string, ttl time.Duration) definitions.LockDefinition {
	return definitions.LockDefinition{
		ID:            id,
		Kind:          definitions.KindParent,
		Resource:      id,
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      ttl,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
}

func TestHoldsManagerAcquireReturnsLease(t *testing.T) {
	defID := "sdk_uc_hold_test"
	reg := newTestRegistry(t, testHoldDefinition(defID, 5*time.Minute))
	drv := testkit.NewMemoryDriver()

	mgr, err := NewManager(reg, drv)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	lease, err := mgr.Acquire(context.Background(), definitions.DetachedAcquireRequest{
		DefinitionID: defID,
		ResourceKeys: []string{"contract:abc"},
		OwnerID:      "owner-1",
	})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if lease.DefinitionID != defID {
		t.Fatalf("expected definition ID %q, got %q", defID, lease.DefinitionID)
	}
	if lease.OwnerID != "owner-1" {
		t.Fatalf("expected ownerID %q, got %q", "owner-1", lease.OwnerID)
	}
	if lease.LeaseTTL != 5*time.Minute {
		t.Fatalf("expected TTL %v, got %v", 5*time.Minute, lease.LeaseTTL)
	}
}

func TestHoldsManagerReleaseCallsBackend(t *testing.T) {
	defID := "sdk_uc_hold_release"
	reg := newTestRegistry(t, testHoldDefinition(defID, 5*time.Minute))
	drv := testkit.NewMemoryDriver()

	mgr, err := NewManager(reg, drv)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	req := definitions.DetachedAcquireRequest{
		DefinitionID: defID,
		ResourceKeys: []string{"contract:abc"},
		OwnerID:      "owner-1",
	}
	if _, err := mgr.Acquire(context.Background(), req); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	err = mgr.Release(context.Background(), definitions.DetachedReleaseRequest{
		DefinitionID: defID,
		ResourceKeys: []string{"contract:abc"},
		OwnerID:      "owner-1",
	})
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	// After release, the same resource can be re-acquired by a different owner.
	_, err = mgr.Acquire(context.Background(), definitions.DetachedAcquireRequest{
		DefinitionID: defID,
		ResourceKeys: []string{"contract:abc"},
		OwnerID:      "owner-2",
	})
	if err != nil {
		t.Fatalf("re-Acquire after Release: %v", err)
	}
}

func TestHoldsManagerAcquireAfterShutdownRejected(t *testing.T) {
	defID := "sdk_uc_hold_shutdown"
	reg := newTestRegistry(t, testHoldDefinition(defID, 5*time.Minute))
	drv := testkit.NewMemoryDriver()

	mgr, err := NewManager(reg, drv)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	mgr.Shutdown()

	_, err = mgr.Acquire(context.Background(), definitions.DetachedAcquireRequest{
		DefinitionID: defID,
		ResourceKeys: []string{"contract:abc"},
		OwnerID:      "owner-1",
	})
	if err == nil {
		t.Fatal("expected error after Shutdown, got nil")
	}
	if !isErrPolicyViolation(err) {
		t.Fatalf("expected ErrPolicyViolation, got %v", err)
	}
}

func TestHoldsManagerAcquireUnknownDefinitionRejected(t *testing.T) {
	reg := newTestRegistry(t) // empty registry
	drv := testkit.NewMemoryDriver()

	mgr, err := NewManager(reg, drv)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	_, err = mgr.Acquire(context.Background(), definitions.DetachedAcquireRequest{
		DefinitionID: "sdk_uc_nonexistent",
		ResourceKeys: []string{"contract:abc"},
		OwnerID:      "owner-1",
	})
	if err == nil {
		t.Fatal("expected error for unknown definition, got nil")
	}
	if !isErrPolicyViolation(err) {
		t.Fatalf("expected ErrPolicyViolation, got %v", err)
	}
}

func TestHoldsManagerNilDriverRejected(t *testing.T) {
	reg := newTestRegistry(t)
	_, err := NewManager(reg, nil)
	if err == nil {
		t.Fatal("expected error for nil driver, got nil")
	}
}

func isErrPolicyViolation(err error) bool {
	return errors.Is(err, lockerrors.ErrPolicyViolation)
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./lockkit/holds/... -v`
Expected: FAIL — package `holds` does not exist yet

- [ ] **Step 3: Implement `lockkit/holds/manager.go`**

Create `lockkit/holds/manager.go`:

```go
package holds

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/registry"
)

// Manager orchestrates detached lease acquisition and release for Hold use cases.
// Acquire is an instant backend call — no callback, no in-flight drain needed.
type Manager struct {
	registry     registry.Reader
	driver       backend.Driver
	shuttingDown atomic.Bool
}

// NewManager validates the registry and returns a configured holds manager.
func NewManager(reg registry.Reader, driver backend.Driver) (*Manager, error) {
	validator, ok := reg.(interface{ Validate() error })
	if !ok {
		return nil, fmt.Errorf("%w: invalid registry", lockerrors.ErrRegistryViolation)
	}
	if err := validator.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", lockerrors.ErrRegistryViolation, err)
	}
	if driver == nil {
		return nil, lockerrors.ErrPolicyViolation
	}
	return &Manager{
		registry: reg,
		driver:   driver,
	}, nil
}

// Acquire obtains a detached lease for the given request.
// It rejects new acquisitions if the manager is shutting down.
func (m *Manager) Acquire(ctx context.Context, req definitions.DetachedAcquireRequest) (backend.LeaseRecord, error) {
	if m.shuttingDown.Load() {
		return backend.LeaseRecord{}, lockerrors.ErrPolicyViolation
	}

	def, err := m.getDefinition(req.DefinitionID)
	if err != nil {
		return backend.LeaseRecord{}, err
	}

	return m.driver.Acquire(ctx, backend.AcquireRequest{
		DefinitionID: req.DefinitionID,
		ResourceKeys: req.ResourceKeys,
		OwnerID:      req.OwnerID,
		LeaseTTL:     def.LeaseTTL,
	})
}

// Release surrenders a previously acquired detached lease.
func (m *Manager) Release(ctx context.Context, req definitions.DetachedReleaseRequest) error {
	return m.driver.Release(ctx, backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: req.ResourceKeys,
		OwnerID:      req.OwnerID,
	})
}

// Shutdown marks the manager as unavailable for new acquisitions.
// In-flight Acquire calls are instant backend operations — no drain required.
func (m *Manager) Shutdown() {
	m.shuttingDown.Store(true)
}

func (m *Manager) getDefinition(id string) (def definitions.LockDefinition, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Join(lockerrors.ErrPolicyViolation, fmt.Errorf("unknown definition %q", id))
		}
	}()
	def = m.registry.MustGet(id)
	return def, nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./lockkit/holds/... -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/holds/manager.go lockkit/holds/manager_test.go
git commit -m "feat(holds): add holds.Manager with Acquire, Release, Shutdown"
```

---

## Task 5: SDK-layer Hold types (`registry.go`, `request.go`, `usecase_hold.go`)

**Files:**

- Modify: `lockman/registry.go`
- Modify: `lockman/request.go`
- Create: `lockman/usecase_hold_test.go`
- Create: `lockman/usecase_hold.go`

- [ ] **Step 1: Add `useCaseKindHold` to `lockman/registry.go`**

In `registry.go`, **replace** the existing `useCaseKind` iota block (currently `useCaseKindRun`, `useCaseKindClaim`) — do not add a second block:

```go
// BEFORE (lines ~13-16)
const (
	useCaseKindRun useCaseKind = iota + 1
	useCaseKindClaim
)

// AFTER
const (
	useCaseKindRun useCaseKind = iota + 1
	useCaseKindClaim
	useCaseKindHold
)
```

- [ ] **Step 2: Add `HoldRequest` and `ForfeitRequest` to `lockman/request.go`**

Append to `request.go`:

```go
// HoldRequest is an opaque request produced by HoldUseCase.With.
type HoldRequest struct {
	useCaseName     string
	resourceKey     string
	ownerID         string
	useCaseCore     *useCaseCore
	registryLink    sdk.RegistryLink
	boundToRegistry bool
}

// ForfeitRequest is an opaque request produced by HoldUseCase.ForfeitWith.
// The token is carried raw; decoding happens inside client.Forfeit.
type ForfeitRequest struct {
	useCaseName     string
	token           string
	useCaseCore     *useCaseCore
	registryLink    sdk.RegistryLink
	boundToRegistry bool
}
```

- [ ] **Step 3: Write failing tests for `HoldUseCase`**

Create `lockman/usecase_hold_test.go`:

```go
package lockman

import (
	"errors"
	"testing"
)

func testHoldUseCase(name string) HoldUseCase[string] {
	return DefineHold[string](
		name,
		BindResourceID("contract", func(v string) string { return v }),
	)
}

func TestHoldUseCaseWithBindsCanonicalResourceKey(t *testing.T) {
	uc := testHoldUseCase("contract.batch_delete")
	mustRegisterUseCases(t, NewRegistry(), uc)

	req, err := uc.With("abc")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	if req.resourceKey != "contract:abc" {
		t.Fatalf("expected canonical resource key, got %q", req.resourceKey)
	}
}

func TestHoldUseCaseWithRejectsEmptyBoundResourceID(t *testing.T) {
	uc := DefineHold[string](
		"contract.batch_delete",
		BindResourceID("contract", func(v string) string { return v }),
	)

	_, err := uc.With("")
	if err == nil {
		t.Fatal("expected error for empty resource ID")
	}
	if !errors.Is(err, errEmptyBindingValue) {
		t.Fatalf("expected errEmptyBindingValue, got %v", err)
	}
}

func TestHoldUseCaseWithRejectsNilBinding(t *testing.T) {
	uc := HoldUseCase[string]{} // zero value — core is nil

	_, err := uc.With("abc")
	if err == nil {
		t.Fatal("expected error for nil use case")
	}
}

func TestHoldUseCaseWithRejectsEmptyOwnerOverride(t *testing.T) {
	uc := testHoldUseCase("contract.batch_delete")

	_, err := uc.With("abc", OwnerID("   "))
	if err == nil {
		t.Fatal("expected error for empty owner override")
	}
	if !errors.Is(err, ErrIdentityRequired) {
		t.Fatalf("expected ErrIdentityRequired, got %v", err)
	}
}

func TestHoldUseCaseWithSetsBoundToRegistryAfterRegistration(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCase("contract.batch_delete")
	mustRegisterUseCases(t, reg, uc)

	req, err := uc.With("abc")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	if !req.boundToRegistry {
		t.Fatal("expected boundToRegistry to be true after registration")
	}
}

func TestHoldUseCaseWithNotBoundToRegistryBeforeRegistration(t *testing.T) {
	uc := testHoldUseCase("contract.batch_delete")

	req, err := uc.With("abc")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	if req.boundToRegistry {
		t.Fatal("expected boundToRegistry to be false before registration")
	}
}

func TestHoldUseCaseForfeitWithPackagesToken(t *testing.T) {
	uc := testHoldUseCase("contract.batch_delete")
	token := "h1_sometoken"

	req := uc.ForfeitWith(token)
	if req.token != token {
		t.Fatalf("expected token %q, got %q", token, req.token)
	}
}
```

- [ ] **Step 4: Run tests to verify failure**

Run: `go test ./... -run 'TestHoldUseCase' -v`
Expected: FAIL — `HoldUseCase`, `DefineHold`, etc. undefined

- [ ] **Step 5: Implement `lockman/usecase_hold.go`**

Create `lockman/usecase_hold.go`:

```go
package lockman

import "fmt"

// HoldHandle carries the opaque token for a detached lease.
// It exposes only Token() — no other state is accessible.
type HoldHandle struct {
	token string
}

// Token returns the opaque serializable token for this hold.
// Store this in job metadata; pass it to ForfeitWith to release the hold.
func (h HoldHandle) Token() string {
	return h.token
}

// HoldUseCase defines a typed detached lease use case.
type HoldUseCase[T any] struct {
	core    *useCaseCore
	binding Binding[T]
}

// DefineHold declares a typed hold use case.
func DefineHold[T any](name string, binding Binding[T], opts ...UseCaseOption) HoldUseCase[T] {
	return HoldUseCase[T]{
		core:    newUseCaseCore(name, useCaseKindHold, opts...),
		binding: binding,
	}
}

// With binds typed input into an opaque hold request.
func (u HoldUseCase[T]) With(input T, opts ...CallOption) (HoldRequest, error) {
	if u.core == nil {
		return HoldRequest{}, fmt.Errorf("lockman: hold use case is not defined")
	}
	if u.binding.build == nil {
		return HoldRequest{}, fmt.Errorf("lockman: hold use case binding is required")
	}

	call := applyCallOptions(opts...)
	if call.ownerIDSet && call.ownerID == "" {
		return HoldRequest{}, fmt.Errorf("lockman: owner override is required: %w", ErrIdentityRequired)
	}

	resourceKey, err := u.binding.build(input)
	if err != nil {
		return HoldRequest{}, fmt.Errorf("lockman: bind hold request: %w", err)
	}

	req := HoldRequest{
		useCaseName: u.core.name,
		resourceKey: resourceKey,
		ownerID:     call.ownerID,
		useCaseCore: u.core,
	}
	if u.core.registry != nil {
		req.registryLink = u.core.registry.link
		req.boundToRegistry = true
	}

	return req, nil
}

// ForfeitWith packages a raw token string into a ForfeitRequest.
// Decoding of the token happens inside client.Forfeit — not here.
func (u HoldUseCase[T]) ForfeitWith(token string) ForfeitRequest {
	req := ForfeitRequest{
		token:       token,
		useCaseCore: u.core,
	}
	if u.core != nil {
		req.useCaseName = u.core.name
		if u.core.registry != nil {
			req.registryLink = u.core.registry.link
			req.boundToRegistry = true
		}
	}
	return req
}

func (u HoldUseCase[T]) sdkUseCase() *useCaseCore {
	return u.core
}
```

- [ ] **Step 6: Run tests to verify pass**

Run: `go test ./... -run 'TestHoldUseCase' -v`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add lockman/registry.go lockman/request.go lockman/usecase_hold.go lockman/usecase_hold_test.go
git commit -m "feat: add HoldUseCase, DefineHold, HoldRequest, ForfeitRequest"
```

---

## Task 6: `client_validation.go` Hold branch

**Files:**

- Modify: `lockman/client_validation.go`

This wires Hold use cases into the existing validation pipeline so they register as engine definitions and `holds.Manager` gets constructed.

- [ ] **Step 1: Add `hasHoldUseCases` to `clientPlan`**

In `client_validation.go`, extend the `clientPlan` struct:

```go
type clientPlan struct {
	engineRegistry   *lockregistry.Registry
	hasRunUseCases   bool
	hasClaimUseCases bool
	hasHoldUseCases  bool
}
```

- [ ] **Step 2: Set `hasHoldUseCases` in `buildClientPlan`**

In the loop where `hasRunUseCases` and `hasClaimUseCases` are set:

```go
if useCase.kind == useCaseKindRun {
    plan.hasRunUseCases = true
}
if useCase.kind == useCaseKindClaim {
    plan.hasClaimUseCases = true
}
if useCase.kind == useCaseKindHold {
    plan.hasHoldUseCases = true
}
```

- [ ] **Step 3: Map `useCaseKindHold` in `toSDKUseCaseKind` and `toExecutionKind`**

Update `toSDKUseCaseKind`:

```go
func toSDKUseCaseKind(kind useCaseKind) sdk.UseCaseKind {
	switch kind {
	case useCaseKindClaim:
		return sdk.UseCaseKindClaim
	case useCaseKindHold:
		return sdk.UseCaseKindHold
	default:
		return sdk.UseCaseKindRun
	}
}
```

Update `toExecutionKind`:

```go
func toExecutionKind(kind useCaseKind) definitions.ExecutionKind {
	if kind == useCaseKindClaim {
		return definitions.ExecutionAsync
	}
	return definitions.ExecutionSync
}
```

`useCaseKindHold` maps to `ExecutionSync` via the default — no change needed here.

- [ ] **Step 4: Add `validateHoldRequest` and `validateForfeitRequest`**

Append to `client_validation.go`:

```go
func (c *Client) validateHoldRequest(ctx context.Context, req HoldRequest) (Identity, error) {
	if req.useCaseCore == nil {
		return Identity{}, ErrUseCaseNotFound
	}
	if !req.boundToRegistry {
		return Identity{}, fmt.Errorf("lockman: use case %q is not registered: %w", req.useCaseName, ErrUseCaseNotFound)
	}
	if c.registry == nil || sdk.RegistryLinkMismatch(c.registry.link, req.registryLink) {
		return Identity{}, fmt.Errorf("lockman: use case %q belongs to a different registry: %w", req.useCaseName, ErrRegistryMismatch)
	}
	return c.resolveIdentity(ctx, req.ownerID)
}

func (c *Client) validateForfeitRequest(req ForfeitRequest) error {
	if req.useCaseCore == nil {
		return ErrUseCaseNotFound
	}
	if !req.boundToRegistry {
		return fmt.Errorf("lockman: use case %q is not registered: %w", req.useCaseName, ErrUseCaseNotFound)
	}
	if c.registry == nil || sdk.RegistryLinkMismatch(c.registry.link, req.registryLink) {
		return fmt.Errorf("lockman: use case %q belongs to a different registry: %w", req.useCaseName, ErrRegistryMismatch)
	}
	return nil
}
```

- [ ] **Step 5: Verify build passes**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add lockman/client_validation.go
git commit -m "feat: wire Hold use cases into client validation pipeline"
```

---

## Task 7: `client.go` holds field and `client_hold.go`

**Files:**

- Modify: `lockman/client.go`
- Create: `lockman/client_hold_test.go`
- Create: `lockman/client_hold.go`

### 7a: Extend `client.go`

- [ ] **Step 1: Add `holds` field to `Client` and init in `New()`**

Import `holds` package in `client.go`:

```go
import (
	"context"
	"sync/atomic"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/idempotency"
	"github.com/tuanuet/lockman/lockkit/holds"
	lockruntime "github.com/tuanuet/lockman/lockkit/runtime"
	"github.com/tuanuet/lockman/lockkit/workers"
)
```

Add `holds` field to `Client`:

```go
type Client struct {
	registry         *Registry
	backend          backend.Driver
	idempotency      idempotency.Store
	identity         Identity
	identityProvider func(context.Context) Identity
	runtime          *lockruntime.Manager
	worker           *workers.Manager
	holds            *holds.Manager
	shuttingDown     atomic.Bool
}
```

In `New()`, after the `hasClaimUseCases` block, add:

```go
if plan.hasHoldUseCases {
    client.holds, err = holds.NewManager(plan.engineRegistry, cfg.backend)
    if err != nil {
        return nil, wrapStartupManagerError("holds", err)
    }
}
```

- [ ] **Step 2: Call `holds.Shutdown()` in `Client.Shutdown()`**

After the existing runtime/worker shutdown calls:

```go
if c.holds != nil {
    c.holds.Shutdown()
}
```

### 7b: Write failing tests for `client.Hold` and `client.Forfeit`

- [ ] **Step 3: Create `lockman/client_hold_test.go`**

```go
package lockman

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuanuet/lockman/lockkit/testkit"
)

func testHoldUseCaseHelper(name string) HoldUseCase[string] {
	return DefineHold[string](
		name,
		BindResourceID("contract", func(v string) string { return v }),
		TTL(15*time.Minute),
	)
}

func mustNewHoldClient(t *testing.T, reg *Registry) *Client {
	t.Helper()
	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return client
}

func TestHoldReturnsHandleWithToken(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCaseHelper("contract.batch_delete")
	mustRegisterUseCases(t, reg, uc)
	client := mustNewHoldClient(t, reg)

	req, err := uc.With("abc")
	if err != nil {
		t.Fatalf("With: %v", err)
	}

	handle, err := client.Hold(context.Background(), req)
	if err != nil {
		t.Fatalf("Hold: %v", err)
	}
	if handle.Token() == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestHoldTokenHasExpectedPrefix(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCaseHelper("contract.batch_delete")
	mustRegisterUseCases(t, reg, uc)
	client := mustNewHoldClient(t, reg)

	req, _ := uc.With("abc")
	handle, err := client.Hold(context.Background(), req)
	if err != nil {
		t.Fatalf("Hold: %v", err)
	}

	token := handle.Token()
	if len(token) < 3 || token[:3] != "h1_" {
		t.Fatalf("expected token to start with h1_, got %q", token)
	}
}

func TestForfeitReleasesHold(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCaseHelper("contract.batch_delete")
	mustRegisterUseCases(t, reg, uc)
	client := mustNewHoldClient(t, reg)

	req, _ := uc.With("abc")
	handle, err := client.Hold(context.Background(), req)
	if err != nil {
		t.Fatalf("Hold: %v", err)
	}

	forfeit := uc.ForfeitWith(handle.Token())
	if err := client.Forfeit(context.Background(), forfeit); err != nil {
		t.Fatalf("Forfeit: %v", err)
	}

	// After forfeit, the same resource should be acquirable again.
	req2, _ := uc.With("abc", OwnerID("owner-2"))
	if _, err := client.Hold(context.Background(), req2); err != nil {
		t.Fatalf("re-Hold after Forfeit: %v", err)
	}
}

func TestHoldRejectsRequestFromDifferentRegistry(t *testing.T) {
	regA := NewRegistry()
	regB := NewRegistry()
	ucA := testHoldUseCaseHelper("contract.approve")
	ucB := testHoldUseCaseHelper("contract.delete")
	mustRegisterUseCases(t, regA, ucA)
	mustRegisterUseCases(t, regB, ucB)

	client := mustNewHoldClient(t, regA)

	req, _ := ucB.With("abc")
	_, err := client.Hold(context.Background(), req)
	if !errors.Is(err, ErrRegistryMismatch) {
		t.Fatalf("expected ErrRegistryMismatch, got %v", err)
	}
}

func TestHoldRejectsUnregisteredUseCase(t *testing.T) {
	reg := NewRegistry()
	registered := testHoldUseCaseHelper("contract.approve")
	unregistered := testHoldUseCaseHelper("contract.delete")
	mustRegisterUseCases(t, reg, registered)
	client := mustNewHoldClient(t, reg)

	req, _ := unregistered.With("abc")
	_, err := client.Hold(context.Background(), req)
	if !errors.Is(err, ErrUseCaseNotFound) {
		t.Fatalf("expected ErrUseCaseNotFound, got %v", err)
	}
}

func TestForfeitWithMalformedTokenReturnsErrHoldTokenInvalid(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCaseHelper("contract.batch_delete")
	mustRegisterUseCases(t, reg, uc)
	client := mustNewHoldClient(t, reg)

	forfeit := uc.ForfeitWith("not-a-valid-token")
	err := client.Forfeit(context.Background(), forfeit)
	if !errors.Is(err, ErrHoldTokenInvalid) {
		t.Fatalf("expected ErrHoldTokenInvalid, got %v", err)
	}
}

func TestForfeitAlreadyReleasedLeaseReturnsErrHoldExpired(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCaseHelper("contract.batch_delete")
	mustRegisterUseCases(t, reg, uc)
	client := mustNewHoldClient(t, reg)

	// Acquire a lease and forfeit it once; the token is now for a released lease.
	req, _ := uc.With("abc")
	handle, err := client.Hold(context.Background(), req)
	if err != nil {
		t.Fatalf("Hold: %v", err)
	}
	token := handle.Token()

	// First forfeit releases the lease successfully.
	if err := client.Forfeit(context.Background(), uc.ForfeitWith(token)); err != nil {
		t.Fatalf("first Forfeit: %v", err)
	}

	// Second forfeit — backend returns ErrLeaseNotFound (lease already gone).
	// mapHoldReleaseError maps ErrLeaseNotFound → ErrHoldExpired.
	err = client.Forfeit(context.Background(), uc.ForfeitWith(token))
	if !errors.Is(err, ErrHoldExpired) {
		t.Fatalf("expected ErrHoldExpired, got %v", err)
	}
}

func TestForfeitRejectsUnregisteredUseCase(t *testing.T) {
	reg := NewRegistry()
	registered := testHoldUseCaseHelper("contract.approve")
	unregistered := testHoldUseCaseHelper("contract.delete")
	mustRegisterUseCases(t, reg, registered)
	client := mustNewHoldClient(t, reg)

	forfeit := unregistered.ForfeitWith("h1_sometoken")
	err := client.Forfeit(context.Background(), forfeit)
	if !errors.Is(err, ErrUseCaseNotFound) {
		t.Fatalf("expected ErrUseCaseNotFound, got %v", err)
	}
}
```

- [ ] **Step 4: Run tests to verify failure**

Run: `go test ./... -run 'TestHold|TestForfeit' -v`
Expected: FAIL — `client.Hold` and `client.Forfeit` undefined; may also have compile errors for missing `holds` import

- [ ] **Step 5: Implement `lockman/client_hold.go`**

Create `lockman/client_hold.go`:

```go
package lockman

import (
	"context"
	"errors"
	"fmt"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/internal/sdk"
	"github.com/tuanuet/lockman/lockkit/definitions"
)

// Hold acquires a detached lease and returns a HoldHandle whose Token can be stored
// in job metadata and later passed to Forfeit to release the lease.
func (c *Client) Hold(ctx context.Context, req HoldRequest) (HoldHandle, error) {
	if c == nil {
		return HoldHandle{}, fmt.Errorf("lockman: client is nil")
	}
	if c.shuttingDown.Load() {
		return HoldHandle{}, ErrShuttingDown
	}

	identity, err := c.validateHoldRequest(ctx, req)
	if err != nil {
		return HoldHandle{}, err
	}
	if c.holds == nil {
		return HoldHandle{}, ErrUseCaseNotFound
	}

	normalized := normalizeUseCase(req.useCaseCore, map[string]int{}, req.registryLink)
	definitionID := normalized.DefinitionID()

	ownerID := identity.OwnerID

	_, err = c.holds.Acquire(ctx, definitions.DetachedAcquireRequest{
		DefinitionID: definitionID,
		ResourceKeys: []string{req.resourceKey},
		OwnerID:      ownerID,
	})
	if err != nil {
		return HoldHandle{}, mapHoldAcquireError(err, c.shuttingDown.Load())
	}

	token, err := sdk.EncodeHoldToken([]string{req.resourceKey}, ownerID)
	if err != nil {
		return HoldHandle{}, fmt.Errorf("lockman: encode hold token: %w", err)
	}

	return HoldHandle{token: token}, nil
}

// Forfeit releases a detached lease identified by the token stored in the ForfeitRequest.
func (c *Client) Forfeit(ctx context.Context, req ForfeitRequest) error {
	if c == nil {
		return fmt.Errorf("lockman: client is nil")
	}
	if c.shuttingDown.Load() {
		return ErrShuttingDown
	}

	if err := c.validateForfeitRequest(req); err != nil {
		return err
	}
	if c.holds == nil {
		return ErrUseCaseNotFound
	}

	resourceKeys, ownerID, err := sdk.DecodeHoldToken(req.token)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHoldTokenInvalid, err)
	}

	normalized := normalizeUseCase(req.useCaseCore, map[string]int{}, req.registryLink)
	definitionID := normalized.DefinitionID()

	err = c.holds.Release(ctx, definitions.DetachedReleaseRequest{
		DefinitionID: definitionID,
		ResourceKeys: resourceKeys,
		OwnerID:      ownerID,
	})
	if err != nil {
		return mapHoldReleaseError(err)
	}

	return nil
}

func mapHoldAcquireError(err error, shuttingDown bool) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, backend.ErrLeaseAlreadyHeld):
		return ErrBusy
	case errors.Is(err, backend.ErrLeaseOwnerMismatch):
		return ErrBusy
	case shuttingDown:
		return ErrShuttingDown
	default:
		return err
	}
}

func mapHoldReleaseError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, backend.ErrLeaseNotFound), errors.Is(err, backend.ErrLeaseExpired):
		return ErrHoldExpired
	case errors.Is(err, backend.ErrLeaseOwnerMismatch):
		return ErrBusy
	default:
		return err
	}
}
```

- [ ] **Step 6: Run tests to verify pass**

Run: `go test ./... -run 'TestHold|TestForfeit' -v`
Expected: all PASS

- [ ] **Step 7: Run full test suite**

Run: `go test ./...`
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add lockman/client.go lockman/client_hold.go lockman/client_hold_test.go
git commit -m "feat: add client.Hold and client.Forfeit for detached lease management"
```

---

## Final Verification

- [ ] **Run full test suite one more time**

Run: `go test ./...`
Expected: all PASS, no build errors

- [ ] **Verify public API matches the spec**

The following should compile without errors (not a test file — just a mental check):

```go
// Definition
var ContractBatchDelete = lockman.DefineHold[struct{ EntityID string }](
    "contract.batch_delete",
    lockman.BindResourceID("contract", func(in struct{ EntityID string }) string { return in.EntityID }),
    lockman.TTL(15*time.Minute),
)

// Producer
req, err := ContractBatchDelete.With(struct{ EntityID string }{EntityID: "abc"})
handle, err := client.Hold(ctx, req)
token := handle.Token()

// Consumer
forfeit := ContractBatchDelete.ForfeitWith(token)
err = client.Forfeit(ctx, forfeit)
```
