// SPDX-License-Identifier: Apache-2.0
package validation_test

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/kengou/jira-ticket-creator/internal/validation"
)

// minimalValidYAML is the smallest document that satisfies the schema.
const minimalValidYAML = `
schemaVersion: "1.0"
defaults:
  projectKey: DEMO
issues:
  - id: STORY-001
    issueType: Story
    summary: A valid story
`

func TestEmbeddedSchemaParses(t *testing.T) {
	// Go tests run with cwd = package directory, so "stories.schema.json" is
	// the embedded copy that ships with the binary. Verifying it is valid JSON
	// catches trailing-comma bugs before they reach the embedded schema.
	data, err := os.ReadFile("stories.schema.json")
	if err != nil {
		t.Fatalf("cannot read schema file: %v", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("stories.schema.json is not valid JSON: %v", err)
	}
	// Also verify the embedded copy loads and resolves correctly.
	if err := validation.ValidateRawYAML([]byte(minimalValidYAML)); err != nil {
		t.Fatalf("embedded schema failed to load (ValidateRawYAML on valid doc): %v", err)
	}
}

func TestValidateRawYAML_Valid(t *testing.T) {
	if err := validation.ValidateRawYAML([]byte(minimalValidYAML)); err != nil {
		t.Errorf("expected nil for valid YAML, got: %v", err)
	}
}

func TestValidateRawYAML_ExtraTopLevelKey(t *testing.T) {
	yaml := minimalValidYAML + "\n__proto__: attack\n"
	err := validation.ValidateRawYAML([]byte(yaml))
	assertSchemaViolation(t, err, "extra top-level key")
}

func TestValidateRawYAML_ExtraIssueKey(t *testing.T) {
	yaml := `
schemaVersion: "1.0"
defaults:
  projectKey: DEMO
issues:
  - id: STORY-001
    issueType: Story
    summary: A story
    execCmd: "rm -rf /"
`
	err := validation.ValidateRawYAML([]byte(yaml))
	assertSchemaViolation(t, err, "extra key in issue object")
}

func TestValidateRawYAML_ExtraDefaultsKey(t *testing.T) {
	yaml := `
schemaVersion: "1.0"
defaults:
  projectKey: DEMO
  internalDebug: true
issues:
  - id: STORY-001
    issueType: Story
    summary: A story
`
	err := validation.ValidateRawYAML([]byte(yaml))
	assertSchemaViolation(t, err, "extra key in defaults")
}

func TestValidateRawYAML_MissingRequired(t *testing.T) {
	// Issue missing required "id" field.
	yaml := `
schemaVersion: "1.0"
defaults:
  projectKey: DEMO
issues:
  - issueType: Story
    summary: A story without an id
`
	err := validation.ValidateRawYAML([]byte(yaml))
	assertSchemaViolation(t, err, "missing required field id")
}

func TestValidateRawYAML_WrongType_SchemaVersion(t *testing.T) {
	// schemaVersion must be a string, not a number.
	yaml := `
schemaVersion: 10
defaults:
  projectKey: DEMO
issues:
  - id: STORY-001
    issueType: Story
    summary: A story
`
	err := validation.ValidateRawYAML([]byte(yaml))
	assertSchemaViolation(t, err, "schemaVersion wrong type")
}

func TestValidateRawYAML_EmptyIssues(t *testing.T) {
	// issues must have minItems: 1.
	yaml := `
schemaVersion: "1.0"
defaults:
  projectKey: DEMO
issues: []
`
	err := validation.ValidateRawYAML([]byte(yaml))
	assertSchemaViolation(t, err, "empty issues array")
}

func TestValidateRawYAML_InvalidYAML(t *testing.T) {
	// Malformed YAML should return a parse error, not a SchemaViolationError.
	err := validation.ValidateRawYAML([]byte("{unclosed: [bracket"))
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	var sve *validation.SchemaViolationError
	if errors.As(err, &sve) {
		t.Errorf("expected parse error, got SchemaViolationError: %v", err)
	}
}

func TestValidateRawYAML_NotAnObject(t *testing.T) {
	// A YAML list at the root violates "type: object".
	yaml := "- just a list\n"
	err := validation.ValidateRawYAML([]byte(yaml))
	assertSchemaViolation(t, err, "root is not an object")
}

func TestValidateRawYAML_SummaryTooLong(t *testing.T) {
	longSummary := strings.Repeat("x", 300)
	yaml := `
schemaVersion: "1.0"
defaults:
  projectKey: DEMO
issues:
  - id: STORY-001
    issueType: Story
    summary: "` + longSummary + `"
`
	err := validation.ValidateRawYAML([]byte(yaml))
	assertSchemaViolation(t, err, "summary exceeds maxLength")
}

func TestValidateRawYAML_SchemaVersionWrongConst(t *testing.T) {
	// schemaVersion must be exactly "1.0".
	yaml := `
schemaVersion: "2.0"
defaults:
  projectKey: DEMO
issues:
  - id: STORY-001
    issueType: Story
    summary: A story
`
	err := validation.ValidateRawYAML([]byte(yaml))
	assertSchemaViolation(t, err, "schemaVersion wrong const")
}

func TestSchemaViolationError_Message(t *testing.T) {
	e := &validation.SchemaViolationError{Violations: []string{"foo: bar", "baz: qux"}}
	msg := e.Error()
	if !strings.Contains(msg, "2 violation") {
		t.Errorf("expected violation count in message, got: %s", msg)
	}
	if !strings.Contains(msg, "foo: bar") || !strings.Contains(msg, "baz: qux") {
		t.Errorf("expected all violations in message, got: %s", msg)
	}
}

// assertSchemaViolation checks that err is a non-nil *SchemaViolationError.
func assertSchemaViolation(t *testing.T, err error, label string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected SchemaViolationError, got nil", label)
	}
	var sve *validation.SchemaViolationError
	if !errors.As(err, &sve) {
		t.Fatalf("%s: expected *SchemaViolationError, got %T: %v", label, err, err)
	}
	if len(sve.Violations) == 0 {
		t.Errorf("%s: SchemaViolationError has no violations", label)
	}
}
