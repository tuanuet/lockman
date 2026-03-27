package definitions

import (
	"fmt"
	"strings"
)

// KeyBuilder produces deterministic resource keys from required input.
type KeyBuilder interface {
	RequiredFields() []string
	Build(input map[string]string) (string, error)
}

// TemplateBuilderMetadata exposes the template string and ordered fields from a template backed KeyBuilder.
type TemplateBuilderMetadata struct {
	Template string
	Fields   []string
}

// TemplateMetadata tries to view the builder as a template backed KeyBuilder and returns the metadata if available.
func TemplateMetadata(builder KeyBuilder) (TemplateBuilderMetadata, bool) {
	view, ok := builder.(interface {
		TemplateMetadata() TemplateBuilderMetadata
	})
	if !ok {
		return TemplateBuilderMetadata{}, false
	}
	return view.TemplateMetadata(), true
}

type templateKeyBuilder struct {
	template string
	fields   []string
}

// NewTemplateKeyBuilder returns a KeyBuilder that fills placeholders from the provided template.
// Fields are required and define the replacement order. Duplicate or empty field names are rejected,
// and the template must already contain a matching placeholder for each field.
func NewTemplateKeyBuilder(template string, fields []string) (KeyBuilder, error) {
	if fields == nil {
		fields = []string{}
	}

	seen := make(map[string]struct{}, len(fields))
	ordered := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			return nil, fmt.Errorf("field names must not be empty")
		}
		if _, exists := seen[field]; exists {
			return nil, fmt.Errorf("duplicate field name %q", field)
		}
		seen[field] = struct{}{}
		ordered = append(ordered, field)
	}

	for _, field := range ordered {
		placeholder := "{" + field + "}"
		if !strings.Contains(template, placeholder) {
			return nil, fmt.Errorf("template missing placeholder for field %s", placeholder)
		}
	}

	fieldsCopy := make([]string, len(ordered))
	copy(fieldsCopy, ordered)
	return &templateKeyBuilder{
		template: template,
		fields:   fieldsCopy,
	}, nil
}

// MustTemplateKeyBuilder is like NewTemplateKeyBuilder but panics on configuration errors.
func MustTemplateKeyBuilder(template string, fields []string) KeyBuilder {
	builder, err := NewTemplateKeyBuilder(template, fields)
	if err != nil {
		panic(err)
	}
	return builder
}

func (t *templateKeyBuilder) RequiredFields() []string {
	fieldsCopy := make([]string, len(t.fields))
	copy(fieldsCopy, t.fields)
	return fieldsCopy
}

func (t *templateKeyBuilder) Build(input map[string]string) (string, error) {
	if input == nil {
		return "", fmt.Errorf("input map must not be nil")
	}

	replacements := make([]string, 0, len(t.fields)*2)
	for _, field := range t.fields {
		value, ok := input[field]
		if !ok {
			return "", fmt.Errorf("missing required field: %s", field)
		}
		replacements = append(replacements, "{"+field+"}", value)
	}

	return strings.NewReplacer(replacements...).Replace(t.template), nil
}

func (t *templateKeyBuilder) TemplateMetadata() TemplateBuilderMetadata {
	fieldsCopy := make([]string, len(t.fields))
	copy(fieldsCopy, t.fields)
	return TemplateBuilderMetadata{
		Template: t.template,
		Fields:   fieldsCopy,
	}
}
