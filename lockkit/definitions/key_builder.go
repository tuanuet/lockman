package definitions

import (
	"errors"
	"fmt"
	"strings"
)

type KeyBuilder interface {
	RequiredFields() []string
	Build(input map[string]string) (string, error)
}

type templateBuilder struct {
	template string
	fields   []string
}

func NewTemplateBuilder(template string, fields []string) KeyBuilder {
	fieldsCopy := make([]string, len(fields))
	copy(fieldsCopy, fields)
	return &templateBuilder{
		template: template,
		fields:   fieldsCopy,
	}
}

func (t *templateBuilder) RequiredFields() []string {
	fieldsCopy := make([]string, len(t.fields))
	copy(fieldsCopy, t.fields)
	return fieldsCopy
}

func (t *templateBuilder) Build(input map[string]string) (string, error) {
	if input == nil {
		return "", errors.New("input map must not be nil")
	}

	replacements := make([]string, 0, len(t.fields)*2)
	for _, field := range t.fields {
		value, ok := input[field]
		if !ok {
			return "", fmt.Errorf("missing required field: %s", field)
		}
		replacements = append(replacements, "{"+field+"}", value)
	}

	builder := strings.NewReplacer(replacements...)
	return builder.Replace(t.template), nil
}
