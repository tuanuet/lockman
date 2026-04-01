package sdk

import (
	"strings"
	"testing"
)

func TestHoldTokenRoundTrip(t *testing.T) {
	resourceKeys := []string{"order:123"}
	ownerID := "owner-a"

	token, err := EncodeHoldToken(resourceKeys, ownerID)
	if err != nil {
		t.Fatalf("EncodeHoldToken returned error: %v", err)
	}

	decodedKeys, decodedOwnerID, err := DecodeHoldToken(token)
	if err != nil {
		t.Fatalf("DecodeHoldToken returned error: %v", err)
	}
	if len(decodedKeys) != 1 || decodedKeys[0] != resourceKeys[0] {
		t.Fatalf("expected decoded keys %v, got %v", resourceKeys, decodedKeys)
	}
	if decodedOwnerID != ownerID {
		t.Fatalf("expected decoded owner %q, got %q", ownerID, decodedOwnerID)
	}
}

func TestHoldTokenMultipleKeys(t *testing.T) {
	resourceKeys := []string{"acct:1", "acct:2", "acct:3"}
	ownerID := "owner-b"

	token, err := EncodeHoldToken(resourceKeys, ownerID)
	if err != nil {
		t.Fatalf("EncodeHoldToken returned error: %v", err)
	}

	decodedKeys, decodedOwnerID, err := DecodeHoldToken(token)
	if err != nil {
		t.Fatalf("DecodeHoldToken returned error: %v", err)
	}
	if len(decodedKeys) != len(resourceKeys) {
		t.Fatalf("expected %d keys, got %d", len(resourceKeys), len(decodedKeys))
	}
	for i := range resourceKeys {
		if decodedKeys[i] != resourceKeys[i] {
			t.Fatalf("expected key %d to be %q, got %q", i, resourceKeys[i], decodedKeys[i])
		}
	}
	if decodedOwnerID != ownerID {
		t.Fatalf("expected decoded owner %q, got %q", ownerID, decodedOwnerID)
	}
}

func TestHoldTokenUnknownVersionPrefix(t *testing.T) {
	_, _, err := DecodeHoldToken("h2_abc")
	if err == nil {
		t.Fatal("expected error for unknown version prefix")
	}
}

func TestHoldTokenMalformedBase64(t *testing.T) {
	_, _, err := DecodeHoldToken("h1_%%%%")
	if err == nil {
		t.Fatal("expected error for malformed base64 payload")
	}
}

func TestHoldTokenEmptyString(t *testing.T) {
	_, _, err := DecodeHoldToken("")
	if err == nil {
		t.Fatal("expected error for empty hold token")
	}
}

func TestHoldTokenHasExpectedPrefix(t *testing.T) {
	token, err := EncodeHoldToken([]string{"resource:1"}, "owner-c")
	if err != nil {
		t.Fatalf("EncodeHoldToken returned error: %v", err)
	}
	if !strings.HasPrefix(token, "h1_") {
		t.Fatalf("expected hold token prefix %q, got %q", "h1_", token)
	}
}
