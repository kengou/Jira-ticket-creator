package cmd

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/kengou/Jira-ticket-creator/internal/jira"
)

const (
	testJiraURL    = "https://jira.example.com"
	testJiraToken  = "my-token"
	testProjectKey = "PROJ"
)

// --- validateEpicsFlags ---

func TestValidateEpicsFlags_AllPresent(t *testing.T) {
	origURL := jiraURL
	origToken := jiraToken
	origProject := projectKey
	origStatus := epicStatus
	defer func() {
		jiraURL = origURL
		jiraToken = origToken
		projectKey = origProject
		epicStatus = origStatus
	}()

	jiraURL = testJiraURL
	jiraToken = testJiraToken
	projectKey = testProjectKey
	epicStatus = ""

	if err := validateEpicsFlags(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateEpicsFlags_WithStatus(t *testing.T) {
	origURL := jiraURL
	origToken := jiraToken
	origProject := projectKey
	origStatus := epicStatus
	defer func() {
		jiraURL = origURL
		jiraToken = origToken
		projectKey = origProject
		epicStatus = origStatus
	}()

	jiraURL = testJiraURL
	jiraToken = testJiraToken
	projectKey = testProjectKey
	epicStatus = "In Progress"

	if err := validateEpicsFlags(); err != nil {
		t.Fatalf("unexpected error with status set: %v", err)
	}
}

func TestValidateEpicsFlags_MissingURL(t *testing.T) {
	origURL := jiraURL
	origToken := jiraToken
	origProject := projectKey
	origStatus := epicStatus
	defer func() {
		jiraURL = origURL
		jiraToken = origToken
		projectKey = origProject
		epicStatus = origStatus
	}()

	jiraURL = ""
	jiraToken = testJiraToken
	projectKey = testProjectKey

	if err := validateEpicsFlags(); err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestValidateEpicsFlags_MissingToken(t *testing.T) {
	origURL := jiraURL
	origToken := jiraToken
	origProject := projectKey
	origStatus := epicStatus
	defer func() {
		jiraURL = origURL
		jiraToken = origToken
		projectKey = origProject
		epicStatus = origStatus
	}()

	jiraURL = testJiraURL
	jiraToken = ""
	projectKey = testProjectKey

	if err := validateEpicsFlags(); err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestValidateEpicsFlags_MissingProjectKey(t *testing.T) {
	origURL := jiraURL
	origToken := jiraToken
	origProject := projectKey
	origStatus := epicStatus
	defer func() {
		jiraURL = origURL
		jiraToken = origToken
		projectKey = origProject
		epicStatus = origStatus
	}()

	jiraURL = testJiraURL
	jiraToken = testJiraToken
	projectKey = ""

	if err := validateEpicsFlags(); err == nil {
		t.Fatal("expected error for missing project key")
	}
}

// --- epicsToYAML ---

func TestEpicsToYAML_BasicOutput(t *testing.T) {
	epics := []jira.Epic{
		{Key: "PROJ-1", Summary: "Epic One", Status: "Open", Description: "Description of epic one."},
		{Key: "PROJ-2", Summary: "Epic Two", Status: "In Progress", Description: "Description of epic two."},
	}

	data, err := epicsToYAML(testProjectKey, epics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(data)

	// Verify schemaVersion
	if !strings.Contains(output, "schemaVersion: \"1.0\"") {
		t.Error("expected schemaVersion: \"1.0\" in output")
	}

	// Verify defaults.projectKey
	if !strings.Contains(output, "projectKey: PROJ") {
		t.Error("expected projectKey: PROJ in output")
	}

	// Verify issues are present
	if !strings.Contains(output, "issueType: Epic") {
		t.Error("expected issueType: Epic in output")
	}

	// Verify descriptions are included
	if !strings.Contains(output, "Description of epic one.") {
		t.Error("expected description of epic one in output")
	}
	if !strings.Contains(output, "Description of epic two.") {
		t.Error("expected description of epic two in output")
	}

	// Verify it unmarshals back correctly
	var cfg epicsYAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to unmarshal generated YAML: %v", err)
	}

	if cfg.SchemaVersion != "1.0" {
		t.Errorf("schemaVersion = %q, want %q", cfg.SchemaVersion, "1.0")
	}
	if cfg.Defaults.ProjectKey != testProjectKey {
		t.Errorf("defaults.projectKey = %q, want %q", cfg.Defaults.ProjectKey, "PROJ")
	}
	if len(cfg.Issues) != 2 {
		t.Fatalf("len(issues) = %d, want 2", len(cfg.Issues))
	}
	if cfg.Issues[0].ID != "PROJ-1" {
		t.Errorf("issues[0].id = %q, want %q", cfg.Issues[0].ID, "PROJ-1")
	}
	if cfg.Issues[0].EpicName != "Epic One" {
		t.Errorf("issues[0].epicName = %q, want %q", cfg.Issues[0].EpicName, "Epic One")
	}
	if cfg.Issues[0].Description != "Description of epic one." {
		t.Errorf("issues[0].description = %q, want %q", cfg.Issues[0].Description, "Description of epic one.")
	}
	if cfg.Issues[1].ID != "PROJ-2" {
		t.Errorf("issues[1].id = %q, want %q", cfg.Issues[1].ID, "PROJ-2")
	}
}

func TestEpicsToYAML_EmptyEpics(t *testing.T) {
	data, err := epicsToYAML(testProjectKey, []jira.Epic{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var cfg epicsYAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to unmarshal generated YAML: %v", err)
	}

	if len(cfg.Issues) != 0 {
		t.Errorf("len(issues) = %d, want 0", len(cfg.Issues))
	}
	if cfg.Defaults.ProjectKey != testProjectKey {
		t.Errorf("defaults.projectKey = %q, want %q", cfg.Defaults.ProjectKey, "PROJ")
	}
}

func TestEpicsToYAML_NoDescription(t *testing.T) {
	epics := []jira.Epic{
		{Key: "PROJ-10", Summary: "Epic Without Description", Status: "Done"},
	}

	data, err := epicsToYAML(testProjectKey, epics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(data)

	// Description field should be omitted when empty (omitempty)
	if strings.Contains(output, "description:") {
		t.Error("expected description field to be omitted when empty")
	}

	var cfg epicsYAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to unmarshal generated YAML: %v", err)
	}

	if cfg.Issues[0].Description != "" {
		t.Errorf("issues[0].description = %q, want empty", cfg.Issues[0].Description)
	}
}

func TestEpicsToYAML_MultilineDescription(t *testing.T) {
	desc := "Line one.\nLine two.\nLine three."
	epics := []jira.Epic{
		{Key: "PROJ-5", Summary: "Epic With Multiline", Status: "Open", Description: desc},
	}

	data, err := epicsToYAML(testProjectKey, epics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var cfg epicsYAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to unmarshal generated YAML: %v", err)
	}

	if cfg.Issues[0].Description != desc {
		t.Errorf("issues[0].description = %q, want %q", cfg.Issues[0].Description, desc)
	}
}
