package workers

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/idempotency"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/internal/policy"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/lockkit/testkit"
	"github.com/tuanuet/lockman/observe"
)

type workerManagerHarness struct {
	*Manager
	testStore *idempotency.MemoryStore
}

func TestExecuteClaimedPersistsIdempotencyBeforeAck(t *testing.T) {
	mgr := newWorkerManagerForTest(t)

	err := mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		if claim.IdempotencyKey == "" {
			t.Fatal("expected claim idempotency key")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}

	record, err := mgr.testStore.Get(context.Background(), "msg:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusCompleted {
		t.Fatalf("expected completed status, got %q", record.Status)
	}
}

func TestExecuteClaimedCancelsContextWhenRenewalFails(t *testing.T) {
	mgr := newWorkerManagerWithRenewFailure(t)

	err := mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, lockerrors.ErrLeaseLost) {
		t.Fatalf("expected lease lost after renew failure, got %v", err)
	}
}

func TestExecuteClaimedRejectsInProgressDuplicateWithoutCallback(t *testing.T) {
	mgr := newWorkerManagerForTest(t)
	if _, err := mgr.testStore.Begin(context.Background(), "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       1,
		TTL:           30 * time.Second,
	}); err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	called := false
	err := mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		called = true
		return nil
	})
	if !errors.Is(err, lockerrors.ErrLockBusy) {
		t.Fatalf("expected retry-mapped error for in-progress duplicate, got %v", err)
	}
	if called {
		t.Fatal("callback must not run for in-progress duplicate")
	}
}

func TestExecuteClaimedTreatsCompletedDuplicateAsAckWithoutCallback(t *testing.T) {
	mgr := newWorkerManagerForTest(t)
	if _, err := mgr.testStore.Begin(context.Background(), "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       1,
		TTL:           30 * time.Second,
	}); err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if err := mgr.testStore.Complete(context.Background(), "msg:123", idempotency.CompleteInput{
		OwnerID:   "worker-a",
		MessageID: "123",
		TTL:       5 * time.Minute,
	}); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	called := false
	err := mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		called = true
		return nil
	})
	if got := policy.OutcomeFromError(err); got != policy.OutcomeAck {
		t.Fatalf("expected terminal duplicate outcome ack, got %q (err=%v)", got, err)
	}
	if called {
		t.Fatal("callback must not run for completed duplicate")
	}
}

func TestExecuteClaimedTreatsFailedDuplicateAsAckWithoutCallback(t *testing.T) {
	mgr := newWorkerManagerForTest(t)
	if _, err := mgr.testStore.Begin(context.Background(), "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       1,
		TTL:           30 * time.Second,
	}); err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if err := mgr.testStore.Fail(context.Background(), "msg:123", idempotency.FailInput{
		OwnerID:   "worker-a",
		MessageID: "123",
		TTL:       5 * time.Minute,
	}); err != nil {
		t.Fatalf("Fail returned error: %v", err)
	}

	called := false
	err := mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		called = true
		return nil
	})
	if got := policy.OutcomeFromError(err); got != policy.OutcomeAck {
		t.Fatalf("expected failed duplicate outcome ack, got %q (err=%v)", got, err)
	}
	if called {
		t.Fatal("callback must not run for failed duplicate")
	}
	record, err := mgr.testStore.Get(context.Background(), "msg:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusFailed {
		t.Fatalf("expected failed status retention, got %q", record.Status)
	}
}

func TestExecuteClaimedReturnsRetryOutcomeForRuntimeOverlap(t *testing.T) {
	driver := testkit.NewMemoryDriver()
	reg := workerRegistryWithLineageChain(t)
	mgr := newWorkerManagerWithDriver(t, reg, driver)

	parentReq := backend.LineageAcquireRequest{
		DefinitionID: "OrderLock",
		ResourceKey:  "order:123",
		OwnerID:      "external-parent",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "parent-lease",
			Kind:    backend.KindParent,
		},
	}
	parentLease, err := driver.AcquireWithLineage(context.Background(), parentReq)
	if err != nil {
		t.Fatalf("AcquireWithLineage failed: %v", err)
	}
	defer func() {
		_ = driver.ReleaseWithLineage(context.Background(), parentLease, parentReq.Lineage)
	}()

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
	reg := workerRegistryWithLineageChain(t)
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

func TestExecuteClaimedSameProcessReentrantRejected(t *testing.T) {
	mgr := newWorkerManagerForTest(t)
	req := messageClaimRequest()
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- mgr.ExecuteClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
			close(entered)
			<-release
			return nil
		})
	}()

	<-entered
	err := mgr.ExecuteClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
		t.Fatal("callback should not run for reentrant claim")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrReentrantAcquire) {
		t.Fatalf("expected reentrant error, got %v", err)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first ExecuteClaimed returned error: %v", err)
	}
}

func TestExecuteClaimedReentrantSameResourceDifferentOwnerRejected(t *testing.T) {
	mgr := newWorkerManagerForTest(t)
	firstReq := messageClaimRequest()

	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- mgr.ExecuteClaimed(context.Background(), firstReq, func(ctx context.Context, claim definitions.ClaimContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	secondReq := messageClaimRequest()
	secondReq.IdempotencyKey = "msg:second-owner"
	secondReq.Ownership.OwnerID = "worker-b"
	secondReq.Ownership.MessageID = "124"
	err := mgr.ExecuteClaimed(context.Background(), secondReq, func(ctx context.Context, claim definitions.ClaimContext) error {
		t.Fatal("callback should not run for same-resource reentrant claim with different owner")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrReentrantAcquire) {
		t.Fatalf("expected reentrant error, got %v", err)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first ExecuteClaimed returned error: %v", err)
	}
}

func TestExecuteClaimedValidatesIdempotencyMetadataWhenRequired(t *testing.T) {
	mgr := newWorkerManagerForTest(t)
	cases := []struct {
		name   string
		mutate func(*definitions.MessageClaimRequest)
	}{
		{
			name: "missing message id",
			mutate: func(req *definitions.MessageClaimRequest) {
				req.Ownership.MessageID = ""
			},
		},
		{
			name: "missing consumer group",
			mutate: func(req *definitions.MessageClaimRequest) {
				req.Ownership.ConsumerGroup = ""
			},
		},
		{
			name: "non-positive attempt",
			mutate: func(req *definitions.MessageClaimRequest) {
				req.Ownership.Attempt = 0
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := messageClaimRequest()
			tc.mutate(&req)

			called := false
			err := mgr.ExecuteClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
				called = true
				return nil
			})
			if !errors.Is(err, lockerrors.ErrPolicyViolation) {
				t.Fatalf("expected policy violation, got %v", err)
			}
			if called {
				t.Fatal("callback should not execute when idempotency metadata is invalid")
			}
		})
	}
}

func TestExecuteClaimedDetectsRenewalFailureAfterCallbackReturns(t *testing.T) {
	reg := newWorkerRegistryForTest(t, true)
	store := idempotency.NewMemoryStore()
	driver := newPostCallbackRenewFailDriver()

	mgr, err := NewManager(reg, driver, store)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	callbackReturned := make(chan struct{})
	go func() {
		<-callbackReturned
		driver.releaseRenew()
	}()

	err = mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		<-driver.renewStarted()
		close(callbackReturned)
		return nil
	})
	if !errors.Is(err, lockerrors.ErrLeaseLost) {
		t.Fatalf("expected lease lost when renew fails after callback return, got %v", err)
	}

	record, err := store.Get(context.Background(), "msg:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusInProgress {
		t.Fatalf("expected in-progress status for retry path, got %q", record.Status)
	}
}

func TestExecuteClaimedStrictPopulatesFencingToken(t *testing.T) {
	reg := strictExecuteWorkerRegistryForTest(t)
	mgr := newWorkerManagerWithDriver(t, reg, testkit.NewMemoryDriver())

	err := mgr.ExecuteClaimed(context.Background(), strictMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		if claim.FencingToken == 0 {
			t.Fatal("expected non-zero fencing token for strict worker execution")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}
}

func TestExecuteClaimedStrictSuiteKeepsStandardFencingTokenZero(t *testing.T) {
	mgr := newWorkerManagerForTest(t)

	err := mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		if claim.FencingToken != 0 {
			t.Fatalf("expected zero fencing token for standard worker execution, got %d", claim.FencingToken)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}
}

func TestExecuteClaimedStrictRenewalPreservesFencingToken(t *testing.T) {
	reg := strictExecuteWorkerRegistryForTest(t)
	driver := newStrictRenewProbeDriver()
	mgr := newWorkerManagerWithDriver(t, reg, driver)

	var callbackToken uint64
	err := mgr.ExecuteClaimed(context.Background(), strictMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		callbackToken = claim.FencingToken
		time.Sleep(220 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}
	if callbackToken == 0 {
		t.Fatal("expected strict callback fencing token to be non-zero")
	}
	if got := driver.renewCalls.Load(); got == 0 {
		t.Fatal("expected strict renew path to be used at least once")
	}
	if got := driver.lastRenewToken.Load(); got != callbackToken {
		t.Fatalf("expected renew token %d, got %d", callbackToken, got)
	}
}

func TestExecuteClaimedStrictRenewalTokenMismatchReturnsLeaseLost(t *testing.T) {
	reg := strictExecuteWorkerRegistryForTest(t)
	driver := newStrictRenewMismatchDriver()
	mgr := newWorkerManagerWithDriver(t, reg, driver)

	err := mgr.ExecuteClaimed(context.Background(), strictMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		time.Sleep(220 * time.Millisecond)
		return nil
	})
	if !errors.Is(err, lockerrors.ErrLeaseLost) {
		t.Fatalf("expected ErrLeaseLost on strict renew token mismatch, got %v", err)
	}
}

func TestExecuteClaimedStrictAcquireErrorReturnsDirectly(t *testing.T) {
	reg := strictExecuteWorkerRegistryForTest(t)
	sentinel := errors.New("strict acquire failed")
	mgr := newWorkerManagerWithDriver(t, reg, strictAcquireFailDriver{
		base: testkit.NewMemoryDriver(),
		err:  sentinel,
	})

	called := false
	err := mgr.ExecuteClaimed(context.Background(), strictMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		called = true
		return nil
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected strict acquire error passthrough, got %v", err)
	}
	if called {
		t.Fatal("callback should not execute when strict acquire fails")
	}
}

func TestOutcomeFromErrorMapsWorkerErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want policy.WorkerOutcome
	}{
		{name: "nil", err: nil, want: policy.OutcomeAck},
		{name: "busy", err: lockerrors.ErrLockBusy, want: policy.OutcomeRetry},
		{name: "duplicate ignored", err: lockerrors.ErrDuplicateIgnored, want: policy.OutcomeAck},
		{name: "dlq wrapped", err: policy.DLQ(errors.New("poison message")), want: policy.OutcomeDLQ},
		{name: "policy violation", err: lockerrors.ErrPolicyViolation, want: policy.OutcomeDrop},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := policy.OutcomeFromError(tc.err)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func newWorkerManagerForTest(t *testing.T) workerManagerHarness {
	t.Helper()
	reg := newWorkerRegistryForTest(t, true)
	store := idempotency.NewMemoryStore()
	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), store)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	return workerManagerHarness{
		Manager:   mgr,
		testStore: store,
	}
}

func newWorkerManagerWithRenewFailure(t *testing.T) workerManagerHarness {
	t.Helper()
	reg := newWorkerRegistryForTest(t, true)
	store := idempotency.NewMemoryStore()
	driver := &renewFailDriver{
		base:     testkit.NewMemoryDriver(),
		renewErr: backend.ErrLeaseExpired,
	}
	mgr, err := NewManager(reg, driver, store)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	return workerManagerHarness{
		Manager:   mgr,
		testStore: store,
	}
}

func newWorkerRegistryForTest(t *testing.T, idempotencyRequired bool) *registry.Registry {
	t.Helper()

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                  "MessageClaimLock",
		Kind:                definitions.KindParent,
		Resource:            "message",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionAsync,
		LeaseTTL:            90 * time.Millisecond,
		IdempotencyRequired: idempotencyRequired,
		KeyBuilder:          definitions.MustTemplateKeyBuilder("message:{message_id}", []string{"message_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	return reg
}

func newWorkerManagerWithDriver(t *testing.T, reg *registry.Registry, driver backend.Driver) *Manager {
	t.Helper()

	mgr, err := NewManager(reg, driver, idempotency.NewMemoryStore())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	return mgr
}

func registryWithShortTTLLineageChain(t *testing.T, ttl time.Duration) *registry.Registry {
	t.Helper()

	reg := registry.New()
	register := func(def definitions.LockDefinition) {
		if err := reg.Register(def); err != nil {
			t.Fatalf("register %s failed: %v", def.ID, err)
		}
	}

	register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionAsync,
		LeaseTTL:      ttl,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	})
	register(definitions.LockDefinition{
		ID:            "ItemLock",
		Kind:          definitions.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionAsync,
		LeaseTTL:      ttl,
		ParentRef:     "OrderLock",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder: definitions.MustTemplateKeyBuilder(
			"order:{order_id}:item:{item_id}",
			[]string{"order_id", "item_id"},
		),
	})

	return reg
}

func messageClaimRequest() definitions.MessageClaimRequest {
	return definitions.MessageClaimRequest{
		DefinitionID:   "MessageClaimLock",
		IdempotencyKey: "msg:123",
		KeyInput: map[string]string{
			"message_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "worker-a",
			MessageID:     "123",
			Attempt:       1,
			ConsumerGroup: "payments",
			HandlerName:   "HandlePayment",
		},
	}
}

func parentMessageClaimRequest() definitions.MessageClaimRequest {
	return definitions.MessageClaimRequest{
		DefinitionID:   "OrderLock",
		IdempotencyKey: "order:123",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "worker-parent",
			MessageID:     "123",
			Attempt:       1,
			ConsumerGroup: "payments",
			HandlerName:   "HandleOrder",
		},
	}
}

func childMessageClaimRequest() definitions.MessageClaimRequest {
	return definitions.MessageClaimRequest{
		DefinitionID:   "ItemLock",
		IdempotencyKey: "order:123:item:line-1",
		KeyInput: map[string]string{
			"order_id": "123",
			"item_id":  "line-1",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "worker-child",
			MessageID:     "line-1",
			Attempt:       1,
			ConsumerGroup: "payments",
			HandlerName:   "HandleItem",
		},
	}
}

func strictMessageClaimRequest() definitions.MessageClaimRequest {
	return definitions.MessageClaimRequest{
		DefinitionID:   "StrictMessageClaimLock",
		IdempotencyKey: "strict-msg:123",
		KeyInput: map[string]string{
			"message_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "worker-strict",
			MessageID:     "123",
			Attempt:       1,
			ConsumerGroup: "payments",
			HandlerName:   "HandleStrictPayment",
		},
	}
}

func strictExecuteWorkerRegistryForTest(t *testing.T) *registry.Registry {
	t.Helper()

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "StrictMessageClaimLock",
		Kind:                 definitions.KindParent,
		Resource:             "message",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionAsync,
		LeaseTTL:             90 * time.Millisecond,
		IdempotencyRequired:  true,
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("message:{message_id}", []string{"message_id"}),
	}); err != nil {
		t.Fatalf("register strict definition failed: %v", err)
	}
	return reg
}

type postCallbackRenewFailDriver struct {
	base             *testkit.MemoryDriver
	renewStartedCh   chan struct{}
	allowRenewResult chan struct{}
}

func newPostCallbackRenewFailDriver() *postCallbackRenewFailDriver {
	return &postCallbackRenewFailDriver{
		base:             testkit.NewMemoryDriver(),
		renewStartedCh:   make(chan struct{}),
		allowRenewResult: make(chan struct{}),
	}
}

func (d *postCallbackRenewFailDriver) renewStarted() <-chan struct{} {
	return d.renewStartedCh
}

func (d *postCallbackRenewFailDriver) releaseRenew() {
	close(d.allowRenewResult)
}

func (d *postCallbackRenewFailDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return d.base.Acquire(ctx, req)
}

func (d *postCallbackRenewFailDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	select {
	case <-d.renewStartedCh:
	default:
		close(d.renewStartedCh)
	}
	<-d.allowRenewResult
	return backend.LeaseRecord{}, backend.ErrLeaseExpired
}

func (d *postCallbackRenewFailDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return d.base.Release(ctx, lease)
}

func (d *postCallbackRenewFailDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return d.base.CheckPresence(ctx, req)
}

func (d *postCallbackRenewFailDriver) Ping(ctx context.Context) error {
	return d.base.Ping(ctx)
}

type renewFailDriver struct {
	base      *testkit.MemoryDriver
	renewErr  error
	renewSeen atomic.Bool
}

func (d *renewFailDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return d.base.Acquire(ctx, req)
}

func (d *renewFailDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	if d.renewSeen.CompareAndSwap(false, true) {
		return backend.LeaseRecord{}, d.renewErr
	}
	return d.base.Renew(ctx, lease)
}

func (d *renewFailDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return d.base.Release(ctx, lease)
}

func (d *renewFailDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return d.base.CheckPresence(ctx, req)
}

func (d *renewFailDriver) Ping(ctx context.Context) error {
	return d.base.Ping(ctx)
}

type strictRenewProbeDriver struct {
	base           *testkit.MemoryDriver
	renewCalls     atomic.Uint64
	lastRenewToken atomic.Uint64
}

func newStrictRenewProbeDriver() *strictRenewProbeDriver {
	return &strictRenewProbeDriver{base: testkit.NewMemoryDriver()}
}

func (d *strictRenewProbeDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return d.base.Acquire(ctx, req)
}

func (d *strictRenewProbeDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	return d.base.Renew(ctx, lease)
}

func (d *strictRenewProbeDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return d.base.Release(ctx, lease)
}

func (d *strictRenewProbeDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return d.base.CheckPresence(ctx, req)
}

func (d *strictRenewProbeDriver) Ping(ctx context.Context) error {
	return d.base.Ping(ctx)
}

func (d *strictRenewProbeDriver) AcquireStrict(ctx context.Context, req backend.StrictAcquireRequest) (backend.FencedLeaseRecord, error) {
	return d.base.AcquireStrict(ctx, req)
}

func (d *strictRenewProbeDriver) RenewStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) (backend.FencedLeaseRecord, error) {
	d.renewCalls.Add(1)
	d.lastRenewToken.Store(fencingToken)
	return d.base.RenewStrict(ctx, lease, fencingToken)
}

func (d *strictRenewProbeDriver) ReleaseStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) error {
	return d.base.ReleaseStrict(ctx, lease, fencingToken)
}

type strictRenewMismatchDriver struct {
	base *testkit.MemoryDriver
}

func newStrictRenewMismatchDriver() *strictRenewMismatchDriver {
	return &strictRenewMismatchDriver{base: testkit.NewMemoryDriver()}
}

func (d *strictRenewMismatchDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return d.base.Acquire(ctx, req)
}

func (d *strictRenewMismatchDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	return d.base.Renew(ctx, lease)
}

func (d *strictRenewMismatchDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return d.base.Release(ctx, lease)
}

func (d *strictRenewMismatchDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return d.base.CheckPresence(ctx, req)
}

func (d *strictRenewMismatchDriver) Ping(ctx context.Context) error {
	return d.base.Ping(ctx)
}

func (d *strictRenewMismatchDriver) AcquireStrict(ctx context.Context, req backend.StrictAcquireRequest) (backend.FencedLeaseRecord, error) {
	return d.base.AcquireStrict(ctx, req)
}

func (d *strictRenewMismatchDriver) RenewStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) (backend.FencedLeaseRecord, error) {
	updated, err := d.base.RenewStrict(ctx, lease, fencingToken)
	if err != nil {
		return backend.FencedLeaseRecord{}, err
	}
	updated.FencingToken++
	return updated, nil
}

func (d *strictRenewMismatchDriver) ReleaseStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) error {
	return d.base.ReleaseStrict(ctx, lease, fencingToken)
}

type strictAcquireFailDriver struct {
	base *testkit.MemoryDriver
	err  error
}

func (d strictAcquireFailDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return d.base.Acquire(ctx, req)
}

func (d strictAcquireFailDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	return d.base.Renew(ctx, lease)
}

func (d strictAcquireFailDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return d.base.Release(ctx, lease)
}

func (d strictAcquireFailDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return d.base.CheckPresence(ctx, req)
}

func (d strictAcquireFailDriver) Ping(ctx context.Context) error {
	return d.base.Ping(ctx)
}

func (d strictAcquireFailDriver) AcquireStrict(ctx context.Context, req backend.StrictAcquireRequest) (backend.FencedLeaseRecord, error) {
	return backend.FencedLeaseRecord{}, d.err
}

func (d strictAcquireFailDriver) RenewStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) (backend.FencedLeaseRecord, error) {
	return d.base.RenewStrict(ctx, lease, fencingToken)
}

func (d strictAcquireFailDriver) ReleaseStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) error {
	return d.base.ReleaseStrict(ctx, lease, fencingToken)
}

type acquireFailDriver struct {
	base *testkit.MemoryDriver
	err  error
}

func (d *acquireFailDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return backend.LeaseRecord{}, d.err
}

func (d *acquireFailDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	return d.base.Renew(ctx, lease)
}

func (d *acquireFailDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return d.base.Release(ctx, lease)
}

func (d *acquireFailDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return d.base.CheckPresence(ctx, req)
}

func (d *acquireFailDriver) Ping(ctx context.Context) error {
	return d.base.Ping(ctx)
}

func TestExecuteClaimedEmitsAcquireLifecycleEvents(t *testing.T) {
	reg := newWorkerRegistryForTest(t, false)
	store := idempotency.NewMemoryStore()
	var events []observe.Event
	bridge := workerTestBridge(func(event observe.Event) {
		events = append(events, event)
	})
	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), store, WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}

	if !hasEventKind(events, observe.EventAcquireStarted) {
		t.Fatal("expected acquire_started event")
	}
	if !hasEventKind(events, observe.EventAcquireSucceeded) {
		t.Fatal("expected acquire_succeeded event")
	}
	if !hasEventKind(events, observe.EventReleased) {
		t.Fatal("expected released event")
	}
}

func TestExecuteClaimedEmitsIdempotencyBeginCompletedEvents(t *testing.T) {
	reg := newWorkerRegistryForTest(t, true)
	store := idempotency.NewMemoryStore()
	var events []observe.Event
	bridge := workerTestBridge(func(event observe.Event) {
		events = append(events, event)
	})
	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), store, WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}

	if !hasEventKind(events, observe.EventAcquireStarted) {
		t.Fatal("expected acquire_started event for idempotency begin")
	}
	if !hasEventKind(events, observe.EventAcquireSucceeded) {
		t.Fatal("expected acquire_succeeded event for idempotency completed")
	}
}

func TestExecuteClaimedEmitsLeaseLostWhenRenewalFails(t *testing.T) {
	reg := newWorkerRegistryForTest(t, true)
	store := idempotency.NewMemoryStore()
	driver := &renewFailDriver{
		base:     testkit.NewMemoryDriver(),
		renewErr: backend.ErrLeaseExpired,
	}
	var events []observe.Event
	bridge := workerTestBridge(func(event observe.Event) {
		events = append(events, event)
	})
	mgr, err := NewManager(reg, driver, store, WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, lockerrors.ErrLeaseLost) {
		t.Fatalf("expected lease lost error, got %v", err)
	}
	if !hasEventKind(events, observe.EventLeaseLost) {
		t.Fatal("expected lease_lost event")
	}
}

func TestExecuteClaimedEmitsRenewalSucceededOnRenewal(t *testing.T) {
	reg := newWorkerRegistryForTest(t, true)
	store := idempotency.NewMemoryStore()
	var events []observe.Event
	bridge := workerTestBridge(func(event observe.Event) {
		events = append(events, event)
	})
	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), store, WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		time.Sleep(150 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}

	if !hasEventKind(events, observe.EventRenewalSucceeded) {
		t.Fatal("expected renewal_succeeded event")
	}
}

func TestExecuteClaimedEmitsAcquireFailedOnError(t *testing.T) {
	reg := newWorkerRegistryForTest(t, false)
	store := idempotency.NewMemoryStore()
	var events []observe.Event
	bridge := workerTestBridge(func(event observe.Event) {
		events = append(events, event)
	})
	driver := &acquireFailDriver{
		base: testkit.NewMemoryDriver(),
		err:  errors.New("acquire failed"),
	}
	mgr, err := NewManager(reg, driver, store, WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected ExecuteClaimed to return error")
	}

	if !hasEventKind(events, observe.EventAcquireFailed) {
		t.Fatal("expected acquire_failed event")
	}
}
