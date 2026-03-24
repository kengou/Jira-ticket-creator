// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunApply_ReviewedMarkerSkippedWhenReviewedTrue(t *testing.T) {
	// A file without "# reviewed: false" should not trigger the review gate.
	content := `schemaVersion: "1.0"
defaults:
  projectKey: TEST
issues:
  - id: t1
    issueType: Task
    summary: "A task"
`
	tmp := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmp, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	// Save and restore globals.
	origCfg := configFile
	origDryRun := dryRun
	t.Cleanup(func() {
		configFile = origCfg
		dryRun = origDryRun
	})

	configFile = tmp
	dryRun = true

	err := runApply()
	// The function may fail downstream (e.g. schema validation, etc.) but
	// it must NOT fail with the review-gate message.
	if err != nil && strings.Contains(err.Error(), "reviewed") {
		t.Errorf("unexpected review gate error: %v", err)
	}
}

func TestRunApply_ReviewedMarkerGate_DryRun(t *testing.T) {
	// In dry-run mode the reviewed-marker gate prints a warning but does not
	// prompt, so the function should get past the gate.
	content := `# ai-generated: true
# reviewed: false
schemaVersion: "1.0"
defaults:
  projectKey: TEST
issues:
  - id: t1
    issueType: Task
    summary: "A task"
`
	tmp := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmp, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	origCfg := configFile
	origDryRun := dryRun
	t.Cleanup(func() {
		configFile = origCfg
		dryRun = origDryRun
	})

	configFile = tmp
	dryRun = true

	err := runApply()
	// The function must NOT fail with the review-gate abort.
	if err != nil && strings.Contains(err.Error(), "aborted") {
		t.Errorf("dry-run should not abort on review gate: %v", err)
	}
}

func TestRunApply_SchemaValidationFailsOnInvalidYAML(t *testing.T) {
	// A file with an extra unknown top-level key should fail schema validation.
	content := `schemaVersion: "1.0"
defaults:
  projectKey: TEST
unknownTopLevelKey: bad
issues:
  - id: t1
    issueType: Task
    summary: "A task"
`
	tmp := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmp, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	origCfg := configFile
	origDryRun := dryRun
	t.Cleanup(func() {
		configFile = origCfg
		dryRun = origDryRun
	})

	configFile = tmp
	dryRun = true

	err := runApply()
	if err == nil {
		t.Fatal("expected schema validation error, got nil")
	}
	if !strings.Contains(err.Error(), "schema validation") {
		t.Errorf("expected 'schema validation' in error, got: %v", err)
	}
}
