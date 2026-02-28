package config

import (
	"os"
	"path/filepath"
	"testing"
)

// --- LoadConfig ---

func TestLoadConfig_ValidFile(t *testing.T) {
	yaml := `
schemaVersion: "1.0"
defaults:
  projectKey: TEST
  issueType: Story
issues:
  - id: story-1
    summary: A story
`
	path := writeTestYAML(t, yaml)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SchemaVersion != "1.0" {
		t.Errorf("schemaVersion = %q, want %q", cfg.SchemaVersion, "1.0")
	}
	if cfg.Defaults.ProjectKey != "TEST" {
		t.Errorf("projectKey = %q, want %q", cfg.Defaults.ProjectKey, "TEST")
	}
	if len(cfg.Issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(cfg.Issues))
	}
	if cfg.Issues[0].ID != "story-1" {
		t.Errorf("issue ID = %q, want %q", cfg.Issues[0].ID, "story-1")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	path := writeTestYAML(t, `{{{invalid`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// --- applyDefaults ---

func TestApplyDefaults_SetsOptionsWhenNil(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()

	if cfg.Options == nil {
		t.Fatal("Options should not be nil after applyDefaults")
	}
	if cfg.Options.MaxConcurrency != 5 {
		t.Errorf("MaxConcurrency = %d, want 5", cfg.Options.MaxConcurrency)
	}
	if cfg.Validation == nil {
		t.Fatal("Validation should not be nil after applyDefaults")
	}
}

func TestApplyDefaults_PreservesExistingOptions(t *testing.T) {
	cfg := &Config{
		Options: &Options{
			MaxConcurrency: 10,
		},
	}
	cfg.applyDefaults()

	if cfg.Options.MaxConcurrency != 10 {
		t.Errorf("MaxConcurrency = %d, want 10 (should be preserved)", cfg.Options.MaxConcurrency)
	}
}

func TestApplyDefaults_SetsMaxConcurrencyWhenZero(t *testing.T) {
	cfg := &Config{
		Options: &Options{MaxConcurrency: 0},
	}
	cfg.applyDefaults()

	if cfg.Options.MaxConcurrency != 5 {
		t.Errorf("MaxConcurrency = %d, want 5 (default)", cfg.Options.MaxConcurrency)
	}
}

// --- IsIdempotencyEnabled ---

func TestIsIdempotencyEnabled_NilOptions(t *testing.T) {
	var o *Options
	if !o.IsIdempotencyEnabled() {
		t.Error("nil Options should default to enabled")
	}
}

func TestIsIdempotencyEnabled_NilField(t *testing.T) {
	o := &Options{}
	if !o.IsIdempotencyEnabled() {
		t.Error("nil IdempotencyEnabled should default to true")
	}
}

func TestIsIdempotencyEnabled_True(t *testing.T) {
	v := true
	o := &Options{IdempotencyEnabled: &v}
	if !o.IsIdempotencyEnabled() {
		t.Error("expected true when explicitly set to true")
	}
}

func TestIsIdempotencyEnabled_False(t *testing.T) {
	v := false
	o := &Options{IdempotencyEnabled: &v}
	if o.IsIdempotencyEnabled() {
		t.Error("expected false when explicitly set to false")
	}
}

// --- EffectiveIssueType ---

func TestEffectiveIssueType_IssueOverride(t *testing.T) {
	cfg := &Config{Defaults: Defaults{IssueType: "Story"}}
	issue := &Issue{IssueType: "Bug"}

	if got := cfg.EffectiveIssueType(issue); got != "Bug" {
		t.Errorf("EffectiveIssueType = %q, want %q", got, "Bug")
	}
}

func TestEffectiveIssueType_FallsBackToDefault(t *testing.T) {
	cfg := &Config{Defaults: Defaults{IssueType: "Story"}}
	issue := &Issue{}

	if got := cfg.EffectiveIssueType(issue); got != "Story" {
		t.Errorf("EffectiveIssueType = %q, want %q", got, "Story")
	}
}

func TestEffectiveIssueType_EmptyEverywhere(t *testing.T) {
	cfg := &Config{}
	issue := &Issue{}

	if got := cfg.EffectiveIssueType(issue); got != "" {
		t.Errorf("EffectiveIssueType = %q, want empty", got)
	}
}

// --- EffectivePriority ---

func TestEffectivePriority_IssueOverride(t *testing.T) {
	cfg := &Config{Defaults: Defaults{Priority: "Low"}}
	issue := &Issue{Priority: "High"}

	if got := cfg.EffectivePriority(issue); got != "High" {
		t.Errorf("EffectivePriority = %q, want %q", got, "High")
	}
}

func TestEffectivePriority_DefaultsToConfigDefault(t *testing.T) {
	cfg := &Config{Defaults: Defaults{Priority: "Low"}}
	issue := &Issue{}

	if got := cfg.EffectivePriority(issue); got != "Low" {
		t.Errorf("EffectivePriority = %q, want %q", got, "Low")
	}
}

func TestEffectivePriority_FallsBackToEmpty(t *testing.T) {
	cfg := &Config{}
	issue := &Issue{}

	if got := cfg.EffectivePriority(issue); got != "" {
		t.Errorf("EffectivePriority = %q, want %q", got, "")
	}
}

// --- EffectiveAssignee ---

func TestEffectiveAssignee_IssueOverride(t *testing.T) {
	defaultAssignee := "default-user"
	cfg := &Config{Defaults: Defaults{Assignee: &defaultAssignee}}
	issue := &Issue{Assignee: "issue-user"}

	got := cfg.EffectiveAssignee(issue)
	if got == nil || *got != "issue-user" {
		t.Errorf("EffectiveAssignee = %v, want %q", got, "issue-user")
	}
}

func TestEffectiveAssignee_FallsBackToDefault(t *testing.T) {
	defaultAssignee := "default-user"
	cfg := &Config{Defaults: Defaults{Assignee: &defaultAssignee}}
	issue := &Issue{}

	got := cfg.EffectiveAssignee(issue)
	if got == nil || *got != "default-user" {
		t.Errorf("EffectiveAssignee = %v, want %q", got, "default-user")
	}
}

func TestEffectiveAssignee_NilWhenNoDefault(t *testing.T) {
	cfg := &Config{}
	issue := &Issue{}

	if got := cfg.EffectiveAssignee(issue); got != nil {
		t.Errorf("EffectiveAssignee = %v, want nil", got)
	}
}

// --- EffectiveReporter ---

func TestEffectiveReporter_IssueOverride(t *testing.T) {
	cfg := &Config{Defaults: Defaults{Reporter: "default-reporter"}}
	issue := &Issue{Reporter: "issue-reporter"}

	if got := cfg.EffectiveReporter(issue); got != "issue-reporter" {
		t.Errorf("EffectiveReporter = %q, want %q", got, "issue-reporter")
	}
}

func TestEffectiveReporter_FallsBackToDefault(t *testing.T) {
	cfg := &Config{Defaults: Defaults{Reporter: "default-reporter"}}
	issue := &Issue{}

	if got := cfg.EffectiveReporter(issue); got != "default-reporter" {
		t.Errorf("EffectiveReporter = %q, want %q", got, "default-reporter")
	}
}

// --- EffectiveLabels ---

func TestEffectiveLabels_MergesDefaultsAndIssue(t *testing.T) {
	cfg := &Config{Defaults: Defaults{Labels: []string{"team-a", "sprint-1"}}}
	issue := &Issue{Labels: []string{"sprint-1", "feature"}}

	got := cfg.EffectiveLabels(issue)
	want := []string{"team-a", "sprint-1", "feature"}

	if len(got) != len(want) {
		t.Fatalf("len(labels) = %d, want %d", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("label[%d] = %q, want %q", i, got[i], v)
		}
	}
}

func TestEffectiveLabels_NoDuplicates(t *testing.T) {
	cfg := &Config{Defaults: Defaults{Labels: []string{"a", "b"}}}
	issue := &Issue{Labels: []string{"b", "c"}}

	got := cfg.EffectiveLabels(issue)
	if len(got) != 3 {
		t.Errorf("expected 3 unique labels, got %d: %v", len(got), got)
	}
}

func TestEffectiveLabels_EmptyBothSides(t *testing.T) {
	cfg := &Config{}
	issue := &Issue{}

	got := cfg.EffectiveLabels(issue)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// --- EffectiveComponents ---

func TestEffectiveComponents_IssueOverride(t *testing.T) {
	cfg := &Config{Defaults: Defaults{Components: []string{"backend"}}}
	issue := &Issue{Components: []string{"frontend"}}

	got := cfg.EffectiveComponents(issue)
	if len(got) != 1 || got[0] != "frontend" {
		t.Errorf("EffectiveComponents = %v, want [frontend]", got)
	}
}

func TestEffectiveComponents_FallsBackToDefault(t *testing.T) {
	cfg := &Config{Defaults: Defaults{Components: []string{"backend"}}}
	issue := &Issue{}

	got := cfg.EffectiveComponents(issue)
	if len(got) != 1 || got[0] != "backend" {
		t.Errorf("EffectiveComponents = %v, want [backend]", got)
	}
}

// --- EffectiveFixVersions ---

func TestEffectiveFixVersions_IssueOverride(t *testing.T) {
	cfg := &Config{Defaults: Defaults{FixVersions: []string{"v1.0"}}}
	issue := &Issue{FixVersions: []string{"v2.0"}}

	got := cfg.EffectiveFixVersions(issue)
	if len(got) != 1 || got[0] != "v2.0" {
		t.Errorf("EffectiveFixVersions = %v, want [v2.0]", got)
	}
}

func TestEffectiveFixVersions_FallsBackToDefault(t *testing.T) {
	cfg := &Config{Defaults: Defaults{FixVersions: []string{"v1.0"}}}
	issue := &Issue{}

	got := cfg.EffectiveFixVersions(issue)
	if len(got) != 1 || got[0] != "v1.0" {
		t.Errorf("EffectiveFixVersions = %v, want [v1.0]", got)
	}
}

// --- EffectiveCustomFields ---

func TestEffectiveCustomFields_MergesWithOverride(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{
			CustomFields: map[string]any{
				"cf_team": "alpha",
				"cf_env":  "prod",
			},
		},
	}
	issue := &Issue{
		CustomFields: map[string]any{
			"cf_env":    "staging", // overrides default
			"cf_region": "eu",      // new field
		},
	}

	got := cfg.EffectiveCustomFields(issue)

	if got["cf_team"] != "alpha" {
		t.Errorf("cf_team = %v, want %q", got["cf_team"], "alpha")
	}
	if got["cf_env"] != "staging" {
		t.Errorf("cf_env = %v, want %q (override)", got["cf_env"], "staging")
	}
	if got["cf_region"] != "eu" {
		t.Errorf("cf_region = %v, want %q", got["cf_region"], "eu")
	}
}

func TestEffectiveCustomFields_EmptyBothSides(t *testing.T) {
	cfg := &Config{}
	issue := &Issue{}

	got := cfg.EffectiveCustomFields(issue)
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

// --- LoadConfig with defaults applied ---

func TestLoadConfig_AppliesDefaults(t *testing.T) {
	yaml := `
schemaVersion: "1.0"
defaults:
  projectKey: PROJ
issues:
  - id: t1
    issueType: Task
    summary: Test
`
	path := writeTestYAML(t, yaml)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Options should be auto-populated
	if cfg.Options == nil {
		t.Fatal("Options should be set by applyDefaults")
	}
	if cfg.Options.MaxConcurrency != 5 {
		t.Errorf("MaxConcurrency = %d, want 5", cfg.Options.MaxConcurrency)
	}
	// Idempotency should default to enabled
	if !cfg.Options.IsIdempotencyEnabled() {
		t.Error("idempotency should default to enabled")
	}
	// Validation should be auto-populated
	if cfg.Validation == nil {
		t.Fatal("Validation should be set by applyDefaults")
	}
}

func TestLoadConfig_IdempotencyExplicitlyDisabled(t *testing.T) {
	yaml := `
schemaVersion: "1.0"
defaults:
  projectKey: PROJ
options:
  idempotencyEnabled: false
issues:
  - id: t1
    issueType: Task
    summary: Test
`
	path := writeTestYAML(t, yaml)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Options.IsIdempotencyEnabled() {
		t.Error("idempotency should be disabled when explicitly set to false")
	}
}

// --- helpers ---

func writeTestYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	return path
}
