//go:build lockman_examples

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func TestSharedDefinitionUseCasesAreDefined(t *testing.T) {
	if contractImport.DefinitionID() != "contract.import" {
		t.Fatalf("expected import use case DefinitionID %q, got %q", "contract.import", contractImport.DefinitionID())
	}
	if contractHold.DefinitionID() != "contract.manual_hold" {
		t.Fatalf("expected hold use case DefinitionID %q, got %q", "contract.manual_hold", contractHold.DefinitionID())
	}
}

func TestSharedDefinitionUsesSameLockDefinition(t *testing.T) {
	// Both use cases should resolve to the same underlying definition ID.
	importReq, err := contractImport.With(contractInput{ContractID: "42"})
	if err != nil {
		t.Fatalf("import With returned error: %v", err)
	}
	holdReq, err := contractHold.With(contractInput{ContractID: "42"})
	if err != nil {
		t.Fatalf("hold With returned error: %v", err)
	}

	// Both should produce the same resource key since they share the definition.
	if importReq.ResourceKey() != holdReq.ResourceKey() {
		t.Fatalf("expected same resource key for shared definition, got import=%q hold=%q",
			importReq.ResourceKey(), holdReq.ResourceKey())
	}
}

func TestRunPrintsSharedDefinitionExample(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run failed: %v", err)
	}
	defer redisServer.Close()

	var out bytes.Buffer
	client := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer client.Close()

	if err := run(&out, client); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"import use case: contract.import",
		"hold use case: contract.manual_hold",
		"shared definition ID: contract",
		"import resource key: contract:42",
		"hold resource key: contract:42",
		"teaching point: multiple use cases can share a single lock definition",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
