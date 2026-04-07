// SPDX-License-Identifier: Apache-2.0

package validation

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"sigs.k8s.io/yaml"
)

//go:embed stories.schema.json
var embeddedSchema []byte

var (
	onceSchema      sync.Once
	cachedSchema    *jsonschema.Resolved
	errCachedSchema error
)

func resolveSchema() (*jsonschema.Resolved, error) {
	onceSchema.Do(func() {
		var s jsonschema.Schema
		if err := json.Unmarshal(embeddedSchema, &s); err != nil {
			errCachedSchema = fmt.Errorf("internal: embedded schema is invalid: %w", err)
			return
		}
		rs, err := s.Resolve(nil)
		if err != nil {
			errCachedSchema = fmt.Errorf("internal: cannot resolve embedded schema: %w", err)
			return
		}
		cachedSchema = rs
	})
	return cachedSchema, errCachedSchema
}

// SchemaViolationError is returned by ValidateRawYAML when the document does not
// conform to the config schema. It is distinct from the per-field validation.Error
// type used by ValidateConfig — this check runs before any deserialization occurs.
type SchemaViolationError struct {
	// Violations lists each schema constraint that was violated.
	Violations []string
}

func (e *SchemaViolationError) Error() string {
	return fmt.Sprintf("schema validation failed (%d violation(s)):\n  - %s",
		len(e.Violations), strings.Join(e.Violations, "\n  - "))
}

// ValidateRawYAML validates raw YAML bytes against the embedded config schema
// before any deserialization into Go structs (OWASP ASVS V5.5).
//
// It converts YAML to JSON, compiles the embedded JSON Schema (Draft-07),
// and validates the document against it. Any structural violation — unexpected
// keys, wrong types, missing required fields, pattern/length failures — is
// returned as a *SchemaViolationError before the caller ever calls yaml.Unmarshal.
//
// Returns nil if the document is schema-valid.
// Returns a plain error if the bytes cannot be parsed as YAML.
// Returns *SchemaViolationError if the document violates the schema.
func ValidateRawYAML(data []byte) error {
	// 1. Convert YAML → JSON. sigs.k8s.io/yaml does this via the YAML-to-JSON
	//    bridge, the same path used internally by yaml.Unmarshal, so the
	//    conversion is lossless for all types valid in the config schema.
	jsonBytes, err := yaml.YAMLToJSON(data)
	if err != nil {
		return fmt.Errorf("cannot parse YAML: %w", err)
	}

	// 2+3. Use the cached, pre-resolved schema (parsed and resolved once via sync.Once).
	rs, err := resolveSchema()
	if err != nil {
		return err
	}

	// 4. Unmarshal the JSON document into a generic value for schema validation.
	//    Using any produces map[string]any / []any / primitive — the types the
	//    jsonschema library expects.
	var doc any
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return fmt.Errorf("cannot parse document: %w", err)
	}

	// 5. Validate. The library returns a descriptive error on the first violation.
	if err := rs.Validate(doc); err != nil {
		return &SchemaViolationError{Violations: []string{err.Error()}}
	}

	return nil
}
