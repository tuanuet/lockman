package definitions

import (
	"reflect"
	"testing"
)

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

func TestTemplateKeyBuilderExposesTemplateMetadata(t *testing.T) {
	builder := MustTemplateKeyBuilder(
		"order:{order_id}:item:{item_id}",
		[]string{"order_id", "item_id"},
	)

	meta, ok := TemplateMetadata(builder)
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

func TestTemplateKeyBuilderReplacesRepeatedPlaceholder(t *testing.T) {
	builder, err := NewTemplateKeyBuilder("order:{order_id}:copy:{order_id}", []string{"order_id"})
	if err != nil {
		t.Fatalf("unexpected builder error: %v", err)
	}

	got, err := builder.Build(map[string]string{"order_id": "42"})
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	if got != "order:42:copy:42" {
		t.Fatalf("expected repeated placeholder replacement, got %q", got)
	}
}
