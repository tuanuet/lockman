package definitions

import (
	"fmt"
	"slices"
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
	template     string
	fields       []string
	placeholders []string // pre-computed: "{field_name}"
	segments     []templateSegment
}

type templateSegment struct {
	literal    string
	fieldIndex int
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
	placeholders := make([]string, len(ordered))
	for i, field := range ordered {
		placeholders[i] = "{" + field + "}"
	}
	type placeholderMatch struct {
		start      int
		end        int
		fieldIndex int
	}
	matches := make([]placeholderMatch, 0, len(placeholders))
	for i, placeholder := range placeholders {
		searchFrom := 0
		for {
			idx := strings.Index(template[searchFrom:], placeholder)
			if idx < 0 {
				break
			}
			start := searchFrom + idx
			matches = append(matches, placeholderMatch{
				start:      start,
				end:        start + len(placeholder),
				fieldIndex: i,
			})
			searchFrom = start + len(placeholder)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("template missing placeholder for field %s", placeholder)
		}
	}
	slices.SortFunc(matches, func(a, b placeholderMatch) int {
		return a.start - b.start
	})
	segments := make([]templateSegment, 0, len(matches)*2+1)
	last := 0
	for _, match := range matches {
		segments = append(segments, templateSegment{literal: template[last:match.start], fieldIndex: -1})
		segments = append(segments, templateSegment{fieldIndex: match.fieldIndex})
		last = match.end
	}
	segments = append(segments, templateSegment{literal: template[last:], fieldIndex: -1})
	return &templateKeyBuilder{
		template:     template,
		fields:       fieldsCopy,
		placeholders: placeholders,
		segments:     segments,
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

	// Fast path: single field — use strings.Replace directly
	if len(t.fields) == 1 {
		value, ok := input[t.fields[0]]
		if !ok {
			return "", fmt.Errorf("missing required field: %s", t.fields[0])
		}
		return strings.ReplaceAll(t.template, t.placeholders[0], value), nil
	}

	// Multi-field: build replacer from pre-computed placeholders
	values := make([]string, len(t.fields))
	totalLen := 0
	for i, field := range t.fields {
		value, ok := input[field]
		if !ok {
			return "", fmt.Errorf("missing required field: %s", field)
		}
		values[i] = value
		totalLen += len(value)
	}
	for _, segment := range t.segments {
		totalLen += len(segment.literal)
	}

	var b strings.Builder
	b.Grow(totalLen)
	for _, segment := range t.segments {
		if segment.fieldIndex >= 0 {
			b.WriteString(values[segment.fieldIndex])
			continue
		}
		b.WriteString(segment.literal)
	}
	return b.String(), nil
}

func (t *templateKeyBuilder) TemplateMetadata() TemplateBuilderMetadata {
	fieldsCopy := make([]string, len(t.fields))
	copy(fieldsCopy, t.fields)
	return TemplateBuilderMetadata{
		Template: t.template,
		Fields:   fieldsCopy,
	}
}
