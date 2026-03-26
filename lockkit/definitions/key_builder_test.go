package definitions

import "testing"

func TestKeyBuilderDeterminism(t *testing.T) {
	builder := NewTemplateBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"})

	first := map[string]string{
		"order_id": "123",
		"item_id":  "789",
	}
	second := map[string]string{
		"item_id":  "789",
		"order_id": "123",
	}

	firstResult, err := builder.Build(first)
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}

	secondResult, err := builder.Build(second)
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}

	if firstResult != secondResult {
		t.Fatalf("expected deterministic result; got %q and %q", firstResult, secondResult)
	}

	expected := "order:123:item:789"
	if firstResult != expected {
		t.Fatalf("expected %q but got %q", expected, firstResult)
	}
}

func TestKeyBuilderMissingField(t *testing.T) {
	builder := NewTemplateBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"})

	_, err := builder.Build(map[string]string{
		"order_id": "123",
	})
	if err == nil {
		t.Fatal("expected error when required field is missing")
	}
}
