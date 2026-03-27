# Lock Management Platform Phase 2a Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Phase 2a of the lock management platform SDK for Go so parent-child lineage is enforced for single-lock and composite execution paths across processes, with Redis and in-memory lineage-aware backends.

**Architecture:** Phase 2a stays registry-first. The implementation should first make lineage structurally provable at definition and registry time, then derive reusable lineage metadata in one shared internal package, then extend backend contracts for atomic lineage acquire/renew/release, and finally route runtime and worker execution through those lineage-aware paths only when a definition participates in a validated parent chain. Composite execution must reuse the same lineage path so it cannot bypass single-lock enforcement.

**Tech Stack:** Go 1.22+, standard library, `testing` package, existing Redis driver based on `github.com/redis/go-redis/v9`, local Docker Compose Redis for integration verification

---

## Planned File Structure

### Existing files to extend

- `README.md`: document Phase 2a behavior, migration caveats, and verification commands
- `examples/phase2-parent-child-runtime/main.go`: refresh example text so it demonstrates enforced parent-child rejection instead of pre-Phase-2a ambiguity
- `examples/phase2-parent-child-runtime/main_test.go`: keep example assertions aligned with new runtime behavior
- `lockkit/definitions/key_builder.go`: expose safe template-backed metadata needed for recursive lineage validation
- `lockkit/definitions/key_builder_test.go`: cover exported template metadata access and builder invariants
- `lockkit/drivers/contracts.go`: add lineage-specific driver capability and request/meta types without widening the base `Driver` contract
- `lockkit/drivers/contracts_phase2_test.go`: assert the base driver remains exact-key and the new optional lineage capability is shaped correctly
- `lockkit/drivers/redis/driver.go`: implement `LineageDriver` on the Redis backend
- `lockkit/drivers/redis/scripts.go`: add atomic Lua scripts and key helpers for lineage acquire, renew, and release
- `lockkit/drivers/redis/driver_integration_test.go`: verify cross-client lineage overlap, renewal, and cleanup behavior against real Redis
- `lockkit/errors/errors.go`: add a distinct runtime overlap error for lineage rejection
- `lockkit/observe/contracts.go`: add a dedicated overlap rejection recorder hook if Phase 2a observability is implemented through the existing recorder surface
- `lockkit/internal/policy/outcome.go`: map runtime overlap to `retry` without changing existing registry-invalidity handling
- `lockkit/registry/registry.go`: expose enough definition snapshot data for manager capability checks if current reader surface is too narrow
- `lockkit/registry/registry_test.go`: cover registry validation and snapshot behavior for lineage-aware definitions
- `lockkit/registry/validation.go`: enforce recursive parent-chain validity, hierarchical template rules, and cycle detection
- `lockkit/runtime/composite.go`: route lineage-aware members through `LineageDriver`
- `lockkit/runtime/composite_test.go`: cover composite routing and cross-process parent-child overlap rejection
- `lockkit/runtime/exclusive.go`: acquire/release lineage-aware leases on standalone execution when needed
- `lockkit/runtime/exclusive_test.go`: cover single-lock parent and child rejection, release cleanup, and manager startup validation
- `lockkit/runtime/manager.go`: fail fast when lineage-aware definitions are registered with a backend that lacks `LineageDriver`
- `lockkit/runtime/presence_test.go`: assert presence queries remain exact-key only after lineage support lands
- `lockkit/testkit/memory_driver.go`: implement deterministic lineage-aware in-memory semantics for unit tests
- `lockkit/testkit/memory_driver_test.go`: cover lineage acquire, renew, release, expiry cleanup, and multiplicity safety
- `lockkit/workers/execute.go`: apply lineage-aware single-lock acquire/release to worker claims
- `lockkit/workers/execute_composite.go`: route lineage-aware composite members through the lineage path
- `lockkit/workers/execute_composite_test.go`: cover composite worker routing and retry semantics on overlap
- `lockkit/workers/execute_test.go`: cover worker overlap retry behavior, lineage cleanup, and renewal semantics
- `lockkit/workers/manager.go`: fail fast on missing lineage backend capability
- `lockkit/workers/manager_test.go`: verify manager construction gating for lineage-aware registries
- `lockkit/workers/renewal.go`: renew lineage-aware leases and descendant markers atomically when supported

### New files to create

- `lockkit/internal/lineage/resolve.go`: resolve concrete lineage chains from validated definitions and request input
- `lockkit/internal/lineage/resolve_test.go`: unit tests for ancestor resolution, recursion depth, and invalid chain handling

## Phase Scope

This plan delivers only what the Phase 2a design requires:

- recursive registry validation for `ParentRef` chains
- mandatory template-backed hierarchical builders for lineage-aware definitions
- optional `LineageDriver` capability with atomic acquire, renew, and release semantics
- in-memory and Redis lineage-aware backend implementations
- runtime and worker enforcement for single-lock parent-child overlap
- composite member routing through lineage-aware backend operations
- migration-facing docs and examples for the new enforcement rules

It does **not** implement:

- strict-mode lineage enforcement
- escalation from child to parent
- generic ancestry inference for arbitrary custom key builders
- new public presence APIs for descendant state
- queue-product-specific middleware
- broader lock taxonomy changes beyond what Phase 2a already specified

## Implementation Notes

- Use @superpowers:test-driven-development for each task. Each step below assumes the test lands first, fails for the expected reason, then the smallest implementation change makes it pass.
- Use @superpowers:verification-before-completion before claiming Phase 2a is done.
- Use @superpowers:requesting-code-review after implementation tasks are complete and before merge.
- Keep `drivers.Driver` exact-key only. All lineage behavior must hang off the optional `drivers.LineageDriver`.
- Keep `CheckPresence` behavior exact-key only. Do not add public descendant inspection APIs in this phase.
- `AncestorKeys` must stay in stable root-first order everywhere (`ResolveAcquirePlan`, Redis `KEYS/ARGV`, renew, release, and tests).
- `LineageAcquireRequest.LeaseID` is required for every lineage-aware acquire, including parent acquires, so cleanup and multiplicity-safe membership attribution remain uniform.
- Test helpers introduced by this plan should live close to the packages they serve, preferably in `*_test.go` or `test_helpers_test.go` files inside `lockkit/runtime`, `lockkit/workers`, `lockkit/drivers/redis`, and `lockkit/testkit`.

### Task 1: Expose Template Lineage Metadata And Recursive Registry Validation

**Files:**
- Modify: `lockkit/definitions/key_builder.go`
- Modify: `lockkit/definitions/key_builder_test.go`
- Modify: `lockkit/registry/validation.go`
- Modify: `lockkit/registry/registry_test.go`

- [ ] **Step 1: Write the failing definition and registry tests**

```go
func TestTemplateKeyBuilderExposesTemplateMetadata(t *testing.T) {
	builder := definitions.MustTemplateKeyBuilder(
		"order:{order_id}:item:{item_id}",
		[]string{"order_id", "item_id"},
	)

	meta, ok := definitions.TemplateMetadata(builder)
	if !ok {
		t.Fatal("expected template metadata to be available")
	}
	if meta.Template != "order:{order_id}:item:{item_id}" {
		t.Fatalf("unexpected template: %q", meta.Template)
	}
	if !reflect.DeepEqual(meta.Fields, []string{"order_id", "item_id"}) {
		t.Fatalf("unexpected field order: %#v", meta.Fields)
	}
}

func TestRegistryValidateRejectsBrokenLineageChain(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:         "order",
		Kind:       definitions.KindParent,
		Resource:   "order",
		Mode:       definitions.ModeStandard,
		LeaseTTL:   30 * time.Second,
		KeyBuilder: definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          definitions.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "order",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("item:{item_id}", []string{"item_id"}),
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "preserve parent template prefix") {
		t.Fatalf("expected lineage prefix validation error, got %v", err)
	}
}

func TestRegistryValidateRejectsNonRejectOverlapForChildLineage(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:         "order",
		Kind:       definitions.KindParent,
		Resource:   "order",
		Mode:       definitions.ModeStandard,
		LeaseTTL:   30 * time.Second,
		KeyBuilder: definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          definitions.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "order",
		OverlapPolicy: definitions.OverlapEscalate,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "only support overlap policy reject") {
		t.Fatalf("expected reject-only overlap validation error, got %v", err)
	}
}

func TestRegistryValidateRejectsUnknownParentRef(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          definitions.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "missing-parent",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "unknown parent") {
		t.Fatalf("expected unknown parent validation error, got %v", err)
	}
}

func TestRegistryValidateRejectsParentRefCycle(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "order",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "item",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          definitions.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "order",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle validation error, got %v", err)
	}
}
```

- [ ] **Step 2: Run the targeted tests to verify they fail**

Run: `go test ./lockkit/definitions ./lockkit/registry -run 'TemplateKeyBuilderExposesTemplateMetadata|BrokenLineageChain|Cycle|UnknownParent' -v`
Expected: FAIL with missing `TemplateMetadata`, missing recursive validation, or no cycle/prefix enforcement

- [ ] **Step 3: Add exported template metadata access and recursive validation**

```go
type TemplateBuilderMetadata struct {
	Template string
	Fields   []string
}

func TemplateMetadata(builder KeyBuilder) (TemplateBuilderMetadata, bool) {
	view, ok := builder.(interface {
		TemplateMetadata() TemplateBuilderMetadata
	})
	if !ok {
		return TemplateBuilderMetadata{}, false
	}
	return view.TemplateMetadata(), true
}

func validateLineageChain(
	def definitions.LockDefinition,
	definitionsByID map[string]definitions.LockDefinition,
	visited map[string]struct{},
) error {
	// Callers must pass a fresh visited map per top-level validation walk.
	if strings.TrimSpace(def.ParentRef) == "" {
		return nil
	}
	if _, seen := visited[def.ID]; seen {
		return errLineageCycleDetected
	}
	visited[def.ID] = struct{}{}

	parent, exists := definitionsByID[def.ParentRef]
	if !exists {
		return fmt.Errorf("child definition references unknown parent %q", def.ParentRef)
	}
	if def.Mode != definitions.ModeStandard || parent.Mode != definitions.ModeStandard {
		return errLineageModeUnsupported
	}
	if def.OverlapPolicy != definitions.OverlapReject {
		return errChildOverlapPolicyUnsupported
	}

	childMeta, ok := definitions.TemplateMetadata(def.KeyBuilder)
	if !ok {
		return errLineageTemplateRequired
	}
	parentMeta, ok := definitions.TemplateMetadata(parent.KeyBuilder)
	if !ok {
		return errLineageTemplateRequired
	}
	if !strings.HasPrefix(childMeta.Template, parentMeta.Template) {
		return errLineageTemplatePrefixInvalid
	}
	if !fieldsContain(parentMeta.Fields, childMeta.Fields) {
		return errLineageFieldsIncomplete
	}

	return validateLineageChain(parent, definitionsByID, visited)
}
```

- [ ] **Step 4: Run the package tests to verify lineage validation passes**

Run: `go test ./lockkit/definitions ./lockkit/registry -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/definitions/key_builder.go lockkit/definitions/key_builder_test.go lockkit/registry/validation.go lockkit/registry/registry_test.go
git commit -m "feat(registry): validate recursive lineage chains"
```

### Task 2: Add Shared Lineage Resolution And Public Error/Outcome Contracts

**Files:**
- Create: `lockkit/internal/lineage/resolve.go`
- Create: `lockkit/internal/lineage/resolve_test.go`
- Modify: `lockkit/errors/errors.go`
- Modify: `lockkit/internal/policy/outcome.go`
- Modify: `lockkit/drivers/contracts.go`
- Modify: `lockkit/drivers/contracts_phase2_test.go`

- [ ] **Step 1: Write the failing lineage resolver and contract tests**

```go
func TestResolveAcquirePlanReturnsAncestorKeys(t *testing.T) {
	defs := map[string]definitions.LockDefinition{
		"order": {
			ID:         "order",
			Kind:       definitions.KindParent,
			Resource:   "order",
			Mode:       definitions.ModeStandard,
			LeaseTTL:   30 * time.Second,
			KeyBuilder: definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		},
		"item": {
			ID:            "item",
			Kind:          definitions.KindChild,
			Resource:      "item",
			Mode:          definitions.ModeStandard,
			LeaseTTL:      30 * time.Second,
			ParentRef:     "order",
			OverlapPolicy: definitions.OverlapReject,
			KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
		},
	}

	plan, err := lineage.ResolveAcquirePlan(defs["item"], defs, map[string]string{
		"order_id": "123",
		"item_id":  "line-1",
	})
	if err != nil {
		t.Fatalf("ResolveAcquirePlan returned error: %v", err)
	}
	if plan.ResourceKey != "order:123:item:line-1" {
		t.Fatalf("unexpected resource key: %q", plan.ResourceKey)
	}
	if len(plan.AncestorKeys) != 1 || plan.AncestorKeys[0].ResourceKey != "order:123" {
		t.Fatalf("unexpected ancestors: %#v", plan.AncestorKeys)
	}
}

func TestOutcomeFromErrorTreatsOverlapAsRetry(t *testing.T) {
	if got := policy.OutcomeFromError(lockerrors.ErrOverlapRejected); got != policy.OutcomeRetry {
		t.Fatalf("expected retry, got %q", got)
	}
}
```

- [ ] **Step 2: Run the targeted tests to verify they fail**

Run: `go test ./lockkit/internal/lineage ./lockkit/errors ./lockkit/internal/policy ./lockkit/drivers -run 'ResolveAcquirePlan|OverlapAsRetry|LineageDriver' -v`
Expected: FAIL with missing package, missing `ErrOverlapRejected`, or missing lineage driver contract

- [ ] **Step 3: Add the shared lineage resolver, error, and optional driver capability**

```go
type AcquirePlan struct {
	DefinitionID string
	ResourceKey  string
	Kind         definitions.LockKind
	AncestorKeys []drivers.AncestorKey
	LeaseID      string
}

func (p AcquirePlan) LeaseMeta() drivers.LineageLeaseMeta {
	return drivers.LineageLeaseMeta{
		LeaseID:      p.LeaseID,
		Kind:         p.Kind,
		AncestorKeys: cloneAncestorKeys(p.AncestorKeys),
	}
}

func ResolveAcquirePlan(
	def definitions.LockDefinition,
	definitionsByID map[string]definitions.LockDefinition,
	input map[string]string,
) (AcquirePlan, error) {
	resourceKey, err := def.KeyBuilder.Build(input)
	if err != nil {
		return AcquirePlan{}, err
	}

	ancestors, err := resolveAncestors(def, definitionsByID, input)
	if err != nil {
		return AcquirePlan{}, err
	}

	return AcquirePlan{
		DefinitionID: def.ID,
		ResourceKey:  resourceKey,
		Kind:         def.Kind,
		// Ancestors are ordered root-first so script key assembly, renew, and release stay stable.
		AncestorKeys: ancestors,
		LeaseID:      newLeaseID(),
	}, nil
}

var ErrOverlapRejected = stdErrors.New("overlap rejected")

type LineageDriver interface {
	AcquireWithLineage(ctx context.Context, req LineageAcquireRequest) (LeaseRecord, error)
	RenewWithLineage(ctx context.Context, lease LeaseRecord, lineage LineageLeaseMeta) (LeaseRecord, LineageLeaseMeta, error)
	ReleaseWithLineage(ctx context.Context, lease LeaseRecord, lineage LineageLeaseMeta) error
}
```

- [ ] **Step 4: Run the targeted package tests to verify the new shared contract works**

Run: `go test ./lockkit/internal/lineage ./lockkit/errors ./lockkit/internal/policy ./lockkit/drivers -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/internal/lineage/resolve.go lockkit/internal/lineage/resolve_test.go lockkit/errors/errors.go lockkit/internal/policy/outcome.go lockkit/drivers/contracts.go lockkit/drivers/contracts_phase2_test.go
git commit -m "feat(drivers): add lineage execution contract"
```

### Task 3: Implement Lineage-Aware In-Memory Driver And Manager Capability Gating

**Files:**
- Modify: `lockkit/testkit/memory_driver.go`
- Modify: `lockkit/testkit/memory_driver_test.go`
- Modify: `lockkit/runtime/manager.go`
- Modify: `lockkit/runtime/presence_test.go`
- Modify: `lockkit/runtime/exclusive_test.go`
- Modify: `lockkit/registry/registry.go`
- Modify: `lockkit/workers/manager.go`
- Modify: `lockkit/workers/manager_test.go`

- [ ] **Step 1: Write the failing in-memory and manager construction tests**

```go
func TestMemoryDriverAcquireWithLineageRejectsParentWhileChildHeld(t *testing.T) {
	driver := testkit.NewMemoryDriver()

	childReq := drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: "item",
			ResourceKeys: []string{"order:123:item:line-1"},
			OwnerID:      "worker-a",
			LeaseTTL:     30 * time.Second,
		},
		Kind:    definitions.KindChild,
		LeaseID: "lease-child",
		AncestorKeys: []drivers.AncestorKey{
			{DefinitionID: "order", ResourceKey: "order:123"},
		},
	}
	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	defer func() { _ = driver.ReleaseWithLineage(context.Background(), childLease, drivers.LineageLeaseMeta{
		LeaseID:      childReq.LeaseID,
		Kind:         childReq.Kind,
		AncestorKeys: childReq.AncestorKeys,
	}) }()

	_, err = driver.AcquireWithLineage(context.Background(), drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: "order",
			ResourceKeys: []string{"order:123"},
			OwnerID:      "worker-b",
			LeaseTTL:     30 * time.Second,
		},
		Kind: definitions.KindParent,
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
}

func TestRuntimeManagerRejectsLineageRegistryWithoutLineageDriver(t *testing.T) {
	reg := registryWithLineageChain(t)
	_, err := runtime.NewManager(reg, exactOnlyDriverStub{}, observe.NewNoopRecorder())
	if err == nil || !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected manager capability rejection, got %v", err)
	}
}

func TestCheckPresenceRemainsExactKeyOnlyWithActiveChild(t *testing.T) {
	driver := testkit.NewMemoryDriver()
	childReq := drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: "item",
			ResourceKeys: []string{"order:123:item:line-1"},
			OwnerID:      "worker-a",
			LeaseTTL:     30 * time.Second,
		},
		Kind:    definitions.KindChild,
		LeaseID: "lease-child",
		AncestorKeys: []drivers.AncestorKey{
			{DefinitionID: "order", ResourceKey: "order:123"},
		},
	}
	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	defer func() { _ = driver.ReleaseWithLineage(context.Background(), childLease, drivers.LineageLeaseMeta{
		LeaseID:      childReq.LeaseID,
		Kind:         childReq.Kind,
		AncestorKeys: childReq.AncestorKeys,
	}) }()

	record, err := driver.CheckPresence(context.Background(), drivers.PresenceRequest{
		DefinitionID: "order",
		ResourceKeys: []string{"order:123"},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}
	if record.Present {
		t.Fatalf("expected exact-key presence only, got %#v", record)
	}
}
```

- [ ] **Step 2: Run the targeted tests to verify they fail**

Run: `go test ./lockkit/testkit ./lockkit/runtime ./lockkit/workers -run 'Lineage|RejectsLineageRegistryWithoutLineageDriver|ExactKeyOnly' -v`
Expected: FAIL with missing `AcquireWithLineage`, missing descendant tracking, or missing manager startup gate

- [ ] **Step 3: Implement lineage tracking in memory and manager capability checks**

```go
type lineageLeaseState struct {
	lease    drivers.LeaseRecord
	lineage  drivers.LineageLeaseMeta
	expireAt time.Time
}

type descendantMembership struct {
	ancestorKey string
	leaseID     string
	expireAt    time.Time
}

func (m *MemoryDriver) AcquireWithLineage(
	ctx context.Context,
	req drivers.LineageAcquireRequest,
) (drivers.LeaseRecord, error) {
	m.pruneExpired(time.Now())
	if err := m.rejectLineageConflict(req); err != nil {
		return drivers.LeaseRecord{}, err
	}
	lease := m.createLease(req.AcquireRequest)
	meta := drivers.LineageLeaseMeta{LeaseID: req.LeaseID, Kind: req.Kind, AncestorKeys: cloneAncestorKeys(req.AncestorKeys)}
	m.storeLeaseAndMembership(lease, meta)
	return lease, nil
}

func registryRequiresLineageDriver(reg registry.Reader) bool {
	snapshot, ok := reg.(definitionSnapshotReader)
	if !ok {
		return false
	}
	childrenByParent := indexChildrenByParent(snapshot.Definitions())
	for _, def := range snapshot.Definitions() {
		if definitionUsesLineage(def, childrenByParent) {
			return true
		}
	}
	return false
}

func definitionUsesLineage(def definitions.LockDefinition, childrenByParent map[string][]string) bool {
	return strings.TrimSpace(def.ParentRef) != "" || len(childrenByParent[def.ID]) > 0
}
```

- [ ] **Step 4: Run the package tests to verify deterministic lineage behavior**

Run: `go test ./lockkit/testkit ./lockkit/runtime ./lockkit/workers -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/testkit/memory_driver.go lockkit/testkit/memory_driver_test.go lockkit/registry/registry.go lockkit/runtime/manager.go lockkit/runtime/presence_test.go lockkit/runtime/exclusive_test.go lockkit/workers/manager.go lockkit/workers/manager_test.go
git commit -m "feat(testkit): add lineage-aware in-memory driver"
```

### Task 4: Implement Redis Atomic Lineage Acquire, Renew, And Release

**Files:**
- Modify: `lockkit/drivers/redis/driver.go`
- Modify: `lockkit/drivers/redis/scripts.go`
- Modify: `lockkit/drivers/redis/driver_integration_test.go`

- [ ] **Step 1: Write the failing Redis integration tests**

```go
func TestDriverAcquireWithLineageRejectsChildWhileParentHeldAcrossClients(t *testing.T) {
	clientA := newRedisClientForTest(t)
	clientB := newRedisClientForTest(t)
	driverA := redis.NewDriver(clientA, "lockman:test")
	driverB := redis.NewDriver(clientB, "lockman:test")

	parentReq := drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: "order",
			ResourceKeys: []string{"order:123"},
			OwnerID:      "runtime-a",
			LeaseTTL:     2 * time.Second,
		},
		Kind:    definitions.KindParent,
		LeaseID: "parent-lease",
	}
	parentLease, err := driverA.AcquireWithLineage(context.Background(), parentReq)
	if err != nil {
		t.Fatalf("parent acquire failed: %v", err)
	}
	defer func() { _ = driverA.ReleaseWithLineage(context.Background(), parentLease, drivers.LineageLeaseMeta{
		LeaseID: parentReq.LeaseID,
		Kind:    parentReq.Kind,
	}) }()

	_, err = driverB.AcquireWithLineage(context.Background(), drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: "item",
			ResourceKeys: []string{"order:123:item:line-1"},
			OwnerID:      "runtime-b",
			LeaseTTL:     2 * time.Second,
		},
		Kind:    definitions.KindChild,
		LeaseID: "child-lease",
		AncestorKeys: []drivers.AncestorKey{
			{DefinitionID: "order", ResourceKey: "order:123"},
		},
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
}

func TestDriverRenewWithLineageExtendsDescendantMembershipTTL(t *testing.T) {
	clientA := newRedisClientForTest(t)
	clientB := newRedisClientForTest(t)
	driverA := redis.NewDriver(clientA, "lockman:test")
	driverB := redis.NewDriver(clientB, "lockman:test")

	childReq := drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: "item",
			ResourceKeys: []string{"order:123:item:line-1"},
			OwnerID:      "runtime-a",
			LeaseTTL:     200 * time.Millisecond,
		},
		Kind:    definitions.KindChild,
		LeaseID: "child-lease",
		AncestorKeys: []drivers.AncestorKey{
			{DefinitionID: "order", ResourceKey: "order:123"},
		},
	}
	childLease, err := driverA.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	childMeta := drivers.LineageLeaseMeta{
		LeaseID:      childReq.LeaseID,
		Kind:         childReq.Kind,
		AncestorKeys: childReq.AncestorKeys,
	}
	defer func() { _ = driverA.ReleaseWithLineage(context.Background(), childLease, childMeta) }()

	time.Sleep(100 * time.Millisecond)
	childLease, childMeta, err = driverA.RenewWithLineage(context.Background(), childLease, childMeta)
	if err != nil {
		t.Fatalf("child renew failed: %v", err)
	}

	time.Sleep(130 * time.Millisecond)
	_, err = driverB.AcquireWithLineage(context.Background(), drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: "order",
			ResourceKeys: []string{"order:123"},
			OwnerID:      "runtime-b",
			LeaseTTL:     200 * time.Millisecond,
		},
		Kind:    definitions.KindParent,
		LeaseID: "parent-lease",
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection after renew, got %v", err)
	}
}

func TestDriverReleaseWithLineageRemovesOnlyReleasedMembership(t *testing.T) {
	clientA := newRedisClientForTest(t)
	clientB := newRedisClientForTest(t)
	driverA := redis.NewDriver(clientA, "lockman:test")
	driverB := redis.NewDriver(clientB, "lockman:test")

	childOneReq := newChildAcquireRequest("child-one", "line-1")
	childOne, err := driverA.AcquireWithLineage(context.Background(), childOneReq)
	if err != nil {
		t.Fatalf("child one acquire failed: %v", err)
	}
	childTwoReq := newChildAcquireRequest("child-two", "line-2")
	childTwo, err := driverA.AcquireWithLineage(context.Background(), childTwoReq)
	if err != nil {
		t.Fatalf("child two acquire failed: %v", err)
	}

	if err := driverA.ReleaseWithLineage(context.Background(), childOne, drivers.LineageLeaseMeta{
		LeaseID:      childOneReq.LeaseID,
		Kind:         childOneReq.Kind,
		AncestorKeys: childOneReq.AncestorKeys,
	}); err != nil {
		t.Fatalf("release child one failed: %v", err)
	}

	_, err = driverB.AcquireWithLineage(context.Background(), newParentAcquireRequest("parent-after-first-release"))
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected parent to stay blocked by child two, got %v", err)
	}

	if err := driverA.ReleaseWithLineage(context.Background(), childTwo, drivers.LineageLeaseMeta{
		LeaseID:      childTwoReq.LeaseID,
		Kind:         childTwoReq.Kind,
		AncestorKeys: childTwoReq.AncestorKeys,
	}); err != nil {
		t.Fatalf("release child two failed: %v", err)
	}

	parentReq := newParentAcquireRequest("parent-after-second-release")
	parentLease, err := driverB.AcquireWithLineage(context.Background(), parentReq)
	if err != nil {
		t.Fatalf("expected parent acquire to succeed after both children release, got %v", err)
	}
	_ = driverB.ReleaseWithLineage(context.Background(), parentLease, drivers.LineageLeaseMeta{
		LeaseID: parentReq.LeaseID,
		Kind:    parentReq.Kind,
	})
}

func TestDriverExpiredChildNoLongerBlocksParentAcquire(t *testing.T) {
	client := newRedisClientForTest(t)
	driver := redis.NewDriver(client, "lockman:test")

	childReq := drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: "item",
			ResourceKeys: []string{"order:123:item:line-1"},
			OwnerID:      "runtime-a",
			LeaseTTL:     120 * time.Millisecond,
		},
		Kind:    definitions.KindChild,
		LeaseID: "expiring-child",
		AncestorKeys: []drivers.AncestorKey{
			{DefinitionID: "order", ResourceKey: "order:123"},
		},
	}
	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}

	time.Sleep(180 * time.Millisecond)

	parentReq := newParentAcquireRequest("parent-after-expiry")
	parentLease, err := driver.AcquireWithLineage(context.Background(), parentReq)
	if err != nil {
		t.Fatalf("expected parent acquire after expiry, got %v", err)
	}
	_ = driver.ReleaseWithLineage(context.Background(), parentLease, drivers.LineageLeaseMeta{
		LeaseID: parentReq.LeaseID,
		Kind:    parentReq.Kind,
	})
	_ = driver.ReleaseWithLineage(context.Background(), childLease, drivers.LineageLeaseMeta{
		LeaseID:      childReq.LeaseID,
		Kind:         childReq.Kind,
		AncestorKeys: childReq.AncestorKeys,
	})
}
```

- [ ] **Step 2: Run the Redis integration tests to verify they fail**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./lockkit/drivers/redis -run 'Lineage|DescendantMembershipTTL' -v`
Expected: FAIL with missing lineage driver methods or missing marker renewal/cleanup

- [ ] **Step 3: Implement Redis lineage scripts and wire them through the driver**

```go
func (d *Driver) AcquireWithLineage(
	ctx context.Context,
	req drivers.LineageAcquireRequest,
) (drivers.LeaseRecord, error) {
	keys := lineageAcquireKeys(d.keyPrefix, req)
	args := lineageAcquireArgs(req)

	result, err := lineageAcquireScript.Run(ctx, d.client, keys, args...).Result()
	if err != nil {
		return drivers.LeaseRecord{}, err
	}

	status, ttlMillis, err := parseLineageAcquireResult(result)
	if err != nil {
		return drivers.LeaseRecord{}, err
	}
	switch status {
	case lineageAcquireOverlap:
		return drivers.LeaseRecord{}, lockerrors.ErrOverlapRejected
	case lineageAcquireBusy:
		return drivers.LeaseRecord{}, drivers.ErrLeaseAlreadyHeld
	}

	lease := buildLeaseRecord(req.AcquireRequest, time.Duration(ttlMillis)*time.Millisecond, d.now())
	return lease, nil
}

// Redis lineage storage model:
// - Main lease key:
//   lockman:lease:<definition_id_b64>:<resource_key_b64>
//   value = owner ID
// - Descendant membership sorted set per concrete resource instance:
//   lockman:lineage:<definition_id_b64>:<resource_key_b64>
//   member = <lease_id>|<definition_id_b64>|<resource_key_b64>
//   score = expiry unix millis for that descendant lease
//   This single key scheme is used both for:
//   1. checking whether the currently requested resource instance has any active descendants
//   2. publishing descendant membership into each ancestor instance
//
// Script responsibilities:
// - Acquire(parent):
//   1. `ZREMRANGEBYSCORE current_lineage_zset -inf now`
//   2. reject with overlap if `ZCARD current_lineage_zset > 0`
//   3. reject with busy if main lease key already exists
//   4. `SET key owner PX ttl NX`
// - Acquire(child/grandchild):
//   1. `ZREMRANGEBYSCORE current_lineage_zset -inf now`
//   2. reject with overlap if `ZCARD current_lineage_zset > 0`
//   3. reject with busy if main lease key already exists
//   4. for each ancestor exact lease key, reject with overlap if it exists
//   5. `SET key owner PX ttl NX`
//   6. `ZADD` the descendant member into every ancestor zset with score = now+ttl
//   7. extend each ancestor zset TTL to `max(existing_pttl, latest_member_expiry-now)` so the key is never shortened while other members remain live
// - Renew:
//   1. verify main lease key exists and owner matches
//   2. `PEXPIRE` main lease key to the renewed ttl
//   3. `ZADD XX` each descendant member with new score = now+ttl
//   4. extend each ancestor zset TTL to `max(existing_pttl, latest_member_expiry-now)` instead of blindly overwriting it
// - Release:
//   1. verify main lease key exists and owner matches
//   2. `DEL` main lease key
//   3. `ZREM` only the exact member derived from `LeaseID`
//   4. `ZREMRANGEBYSCORE ... -inf now`
//   5. `DEL` ancestor zset when `ZCARD == 0`
//
// KEYS layout:
// - KEYS[1] = main lease key
// - KEYS[2] = current resource lineage zset key (`lockman:lineage:<definition_id_b64>:<resource_key_b64>`)
// - KEYS[3..2+ancestor_count] = ancestor exact lease keys
// - KEYS[3+ancestor_count..] = ancestor lineage zset keys using the same `lockman:lineage:<definition_id_b64>:<resource_key_b64>` format
//
// ARGV layout:
// - ARGV[1] = owner ID
// - ARGV[2] = ttl millis
// - ARGV[3] = current unix millis
// - ARGV[4] = lock kind
// - ARGV[5] = lease ID
// - ARGV[6] = definition ID (base64 encoded)
// - ARGV[7] = resource key (base64 encoded)
// - ARGV[8] = ancestor count
// - ARGV[9..] = descendant member strings in ancestor order
//
var lineageAcquireScript = goredis.NewScript(`
-- KEYS[1] = main lease key
-- KEYS[2] = current resource lineage zset key
-- KEYS[3..2+ancestor_count] = ancestor exact lease keys
-- KEYS[3+ancestor_count..] = ancestor lineage zset keys
-- ARGV = owner, ttl_ms, now_ms, kind, lease_id, definition_id_b64, resource_key_b64, ancestor_count, membership members...
-- Return:
--   {1, ttl_ms} on success
--   {-1, 0} on exact-key busy
--   {-2, 0} on parent-child overlap
`)
```

- [ ] **Step 4: Run Redis package tests to verify atomic lineage behavior**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./lockkit/drivers/redis -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/drivers/redis/driver.go lockkit/drivers/redis/scripts.go lockkit/drivers/redis/driver_integration_test.go
git commit -m "feat(redis): add atomic lineage lease operations"
```

### Task 5: Route Runtime Single-Lock And Composite Execution Through Lineage

**Files:**
- Modify: `lockkit/runtime/exclusive.go`
- Modify: `lockkit/runtime/composite.go`
- Modify: `lockkit/runtime/exclusive_test.go`
- Modify: `lockkit/runtime/composite_test.go`
- Modify: `lockkit/observe/contracts.go`

- [ ] **Step 1: Write the failing runtime tests**

```go
func TestExecuteExclusiveRejectsParentWhenChildHeldByAnotherManager(t *testing.T) {
	reg := registryWithLineageChain(t)
	driver := testkit.NewMemoryDriver()

	childManager, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("child manager init failed: %v", err)
	}
	parentManager, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("parent manager init failed: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = childManager.ExecuteExclusive(context.Background(), childSyncRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err = parentManager.ExecuteExclusive(context.Background(), parentSyncRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
		t.Fatal("parent callback should not run")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
	close(release)
}

func TestExecuteExclusiveRejectsChildWhenParentHeldByAnotherManager(t *testing.T) {
	reg := registryWithLineageChain(t)
	driver := testkit.NewMemoryDriver()

	parentManager, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("parent manager init failed: %v", err)
	}
	childManager, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("child manager init failed: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = parentManager.ExecuteExclusive(context.Background(), parentSyncRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err = childManager.ExecuteExclusive(context.Background(), childSyncRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
		t.Fatal("child callback should not run")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
	close(release)
}

func TestExecuteCompositeExclusiveUsesLineageDriverForLineageMembers(t *testing.T) {
	reg := registryWithCompositeLineageMembers(t)
	driver := testkit.NewMemoryDriver()
	holder, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("holder init failed: %v", err)
	}
	compositeMgr, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("composite manager init failed: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = holder.ExecuteExclusive(context.Background(), childSyncRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err = compositeMgr.ExecuteCompositeExclusive(context.Background(), compositeParentMemberRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
		t.Fatal("composite callback should not run while child is held")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected composite overlap rejection, got %v", err)
	}

	close(release)

	entered = make(chan struct{})
	release = make(chan struct{})
	go func() {
		_ = holder.ExecuteExclusive(context.Background(), parentSyncRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err = compositeMgr.ExecuteCompositeExclusive(context.Background(), compositeChildMemberRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
		t.Fatal("composite callback should not run while parent is held")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected composite overlap rejection, got %v", err)
	}
	close(release)
}
```

- [ ] **Step 2: Run the targeted runtime tests to verify they fail**

Run: `go test ./lockkit/runtime -run 'RejectsParentWhenChildHeld|RejectsChildWhenParentHeld|UsesLineageDriverForLineageMembers|LineageRegistryWithoutLineageDriver' -v`
Expected: FAIL with plain `Acquire` path bypassing lineage enforcement

- [ ] **Step 3: Implement runtime lineage routing and release semantics**

```go
type heldLease struct {
	lease   drivers.LeaseRecord
	lineage *drivers.LineageLeaseMeta
}

func (m *Manager) acquireLease(
	ctx context.Context,
	def definitions.LockDefinition,
	resourceKey string,
	keyInput map[string]string,
	ownerID string,
) (heldLease, error) {
	childrenByParent := m.childrenByParent()
	if !definitionUsesLineage(def, childrenByParent) {
		lease, err := m.driver.Acquire(ctx, drivers.AcquireRequest{
			DefinitionID: def.ID,
			ResourceKeys: []string{resourceKey},
			OwnerID:      ownerID,
			LeaseTTL:     def.LeaseTTL,
		})
		return heldLease{lease: lease}, err
	}

	plan, err := lineage.ResolveAcquirePlan(def, m.definitionsByID(), keyInput)
	if err != nil {
		return heldLease{}, err
	}
	lineageDriver := m.lineageDriver()
	lease, err := lineageDriver.AcquireWithLineage(ctx, drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: def.ID,
			ResourceKeys: []string{plan.ResourceKey},
			OwnerID:      ownerID,
			LeaseTTL:     def.LeaseTTL,
		},
		Kind:         plan.Kind,
		LeaseID:      plan.LeaseID,
		AncestorKeys: plan.AncestorKeys,
	})
	if err != nil {
		if errors.Is(err, lockerrors.ErrOverlapRejected) {
			m.recorder.RecordOverlapRejected(ctx, def.ID)
		}
		return heldLease{}, mapAcquireError(err)
	}
	meta := plan.LeaseMeta()
	return heldLease{lease: lease, lineage: &meta}, nil
}

func mapAcquireError(err error) error {
	switch {
	case errors.Is(err, lockerrors.ErrOverlapRejected):
		return lockerrors.ErrOverlapRejected
	case errors.Is(err, drivers.ErrLeaseAlreadyHeld):
		return lockerrors.ErrLockBusy
	case errors.Is(err, context.DeadlineExceeded):
		return lockerrors.ErrLockAcquireTimeout
	default:
		return err
	}
}
```

- [ ] **Step 4: Run the runtime package tests to verify single-lock and composite enforcement**

Run: `go test ./lockkit/runtime -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/runtime/exclusive.go lockkit/runtime/composite.go lockkit/runtime/exclusive_test.go lockkit/runtime/composite_test.go lockkit/observe/contracts.go
git commit -m "feat(runtime): enforce lineage overlap in runtime paths"
```

### Task 6: Route Worker Single-Lock, Composite, And Renewal Paths Through Lineage

**Files:**
- Modify: `lockkit/workers/execute.go`
- Modify: `lockkit/workers/execute_composite.go`
- Modify: `lockkit/workers/renewal.go`
- Modify: `lockkit/workers/execute_test.go`
- Modify: `lockkit/workers/execute_composite_test.go`

- [ ] **Step 1: Write the failing worker tests**

```go
func TestExecuteClaimedReturnsRetryOutcomeForRuntimeOverlap(t *testing.T) {
	driver := testkit.NewMemoryDriver()
	reg := registryWithLineageChain(t)
	mgr := newWorkerManagerWithDriver(t, reg, driver)

	parentLease, parentMeta, err := driver.AcquireWithLineage(context.Background(), drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: "order",
			ResourceKeys: []string{"order:123"},
			OwnerID:      "external-parent",
			LeaseTTL:     30 * time.Second,
		},
		Kind:    definitions.KindParent,
		LeaseID: "parent-lease",
	})
	if err != nil {
		t.Fatalf("AcquireWithLineage failed: %v", err)
	}
	defer func() { _ = driver.ReleaseWithLineage(context.Background(), parentLease, parentMeta) }()

	err = mgr.ExecuteClaimed(context.Background(), childMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		t.Fatal("callback should not run")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap error, got %v", err)
	}
	if got := policy.OutcomeFromError(err); got != policy.OutcomeRetry {
		t.Fatalf("expected retry outcome, got %q", got)
	}
}

func TestExecuteClaimedRejectsParentWhenChildHeldByAnotherWorker(t *testing.T) {
	driver := testkit.NewMemoryDriver()
	reg := registryWithLineageChain(t)
	childMgr := newWorkerManagerWithDriver(t, reg, driver)
	parentMgr := newWorkerManagerWithDriver(t, reg, driver)

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- childMgr.ExecuteClaimed(context.Background(), childMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err := parentMgr.ExecuteClaimed(context.Background(), parentMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		t.Fatal("parent callback should not run")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
	if got := policy.OutcomeFromError(err); got != policy.OutcomeRetry {
		t.Fatalf("expected retry outcome, got %q", got)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("child ExecuteClaimed returned error: %v", err)
	}
}

func TestExecuteClaimedRenewsLineageMembershipUntilCallbackCompletes(t *testing.T) {
	driver := testkit.NewMemoryDriver()
	reg := registryWithShortTTLLineageChain(t, 150*time.Millisecond)
	childMgr := newWorkerManagerWithDriver(t, reg, driver)
	parentMgr := newWorkerManagerWithDriver(t, reg, driver)

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- childMgr.ExecuteClaimed(context.Background(), childMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	time.Sleep(220 * time.Millisecond)
	err := parentMgr.ExecuteClaimed(context.Background(), parentMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		t.Fatal("parent callback should not run while child renewals succeed")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection after renew window, got %v", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("child ExecuteClaimed returned error: %v", err)
	}
}

func TestExecuteCompositeClaimedUsesLineageDriverForLineageMembers(t *testing.T) {
	driver := testkit.NewMemoryDriver()
	reg := registryWithCompositeLineageMembers(t)
	holder := newWorkerManagerWithDriver(t, reg, driver)
	compositeMgr := newWorkerManagerWithDriver(t, reg, driver)

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- holder.ExecuteClaimed(context.Background(), parentMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err := compositeMgr.ExecuteCompositeClaimed(context.Background(), compositeChildMemberClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		t.Fatal("composite callback should not run while parent is held")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
	if got := policy.OutcomeFromError(err); got != policy.OutcomeRetry {
		t.Fatalf("expected retry outcome, got %q", got)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("holder ExecuteClaimed returned error: %v", err)
	}
}
```

- [ ] **Step 2: Run the targeted worker tests to verify they fail**

Run: `go test ./lockkit/workers -run 'RuntimeOverlap|RejectsParentWhenChildHeld|RenewsLineageMembership|ExecuteCompositeClaimedUsesLineageDriver' -v`
Expected: FAIL with plain `Acquire` and plain renewal paths allowing overlap or expiring markers

- [ ] **Step 3: Implement worker lineage acquire, renewal, release, and composite routing**

```go
type renewableLease struct {
	lease   drivers.LeaseRecord
	lineage *drivers.LineageLeaseMeta
}

func (m *Manager) acquireClaimLease(
	ctx context.Context,
	def definitions.LockDefinition,
	keyInput map[string]string,
	ownerID string,
) (renewableLease, error) {
	plan, err := lineage.ResolveAcquirePlan(def, m.definitionsByID(), keyInput)
	if err != nil {
		return renewableLease{}, err
	}
	if !definitionUsesLineage(def, m.childrenByParent()) {
		lease, err := m.driver.Acquire(ctx, drivers.AcquireRequest{
			DefinitionID: def.ID,
			ResourceKeys: []string{plan.ResourceKey},
			OwnerID:      ownerID,
			LeaseTTL:     def.LeaseTTL,
		})
		return renewableLease{lease: lease}, err
	}

	lineageDriver := m.lineageDriver()
	lease, err := lineageDriver.AcquireWithLineage(ctx, drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: def.ID,
			ResourceKeys: []string{plan.ResourceKey},
			OwnerID:      ownerID,
			LeaseTTL:     def.LeaseTTL,
		},
		Kind:         plan.Kind,
		LeaseID:      plan.LeaseID,
		AncestorKeys: plan.AncestorKeys,
	})
	if err != nil {
		return renewableLease{}, mapAcquireError(err)
	}
	meta := plan.LeaseMeta()
	return renewableLease{lease: lease, lineage: &meta}, nil
}

func (m *Manager) renewLease(
	ctx context.Context,
	current drivers.LeaseRecord,
	lineageMeta *drivers.LineageLeaseMeta,
) (drivers.LeaseRecord, *drivers.LineageLeaseMeta, error) {
	if lineageMeta == nil {
		updated, err := m.driver.Renew(ctx, current)
		return updated, nil, err
	}
	updated, nextMeta, err := m.lineageDriver().RenewWithLineage(ctx, current, *lineageMeta)
	return updated, &nextMeta, err
}

func mapAcquireError(err error) error {
	switch {
	case errors.Is(err, lockerrors.ErrOverlapRejected):
		return lockerrors.ErrOverlapRejected
	case errors.Is(err, drivers.ErrLeaseAlreadyHeld):
		return lockerrors.ErrLockBusy
	case errors.Is(err, context.DeadlineExceeded):
		return lockerrors.ErrLockAcquireTimeout
	default:
		return err
	}
}
```

- [ ] **Step 4: Run worker package tests to verify retry mapping and lineage renewal**

Run: `go test ./lockkit/workers -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/workers/execute.go lockkit/workers/execute_composite.go lockkit/workers/renewal.go lockkit/workers/execute_test.go lockkit/workers/execute_composite_test.go
git commit -m "feat(workers): enforce lineage overlap in worker paths"
```

### Task 7: Update Docs, Examples, And Full Verification Coverage

**Files:**
- Modify: `README.md`
- Modify: `examples/phase2-parent-child-runtime/main.go`
- Modify: `examples/phase2-parent-child-runtime/main_test.go`
- Modify: `lockkit/drivers/redis/driver_integration_test.go`
- Modify: `lockkit/runtime/composite_test.go`
- Modify: `lockkit/runtime/presence_test.go`
- Modify: `lockkit/workers/execute_test.go`
- Modify: `lockkit/workers/execute_composite_test.go`

- [ ] **Step 1: Write or update the failing docs/example assertions**

```go
func TestPhase2ParentChildRuntimeExampleReportsOverlapRejection(t *testing.T) {
	output := runExample(t, "./examples/phase2-parent-child-runtime")
	if !strings.Contains(output, "overlap rejected") {
		t.Fatalf("expected overlap rejection output, got %q", output)
	}
	if strings.Contains(output, "ParentRef is metadata only") {
		t.Fatalf("example still describes pre-phase-2a behavior: %q", output)
	}
}

func TestPresenceAPIStillIgnoresDescendantMarkers(t *testing.T) {
	reg := registryWithLineageChain(t)
	driver := testkit.NewMemoryDriver()
	mgr, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("manager init failed: %v", err)
	}

	childLease, childMeta, err := driver.AcquireWithLineage(context.Background(), drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: "item",
			ResourceKeys: []string{"order:123:item:line-1"},
			OwnerID:      "runtime-a",
			LeaseTTL:     30 * time.Second,
		},
		Kind:    definitions.KindChild,
		LeaseID: "presence-child",
		AncestorKeys: []drivers.AncestorKey{
			{DefinitionID: "order", ResourceKey: "order:123"},
		},
	})
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	defer func() { _ = driver.ReleaseWithLineage(context.Background(), childLease, childMeta) }()

	record, err := mgr.CheckPresence(context.Background(), definitions.PresenceRequest{
		DefinitionID: "order",
		KeyInput: map[string]string{
			"order_id": "123",
		},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}
	if record.Present {
		t.Fatalf("expected parent exact key to remain absent, got %#v", record)
	}
}

func TestRedisLineageMarkersDisappearAfterLeaseExpiry(t *testing.T) {
	client := newRedisClientForTest(t)
	driver := redis.NewDriver(client, "lockman:test")

	childReq := drivers.LineageAcquireRequest{
		AcquireRequest: drivers.AcquireRequest{
			DefinitionID: "item",
			ResourceKeys: []string{"order:123:item:line-1"},
			OwnerID:      "runtime-a",
			LeaseTTL:     120 * time.Millisecond,
		},
		Kind:    definitions.KindChild,
		LeaseID: "expiring-child",
		AncestorKeys: []drivers.AncestorKey{
			{DefinitionID: "order", ResourceKey: "order:123"},
		},
	}
	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}

	time.Sleep(180 * time.Millisecond)

	parentReq := newParentAcquireRequest("parent-after-expiry")
	parentLease, err := driver.AcquireWithLineage(context.Background(), parentReq)
	if err != nil {
		t.Fatalf("expected parent acquire after expiry, got %v", err)
	}
	_ = driver.ReleaseWithLineage(context.Background(), parentLease, drivers.LineageLeaseMeta{
		LeaseID: parentReq.LeaseID,
		Kind:    parentReq.Kind,
	})
	_ = driver.ReleaseWithLineage(context.Background(), childLease, drivers.LineageLeaseMeta{
		LeaseID:      childReq.LeaseID,
		Kind:         childReq.Kind,
		AncestorKeys: childReq.AncestorKeys,
	})
}

func TestWorkerRenewFailureDoesNotLeavePermanentDescendantBlockers(t *testing.T) {
	driver := newMemoryDriverWithForcedRenewFailure(t)
	reg := registryWithShortTTLLineageChain(t, 120*time.Millisecond)
	childMgr := newWorkerManagerWithDriver(t, reg, driver)
	parentMgr := newWorkerManagerWithDriver(t, reg, driver)

	err := childMgr.ExecuteClaimed(context.Background(), childMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, lockerrors.ErrLeaseLost) {
		t.Fatalf("expected lease lost after forced renewal failure, got %v", err)
	}

	time.Sleep(180 * time.Millisecond)

	err = parentMgr.ExecuteClaimed(context.Background(), parentMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected parent claim to succeed after failed child expiry, got %v", err)
	}
}
```

- [ ] **Step 2: Run the docs/example verification to confirm it fails first**

Run: `go test ./examples/phase2-parent-child-runtime -v`
Expected: FAIL until the example narrative and assertions are updated for Phase 2a

- [ ] **Step 3: Update docs, examples, and any missing verification scenarios**

```md
## Phase 2a Status

- Single-lock parent-child overlap is now enforced in `ExecuteExclusive` and `ExecuteClaimed`
- Composite lineage members route through the same backend lineage path, so composite execution does not bypass overlap rules
- `CheckPresence` remains exact-key only; descendant membership is internal coordination state

## Migration Note

Applications that nested parent and child acquires across goroutines, workers, or processes may now receive `ErrOverlapRejected`.
```

- [ ] **Step 4: Run the full verification matrix**

Run: `go test ./... -v`
Expected: PASS

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./lockkit/drivers/redis ./lockkit/runtime ./lockkit/workers -v`
Expected: PASS

Run: `go test ./examples/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add README.md examples/phase2-parent-child-runtime/main.go examples/phase2-parent-child-runtime/main_test.go lockkit/drivers/redis/driver_integration_test.go lockkit/runtime/composite_test.go lockkit/runtime/presence_test.go lockkit/workers/execute_test.go lockkit/workers/execute_composite_test.go
git commit -m "docs(readme): document phase 2a lineage enforcement"
```
