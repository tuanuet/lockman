# Definition + Variant Design

## Problem Statement

Currently, lock keys are constructed from `DefinitionID:resourceKey`, where `DefinitionID` is a hash derived from `useCaseName + kind`. This means two different usecases (e.g., `order.process` and `order.cancel`) cannot mutually exclude on the same resource (e.g., `order:123`) — they produce two different Redis keys.

**Requirements:** Allow multiple usecases to share a lock key so that:
1. Mutual exclusion on the same resource across usecases
2. Explicit, intuitive API — "base definition + variants" concept
3. Works with composite, lineage, and hold modes without restrictions
4. Minimal code changes — no SDK struct, request struct, or Lua script changes

## Solution: Definition + Variant

### Core Concept

A `Definition` is a shared lock identity. Multiple variants inherit the same `definitionID` → same lock key. Variants differ only in name (for logging/observe) and optional config overrides.

### API Design

#### 1. Define Base Definition

`DefineRun`, `DefineHold`, `DefineClaim` remain unchanged — they return `RunUseCase[T]`/`HoldUseCase[T]`/`ClaimUseCase[T]` as before.

For shared definitions, use `DefineDefinition`:

```go
contract := lockman.DefineDefinition("contract", lockman.Run,
    lockman.BindResourceID("order", func(o Order) string { return o.ID }),
    lockman.TTL(30*time.Second),
)
```

- `DefineDefinition` creates a `*Definition[T]` with a stable `definitionID` (FNV-64a hash of name + kind)
- Binding is specified at the base definition level — variants inherit it
- Variants cannot override the binding (they share the same lock resource)
- `DefineDefinition` does NOT create a usable usecase directly — variants do

#### 2. Create Variants

```go
importUC := contract.Variant("import")
deleteUC := contract.Variant("delete")
```

- Variants inherit `definitionID` and binding from the base definition
- Variant name = `baseName.variantName` (e.g., `contract.import`)
- Variants can add config overrides via `UseCaseOption` (e.g., `Strict()`, `TTL()`)
- Variants register as independent usecases in the registry (unique name)
- `Variant` is generic: `func (d *Definition[T]) Variant(name string, opts ...UseCaseOption) RunUseCase[T]`

#### Composite with Variants

Use the existing `Composite()` option — variants work naturally as composite members:

```go
// All composite members must share the same input type T
type SyncInput struct {
    OrderID   string
    SettingID string
}

syncUC := lockman.DefineRun("sync", syncBinding,
    lockman.Composite(
        lockman.DefineCompositeMember("setting", func(i SyncInput) map[string]string {
            return map[string]string{"setting_id": i.SettingID}
        }),
        lockman.DefineCompositeMember("import", func(i SyncInput) map[string]string {
            return map[string]string{"order_id": i.OrderID}
        }),
    ),
)
```

- Composite members use `DefineCompositeMember` with binding functions that extract the relevant key from the shared input type
- The `Definition` exposes its binding via `Binding() Binding[T]` method for use in non-composite scenarios
- Composite atomicity works naturally
- **Type constraint:** All composite members must share the same `T` as the parent usecase — enforced at compile time

#### 4. Lineage with Variants

Add a `LineageParent` function that accepts a usecase:

```go
validateUC := contract.Variant("validate")
importUC := contract.Variant("import", lockman.LineageParent(validateUC))
```

- `LineageParent` extracts the parent's name: `func LineageParent[T any](parent RunUseCase[T]) UseCaseOption`
- **Restriction:** `LineageParent` cannot reference a variant of the same definition. Both parent and child share the same `definitionID`, which would cause the child's `ParentRef` to equal its own `ID` — breaking lineage resolution.
- **Enforcement:** In `buildClientPlan`, detect same-definition lineage and reject: `"lockman: lineage parent cannot be a variant of the same definition"`
- If parent and child are from different definitions (different `definitionID`), lineage works normally

#### 5. Hold with Variants

```go
holdDef := lockman.DefineDefinition("contract", lockman.Hold,
    lockman.BindResourceID("order", func(o Order) string { return o.ID }),
)
holdUC := holdDef.HoldVariant("hold")
```

- Hold variants work the same as run variants — shared `definitionID`
- No data flow changes needed — hold uses `def.ID` which is already the shared ID

#### 6. Standalone Usecase (Non-Variant)

Existing `DefineRun`/`DefineHold`/`DefineClaim` remain unchanged:

```go
settingUC := lockman.DefineRun("setting", settingBinding, lockman.TTL(30*time.Second))
```

- No breaking changes — existing code works as-is
- `DefineRun` returns `RunUseCase[T]` with `.With()` method

#### 6. Force Release

```go
err := contract.ForceRelease(ctx, client, "order:123")
```

- Force release is a method on `Definition[T]`
- Uses the shared `definitionID` for key construction
- Idempotent: returns `nil` if lock does not exist

### Key Construction

#### Before (current)
```
lockman:lease:<definitionID>:<resourceKey>
```

#### After (with variants)
```
lockman:lease:<shared_definitionID>:<resourceKey>
```

- Variants share the same `definitionID` → same lock key
- Non-variant usecases remain unchanged — fully backward compatible
- Lease value remains plain ownerID string — no changes to Lua scripts
- No new keys, no metadata, no index — just shared definitionID

### Implementation Details

#### useCaseConfig Change

```go
// binding.go
type useCaseConfig struct {
    // ... existing fields ...
    definitionID string // shared definitionID for variants (empty = derive from name)
}
```

#### Definition Type

```go
// definition.go (root lockman package)
type Definition[T any] struct {
    name     string
    id       string // stable hash: FNV-64a(name + kind)
    kind     useCaseKind
    binding  Binding[T]
}

func DefineDefinition[T any](name string, kind useCaseKind, binding Binding[T], opts ...UseCaseOption) *Definition[T] {
    trimmed := strings.TrimSpace(name)
    if trimmed == "" {
        panic("lockman: definition name is required")
    }
    if binding.build == nil {
        panic("lockman: binding is required")
    }
    id := stableDefinitionID(trimmed, kind)
    return &Definition[T]{name: trimmed, id: id, kind: kind, binding: binding}
}

func stableDefinitionID(name string, kind useCaseKind) string {
    hash := fnv.New64a()
    _, _ = hash.Write([]byte{kindToByte(kind)})
    _, _ = hash.Write([]byte(name))
    return fmt.Sprintf("%016x", hash.Sum64())  // inline hex encoding, no dependency on internal/sdk
}

func kindToByte(kind useCaseKind) byte {
    switch kind {
    case useCaseKindRun: return 'r'
    case useCaseKindClaim: return 'c'
    case useCaseKindHold: return 'h'
    default: return '?'
    }
}
```

#### Variant Method

```go
func (d *Definition[T]) Variant(name string, opts ...UseCaseOption) RunUseCase[T] {
    if d.kind != useCaseKindRun {
        panic("lockman: Variant is only supported for Run definitions")
    }
    trimmed := strings.TrimSpace(name)
    if trimmed == "" {
        panic("lockman: variant name is required")
    }
    fullName := d.name + "." + trimmed
    cfg := useCaseConfig{definitionID: d.id}
    for _, opt := range opts {
        if opt != nil {
            opt(&cfg)
        }
    }
    return RunUseCase[T]{
        core:    newUseCaseCoreWithConfig(fullName, d.kind, cfg),
        binding: d.binding,
    }
}
```

#### Hold Variant Method

```go
func (d *Definition[T]) HoldVariant(name string, opts ...UseCaseOption) HoldUseCase[T] {
    if d.kind != useCaseKindHold {
        panic("lockman: HoldVariant is only supported for Hold definitions")
    }
    trimmed := strings.TrimSpace(name)
    if trimmed == "" {
        panic("lockman: variant name is required")
    }
    fullName := d.name + "." + trimmed
    cfg := useCaseConfig{definitionID: d.id}
    for _, opt := range opts {
        if opt != nil {
            opt(&cfg)
        }
    }
    return HoldUseCase[T]{
        core:    newUseCaseCoreWithConfig(fullName, d.kind, cfg),
        binding: d.binding,
    }
}
```

#### Claim Variant Method

```go
func (d *Definition[T]) ClaimVariant(name string, opts ...UseCaseOption) ClaimUseCase[T] {
    if d.kind != useCaseKindClaim {
        panic("lockman: ClaimVariant is only supported for Claim definitions")
    }
    trimmed := strings.TrimSpace(name)
    if trimmed == "" {
        panic("lockman: variant name is required")
    }
    fullName := d.name + "." + trimmed
    cfg := useCaseConfig{definitionID: d.id}
    for _, opt := range opts {
        if opt != nil {
            opt(&cfg)
        }
    }
    return ClaimUseCase[T]{
        core:    newUseCaseCoreWithConfig(fullName, d.kind, cfg),
        binding: d.binding,
    }
}
```

#### Composite Method

Composite uses the existing `Composite()` option. The `Definition` exposes its binding for use in composite member definitions:

```go
func (d *Definition[T]) Binding() Binding[T] {
    return d.binding
}
```

Example:
```go
syncUC := lockman.DefineRun("sync", syncBinding,
    lockman.Composite(
        lockman.DefineCompositeMember("setting", settingBinding),
        lockman.DefineCompositeMember("import", contract.Binding()),
    ),
)
```

#### LineageParent Function

```go
func LineageParent[T any](parent RunUseCase[T]) UseCaseOption {
    return func(cfg *useCaseConfig) {
        cfg.lineageParent = parent.core.name
    }
}
```

#### Force Release

```go
func (d *Definition[T]) ForceRelease(ctx context.Context, client *Client, resourceKey string) error {
    if client == nil {
        return fmt.Errorf("lockman: client is required")
    }
    return client.backend.ForceReleaseDefinition(ctx, d.id, resourceKey)
}
```

#### newUseCaseCoreWithConfig

```go
// registry.go
func newUseCaseCoreWithConfig(name string, kind useCaseKind, cfg useCaseConfig) *useCaseCore {
    return &useCaseCore{
        name:   strings.TrimSpace(name),
        kind:   kind,
        config: cfg,
    }
}
```

#### Same-Definition Lineage Enforcement

In `buildClientPlan`, after building `normalizedByName`, check for same-definition lineage:

```go
for _, useCase := range useCases {
    parentName := strings.TrimSpace(useCase.config.lineageParent)
    if parentName == "" {
        continue
    }
    parent, ok := cfg.registry.byName[parentName]
    if !ok {
        continue // will be caught by existing lineage parent validation
    }
    if useCase.config.definitionID != "" &&
        useCase.config.definitionID == parent.config.definitionID {
        return clientPlan{}, fmt.Errorf(
            "lockman: use case %q cannot use lineage parent %q — both share the same definition",
            useCase.name, parentName)
    }
}
```

This check runs after all usecases are registered, before engine registry construction.

In `client_validation.go`, when creating `sdk.UseCase`, pass the shared `definitionID`:

```go
func normalizeUseCase(useCase *useCaseCore, childCounts map[string]int, link sdk.RegistryLink) sdk.UseCase {
    return sdk.NewUseCaseWithID(
        useCase.name,
        useCase.config.definitionID,  // empty = derive from name (default)
        toSDKUseCaseKind(useCase.kind),
        sdk.CapabilityRequirements{
            RequiresIdempotency: useCase.kind == useCaseKindClaim && useCase.config.idempotent,
            RequiresStrict:      useCase.config.strict,
            RequiresLineage:     strings.TrimSpace(useCase.config.lineageParent) != "" || childCounts[useCase.name] > 0,
        },
        link,
    )
}
```

#### SDK Change

Add `NewUseCaseWithID` to `internal/sdk/usecase.go`:

```go
func NewUseCaseWithID(name string, definitionID string, kind UseCaseKind, reqs CapabilityRequirements, link RegistryLink) UseCase {
    id := definitionID
    if id == "" {
        id = stableUseCaseID(name, kind)  // default behavior
    }
    return useCase{
        id:           id,
        publicName:   name,
        kind:         kind,
        requirements: reqs,
        registryLink: link,
    }
}
```

#### Backend Change

Add `ForceReleaseDriver` as an **optional interface** (following the existing `StrictDriver`/`LineageDriver` pattern):

```go
// backend/contracts.go — new optional interface, NOT added to Driver
type ForceReleaseDriver interface {
    ForceReleaseDefinition(ctx context.Context, definitionID, resourceKey string) error
}
```

`Definition.ForceRelease` type-asserts:

```go
func (d *Definition[T]) ForceRelease(ctx context.Context, client *Client, resourceKey string) error {
    if client == nil {
        return fmt.Errorf("lockman: client is required")
    }
    fr, ok := client.backend.(ForceReleaseDriver)
    if !ok {
        return fmt.Errorf("lockman: backend does not support force release")
    }
    return fr.ForceReleaseDefinition(ctx, d.id, resourceKey)
}
```

Implementation in `backend/redis/driver.go`:

```go
func (d *Driver) ForceReleaseDefinition(ctx context.Context, definitionID, resourceKey string) error {
    keys := []string{
        d.buildLeaseKey(definitionID, resourceKey),
        d.buildStrictFenceCounterKey(definitionID, resourceKey),
        d.buildStrictTokenKey(definitionID, resourceKey),
        d.buildLineageKey(definitionID, resourceKey),
    }
    if err := d.client.Del(ctx, keys...).Err(); err != nil {
        return fmt.Errorf("lockman: force release definition: %w", err)
    }
    return nil
}
```

### File Changes (Expected)

| File | Change |
|------|--------|
| `definition.go` | New: Definition[T] type, DefineDefinition, Variant, HoldVariant, ClaimVariant, Binding, ForceRelease, stableDefinitionID, kindToByte |
| `binding.go` | Modify: Add definitionID to useCaseConfig; add LineageParent function |
| `registry.go` | New: newUseCaseCoreWithConfig function |
| `client_validation.go` | Modify: normalizeUseCase passes definitionID to SDK |
| `internal/sdk/usecase.go` | Modify: Add NewUseCaseWithID function |
| `backend/contracts.go` | New: ForceReleaseDriver optional interface |
| `backend/redis/driver.go` | Modify: Implement ForceReleaseDefinition (includes lineage key cleanup) |
| `definition_test.go` | New: unit tests for Definition + Variant API |
| `examples/core/definition-variant/main.go` | New: example usage |

### Testing Strategy

1. **Unit tests:**
   - DefineDefinition creates stable definitionID
   - Variant/HoldVariant/ClaimVariant inherit definitionID from base
   - Variant name = baseName.variantName
   - Variant binding type safety (compile-time)
   - LineageParent extracts parent name correctly
   - Same-definition lineage rejection in buildClientPlan
   - ForceRelease calls backend with correct definitionID
   - Binding type mismatch panics with clear message

2. **Integration tests:**
   - Two variants of same definition → mutual exclusion (one acquires, other fails)
   - Composite with variant binding → atomic acquire
   - Lineage with variant parent/child → overlap rejection works
   - Hold variant → acquire/release works with shared key
   - Force release behavior (cleans lease + auxiliary + lineage keys, idempotent)

3. **Backward compatibility tests:**
   - Non-variant usecases unchanged (derive definitionID from name)
   - Mixed environment (some variants, some not) coexist

### Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Variant binding type mismatch | Compile-time safety via generics; panic with clear message if misused |
| Composite with duplicate variant members | Composite validation already handles duplicate member IDs |
| Lineage with cross-definition parent/child | Lineage checks use definitionID — cross-definition works naturally |
| ForceReleaseDefinition interface change | New method on existing interface — all implementations must add it |
| Same-definition lineage rejection | Enforced in buildClientPlan; documented restriction |
| Variant only works for matching kind | Documented restriction; panic at define time if misused |
