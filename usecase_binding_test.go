package lockman

import (
	"errors"
	"strings"
	"testing"
)

func TestRunUseCaseWithBindsCanonicalResourceKey(t *testing.T) {
	uc := DefineRun[string](
		"order.approve",
		BindResourceID("order", func(v string) string { return v }),
	)

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	if req.resourceKey != "order:123" {
		t.Fatalf("expected canonical resource key, got %q", req.resourceKey)
	}
}

func TestRunUseCaseWithRejectsEmptyBoundResourceID(t *testing.T) {
	uc := DefineRun[string](
		"order.approve",
		BindResourceID("order", func(v string) string { return v }),
	)

	_, err := uc.With("")
	if err == nil {
		t.Fatal("expected empty bound resource id to fail")
	}
	if !errors.Is(err, errEmptyBindingValue) {
		t.Fatalf("expected errEmptyBindingValue, got %v", err)
	}
}

func TestRunUseCaseWithRejectsInvalidResourcePrefix(t *testing.T) {
	cases := []struct {
		name     string
		resource string
	}{
		{name: "empty resource", resource: "   "},
		{name: "contains colon", resource: "order:sub"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := DefineRun[string](
				"order.approve",
				BindResourceID(tc.resource, func(v string) string { return v }),
			)

			_, err := uc.With("123")
			if err == nil {
				t.Fatal("expected invalid resource prefix error")
			}
		})
	}
}

func TestRunUseCaseWithTrimsResourcePrefix(t *testing.T) {
	uc := DefineRun[string](
		"order.approve",
		BindResourceID(" order ", func(v string) string { return v }),
	)

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	if req.resourceKey != "order:123" {
		t.Fatalf("expected trimmed canonical key, got %q", req.resourceKey)
	}
}

func TestBindKeyRejectsEmptyBoundKeyWithGenericError(t *testing.T) {
	uc := DefineRun[string](
		"order.approve",
		BindKey(func(v string) string { return v }),
	)

	_, err := uc.With("  ")
	if err == nil {
		t.Fatal("expected empty bound key to fail")
	}
	if !errors.Is(err, errEmptyBindingValue) {
		t.Fatalf("expected errEmptyBindingValue, got %v", err)
	}
}

func TestRunUseCaseWithNilBindResourceIDFunctionReturnsErrorNotPanic(t *testing.T) {
	uc := DefineRun[string](
		"order.approve",
		BindResourceID[string]("order", nil),
	)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected error, got panic: %v", recovered)
		}
	}()

	_, err := uc.With("123")
	if err == nil {
		t.Fatal("expected nil binding function error")
	}
	if !errors.Is(err, errBindingFunctionRequired) {
		t.Fatalf("expected errBindingFunctionRequired, got %v", err)
	}
}

func TestRunUseCaseWithNilBindKeyFunctionReturnsErrorNotPanic(t *testing.T) {
	uc := DefineRun[string](
		"order.approve",
		BindKey[string](nil),
	)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected error, got panic: %v", recovered)
		}
	}()

	_, err := uc.With("123")
	if err == nil {
		t.Fatal("expected nil binding function error")
	}
	if !errors.Is(err, errBindingFunctionRequired) {
		t.Fatalf("expected errBindingFunctionRequired, got %v", err)
	}
}

func TestRunUseCaseWithRejectsEmptyOwnerOverride(t *testing.T) {
	uc := DefineRun[string](
		"order.approve",
		BindKey(func(v string) string { return v }),
	)

	_, err := uc.With("key-1", OwnerID("   "))
	if err == nil {
		t.Fatal("expected empty owner override to fail")
	}
	if !errors.Is(err, ErrIdentityRequired) {
		t.Fatalf("expected ErrIdentityRequired, got %v", err)
	}
}

func TestClaimUseCaseWithRejectsInvalidDelivery(t *testing.T) {
	uc := DefineClaim[string](
		"order.process",
		BindResourceID("order", func(v string) string { return v }),
		Idempotent(),
	)

	cases := []struct {
		name     string
		delivery Delivery
	}{
		{
			name: "missing message id",
			delivery: Delivery{
				ConsumerGroup: "orders",
				Attempt:       1,
			},
		},
		{
			name: "missing consumer group",
			delivery: Delivery{
				MessageID: "msg-1",
				Attempt:   1,
			},
		},
		{
			name: "non-positive attempt",
			delivery: Delivery{
				MessageID:     "msg-1",
				ConsumerGroup: "orders",
				Attempt:       0,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := uc.With("123", tc.delivery)
			if err == nil {
				t.Fatal("expected delivery validation error")
			}
			if !strings.Contains(err.Error(), "delivery") {
				t.Fatalf("expected delivery validation error, got %v", err)
			}
		})
	}
}

func TestClaimUseCaseWithRejectsEmptyOwnerOverride(t *testing.T) {
	uc := DefineClaim[string](
		"order.process",
		BindResourceID("order", func(v string) string { return v }),
	)

	_, err := uc.With("123", Delivery{
		MessageID:     "msg-1",
		ConsumerGroup: "orders",
		Attempt:       1,
	}, OwnerID("   "))
	if err == nil {
		t.Fatal("expected empty owner override to fail")
	}
	if !errors.Is(err, ErrIdentityRequired) {
		t.Fatalf("expected ErrIdentityRequired, got %v", err)
	}
}
