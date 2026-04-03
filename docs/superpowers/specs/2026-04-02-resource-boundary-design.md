# Definition + Variant Design

## Problem Statement

Currently, lock keys are constructed from `DefinitionID:resourceKey`, where `DefinitionID` is a hash derived from `useCaseName + kind`. This means two different usecases (e.g., `order.process` and `order.cancel`) cannot mutually exclude on the same resource (e.g., `order:123`) — they produce two different Redis keys.

**Requirements:** Allow multiple usecases to share a lock key so that:
1. Mutual exclusion on the same resource across usecases
2. Explicit, intuitive API — "base definition + variants" concept
3. Works with composite, lineage, and hold modes without restrictions
4. No changes to SDK structs, request structs, backend driver, or Lua scripts

## Solution: Definition + Variant

### Core Concept

A `Definition` is a shared lock identity. Multiple variants inherit the same `definitionID` → same lock key. Variants differ only in name (for logging/observe) and optional config overrides.

### API Design

#### 1. Define Base Definition

```go
contract := lockman.Define("contract", lockman.Run,
    lockman.BindResourceID("order", func(o Order) string { return o.ID }),
    lockman.TTL(30*time.Second),
)
```

- `Define` creates a `*Definition` with a stable `definitionID` (FNV-64a hash of name + kind)
- Binding is specified at the base definition level — variants inherit it
- Variants cannot override the binding (they share the same lock resource)

#### 2. Create Variants

```go
importUC := contract.Variant("import")
deleteUC := contract.Variant("delete")
```

- Variants inherit `definitionID` and binding from the base definition
- Variant name = `baseName.variantName` (e.g., `contract.import`)
- Variants can add config overrides via `UseCaseOption` (e.g., `Strict()`, `TTL()`)
- Variants register as independent usecases in the registry (unique name)

#### 3. Composite with Variants

```go
settingUC := lockman.Define("setting", lockman.Run, settingBinding)
syncUC := contract.Composite("sync", settingUC, importUC)
```

- Composite members can be variants or standalone usecases
- Variant members use the shared `definitionID` for lock acquisition
- Composite atomicity works naturally — variant = just another member

#### 4. Lineage with Variants

```go
validateUC := contract.Variant("validate")
importUC := contract.Variant("import", lockman.LineageParent(validateUC))
```

- Parent and child variants share the same `definitionID`
- Lineage key resolution uses shared `definitionID` → consistent behavior
- No restriction needed — lineage + variant works naturally

#### 5. Hold with Variants

```go
holdDef := lockman.Define("contract", lockman.Hold,
    lockman.BindResourceID("order", func(o Order) string { return o.ID }),
)
holdUC := holdDef.Variant("hold")
```

- Hold variants work the same as run variants — shared `definitionID`
- No data flow changes needed — hold uses `def.ID` which is already the shared ID

#### 6. Force Release

```go
err := contract.ForceRelease(ctx, client, "order:123")
```

- Force release is a method on `Definition`
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
type useCaseConfig struct {
    // ... existing fields ...
    definitionID string // shared definitionID for variants (empty = derive from name)
}
```

#### Definition Type

```go
// definition.go (root lockman package)
type Definition struct {
    name    string
    id      string // stable hash: FNV-64a(name + kind)
    kind    useCaseKind
    binding interface{} // resource binding from Define()
}

func Define[T any](name string, kind useCaseKind, binding Binding[T], opts ...UseCaseOption) *Definition {
    trimmed := strings.TrimSpace(name)
    if trimmed == "" {
        panic("lockman: definition name is required")
    }
    if binding.build == nil {
        panic("lockman: binding is required")
    }
    id := stableDefinitionID(trimmed, kind)
    return &Definition{name: trimmed, id: id, kind: kind, binding: binding}
}

func stableDefinitionID(name string, kind useCaseKind) string {
    hash := fnv.New64a()
    _, _ = hash.Write([]byte{kindDelimiter(kind)})
    _, _ = hash.Write([]byte(name))
    return toHex(hash.Sum64())
}
```

#### Variant Method

```go
func (d *Definition) Variant(variantName string, opts ...UseCaseOption) UseCase {
    trimmed := strings.TrimSpace(variantName)
    if trimmed == "" {
        panic("lockman: variant name is required")
    }
    fullName := d.name + "." + trimmed
    cfg := applyUseCaseOptions(opts...)
    cfg.definitionID = d.id  // shared!
    return newUseCase(fullName, d.kind, cfg, d.binding)
}
```

#### Composite Method

```go
func (d *Definition) Composite[T any](compositeName string, members ...CompositeMember[T]) UseCase {
    fullName := d.name + "." + strings.TrimSpace(compositeName)
    cfg := useCaseConfig{
        definitionID: d.id,
        composite:    buildCompositeMembers(members),
    }
    return newUseCase(fullName, d.kind, cfg, nil) // composite has its own binding
}
```

#### Force Release

```go
func (d *Definition) ForceRelease(ctx context.Context, client *Client, resourceKey string) error {
    if client == nil {
        return fmt.Errorf("lockman: client is required")
    }
    return client.backend.ForceReleaseDefinition(ctx, d.id, resourceKey)
}
```

#### normalizeUseCase Change

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

Add `ForceReleaseDefinition` to `backend.Driver` interface:

```go
type Driver interface {
    // ... existing methods ...
    ForceReleaseDefinition(ctx context.Context, definitionID, resourceKey string) error
}
```

Implementation in `backend/redis/driver.go`:

```go
func (d *Driver) ForceReleaseDefinition(ctx context.Context, definitionID, resourceKey string) error {
    keys := []string{
        d.buildLeaseKey(definitionID, resourceKey),
        d.buildStrictFenceCounterKey(definitionID, resourceKey),
        d.buildStrictTokenKey(definitionID, resourceKey),
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
| `definition.go` | New: Definition type, Define, Variant, Composite, ForceRelease |
| `binding.go` | Modify: Add definitionID to useCaseConfig |
| `registry.go` | Modify: newUseCaseCore accepts binding parameter |
| `client_validation.go` | Modify: normalizeUseCase passes definitionID to SDK |
| `internal/sdk/usecase.go` | Modify: Add NewUseCaseWithID function |
| `internal/sdk/request.go` | No changes needed |
| `backend/contracts.go` | Modify: Add ForceReleaseDefinition to Driver interface |
| `backend/redis/driver.go` | Modify: Implement ForceReleaseDefinition |
| `definition_test.go` | New: unit tests for Definition + Variant API |
| `examples/core/definition-variant/main.go` | New: example usage |

### Testing Strategy

1. **Unit tests:**
   - Define creates stable definitionID
   - Variant inherits definitionID from base
   - Variant name = baseName.variantName
   - Composite with variant members
   - Lineage with variant parent/child
   - Hold variant inherits definitionID
   - ForceRelease calls backend with correct definitionID

2. **Integration tests:**
   - Two variants of same definition → mutual exclusion (one acquires, other fails)
   - Composite with variant members → atomic acquire
   - Lineage with variant parent/child → overlap rejection works
   - Hold variant → acquire/release works with shared key
   - Force release behavior (cleans lease + auxiliary keys, idempotent)

3. **Backward compatibility tests:**
   - Non-variant usecases unchanged (derive definitionID from name)
   - Mixed environment (some variants, some not) coexist

### Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Variant binding override confusion | Variants cannot override binding — enforced at API level |
| Composite with duplicate variant members | Composite validation already handles duplicate member IDs |
| Lineage with cross-definition parent/child | Lineage checks use definitionID — cross-definition works naturally |
| ForceReleaseDefinition interface change | New method on existing interface — all implementations must add it |
