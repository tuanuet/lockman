package definitions

import "testing"

func TestKeyBuilderDeterminism(t *testing.T) {
	builder, err := NewTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"})
	if err != nil {
		t.Fatalf("unexpected builder error: %v", err)
	}

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
	builder, err := NewTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"})
	if err != nil {
		t.Fatalf("unexpected builder error: %v", err)
	}

	_, err = builder.Build(map[string]string{
		"order_id": "123",
	})
	if err == nil {
		t.Fatal("expected error when required field is missing")
	}
}

func TestNewTemplateKeyBuilderValidation(t *testing.T) {
	tests := []struct {
		name     string
		template string
		fields   []string
	}{
		{
			name:     "duplicate fields",
			template: "lock:{id}",
			fields:   []string{"id", "id"},
		},
		{
			name:     "missing placeholder",
			template: "order:{order_id}",
			fields:   []string{"order_id", "item_id"},
		},
		{
			name:     "empty field name",
			template: "lock:{id}",
			fields:   []string{"id", ""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewTemplateKeyBuilder(tc.template, tc.fields); err == nil {
				t.Fatalf("expected error for %s configuration", tc.name)
			}
		})
	}
}

func TestMustTemplateKeyBuilderPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid configuration")
		}
	}()
	MustTemplateKeyBuilder("lock:{id}", []string{"id", "id"})
}
