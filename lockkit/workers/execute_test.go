package workers

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
	"lockman/lockkit/idempotency"
	"lockman/lockkit/internal/policy"
	"lockman/lockkit/registry"
	"lockman/lockkit/testkit"
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
		renewErr: drivers.ErrLeaseExpired,
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

func (d *postCallbackRenewFailDriver) Acquire(ctx context.Context, req drivers.AcquireRequest) (drivers.LeaseRecord, error) {
	return d.base.Acquire(ctx, req)
}

func (d *postCallbackRenewFailDriver) Renew(ctx context.Context, lease drivers.LeaseRecord) (drivers.LeaseRecord, error) {
	select {
	case <-d.renewStartedCh:
	default:
		close(d.renewStartedCh)
	}
	<-d.allowRenewResult
	return drivers.LeaseRecord{}, drivers.ErrLeaseExpired
}

func (d *postCallbackRenewFailDriver) Release(ctx context.Context, lease drivers.LeaseRecord) error {
	return d.base.Release(ctx, lease)
}

func (d *postCallbackRenewFailDriver) CheckPresence(ctx context.Context, req drivers.PresenceRequest) (drivers.PresenceRecord, error) {
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

func (d *renewFailDriver) Acquire(ctx context.Context, req drivers.AcquireRequest) (drivers.LeaseRecord, error) {
	return d.base.Acquire(ctx, req)
}

func (d *renewFailDriver) Renew(ctx context.Context, lease drivers.LeaseRecord) (drivers.LeaseRecord, error) {
	if d.renewSeen.CompareAndSwap(false, true) {
		return drivers.LeaseRecord{}, d.renewErr
	}
	return d.base.Renew(ctx, lease)
}

func (d *renewFailDriver) Release(ctx context.Context, lease drivers.LeaseRecord) error {
	return d.base.Release(ctx, lease)
}

func (d *renewFailDriver) CheckPresence(ctx context.Context, req drivers.PresenceRequest) (drivers.PresenceRecord, error) {
	return d.base.CheckPresence(ctx, req)
}

func (d *renewFailDriver) Ping(ctx context.Context) error {
	return d.base.Ping(ctx)
}
