package cmd

import (
	"testing"

	"github.com/kengou/Jira-ticket-creator/internal/config"
	"github.com/kengou/Jira-ticket-creator/internal/jira"
)

// --- validateConfig ---

func TestValidateConfig_ValidConfig(t *testing.T) {
	cfg := validConfig()
	errs := validateConfig(cfg)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %d: %v", len(errs), errs)
	}
}

// --- validateSchema ---

func TestValidateSchema_MissingSchemaVersion(t *testing.T) {
	cfg := validConfig()
	cfg.SchemaVersion = ""

	errs := validateSchema(cfg)
	if !hasError(errs, "schemaVersion", "error") {
		t.Error("expected error for missing schemaVersion")
	}
}

func TestValidateSchema_UnsupportedSchemaVersion(t *testing.T) {
	cfg := validConfig()
	cfg.SchemaVersion = "2.0"

	errs := validateSchema(cfg)
	if !hasError(errs, "schemaVersion", "warning") {
		t.Error("expected warning for unsupported schemaVersion")
	}
}

func TestValidateSchema_MissingProjectKey(t *testing.T) {
	cfg := validConfig()
	cfg.Defaults.ProjectKey = ""

	errs := validateSchema(cfg)
	if !hasError(errs, "defaults.projectKey", "error") {
		t.Error("expected error for missing projectKey")
	}
}

func TestValidateSchema_NoIssues(t *testing.T) {
	cfg := validConfig()
	cfg.Issues = nil

	errs := validateSchema(cfg)
	if !hasError(errs, "issues", "error") {
		t.Error("expected error for empty issues")
	}
}

func TestValidateSchema_MissingIssueID(t *testing.T) {
	cfg := validConfig()
	cfg.Issues[0].ID = ""

	errs := validateSchema(cfg)
	if !hasError(errs, "id", "error") {
		t.Error("expected error for missing issue ID")
	}
}

func TestValidateSchema_MissingIssueType(t *testing.T) {
	cfg := validConfig()
	cfg.Issues[0].IssueType = ""
	cfg.Defaults.IssueType = "" // no default either

	errs := validateSchema(cfg)
	if !hasError(errs, "issueType", "error") {
		t.Error("expected error for missing issueType")
	}
}

func TestValidateSchema_IssueTypeFromDefault(t *testing.T) {
	cfg := validConfig()
	cfg.Issues[0].IssueType = ""
	cfg.Defaults.IssueType = "Story" // issue gets type from default

	errs := validateSchema(cfg)
	if hasError(errs, "issueType", "error") {
		t.Error("should NOT error when issueType comes from defaults")
	}
}

func TestValidateSchema_MissingSummary(t *testing.T) {
	cfg := validConfig()
	cfg.Issues[0].Summary = ""

	errs := validateSchema(cfg)
	if !hasError(errs, "summary", "error") {
		t.Error("expected error for missing summary")
	}
}

func TestValidateSchema_LongSummary(t *testing.T) {
	cfg := validConfig()
	cfg.Issues[0].Summary = string(make([]byte, 300))

	errs := validateSchema(cfg)
	if !hasError(errs, "summary", "warning") {
		t.Error("expected warning for long summary")
	}
}

func TestValidateSchema_EpicWithoutEpicName(t *testing.T) {
	cfg := validConfig()
	cfg.Issues[0].IssueType = "Epic"
	cfg.Issues[0].EpicName = ""

	errs := validateSchema(cfg)
	if !hasError(errs, "epicName", "error") {
		t.Error("expected error for Epic without epicName")
	}
}

func TestValidateSchema_EpicWithEpicName(t *testing.T) {
	cfg := validConfig()
	cfg.Issues[0].IssueType = "Epic"
	cfg.Issues[0].EpicName = "My Epic"

	errs := validateSchema(cfg)
	if hasError(errs, "epicName", "error") {
		t.Error("should NOT error for Epic with epicName")
	}
}

// --- validateBusinessLogic ---

func TestValidateBusinessLogic_DuplicateIDs(t *testing.T) {
	cfg := validConfig()
	cfg.Issues = append(cfg.Issues, config.Issue{
		ID:        "task-1", // duplicate
		IssueType: "Task",
		Summary:   "Duplicate",
	})

	errs := validateBusinessLogic(cfg)
	if !hasError(errs, "id", "error") {
		t.Error("expected error for duplicate IDs")
	}
}

func TestValidateBusinessLogic_AllowedIssueTypes(t *testing.T) {
	cfg := validConfig()
	cfg.Validation = &config.Validation{
		AllowedIssueTypes: []string{"Story", "Bug"},
	}
	cfg.Issues[0].IssueType = "Feature" // not allowed

	errs := validateBusinessLogic(cfg)
	if !hasError(errs, "issueType", "error") {
		t.Error("expected error for disallowed issue type")
	}
}

func TestValidateBusinessLogic_AllowedPriorities(t *testing.T) {
	cfg := validConfig()
	cfg.Validation = &config.Validation{
		AllowedPriorities: []string{"High", "Medium", "Low"},
	}
	cfg.Issues[0].Priority = "Critical" // not allowed

	errs := validateBusinessLogic(cfg)
	if !hasError(errs, "priority", "error") {
		t.Error("expected error for disallowed priority")
	}
}

func TestValidateBusinessLogic_MissingParentRef(t *testing.T) {
	cfg := validConfig()
	cfg.Issues[0].Parent = "nonexistent"

	errs := validateBusinessLogic(cfg)
	if !hasError(errs, "parent", "error") {
		t.Error("expected error for missing parent reference")
	}
}

func TestValidateBusinessLogic_MissingDependsOnRef(t *testing.T) {
	cfg := validConfig()
	cfg.Issues[0].DependsOn = []string{"nonexistent"}

	errs := validateBusinessLogic(cfg)
	if !hasError(errs, "dependsOn", "error") {
		t.Error("expected error for missing dependency reference")
	}
}

func TestValidateBusinessLogic_MissingLinkTarget(t *testing.T) {
	cfg := validConfig()
	cfg.Issues[0].Links = []config.IssueLink{
		{Type: "blocks", Target: "nonexistent"},
	}

	errs := validateBusinessLogic(cfg)
	if !hasError(errs, "links", "error") {
		t.Error("expected error for missing link target")
	}
}

func TestValidateBusinessLogic_ValidLinkTarget(t *testing.T) {
	cfg := validConfig()
	cfg.Issues = append(cfg.Issues, config.Issue{
		ID:        "task-2",
		IssueType: "Task",
		Summary:   "Task 2",
	})
	cfg.Issues[0].Links = []config.IssueLink{
		{Type: "blocks", Target: "task-2"},
	}

	errs := validateBusinessLogic(cfg)
	if hasError(errs, "links", "error") {
		t.Errorf("should not error for valid link target, got %v", errs)
	}
}

func TestValidateBusinessLogic_UncommonLinkType(t *testing.T) {
	cfg := validConfig()
	cfg.Issues = append(cfg.Issues, config.Issue{
		ID:        "task-2",
		IssueType: "Task",
		Summary:   "Task 2",
	})
	cfg.Issues[0].Links = []config.IssueLink{
		{Type: "weird-link-type", Target: "task-2"},
	}

	errs := validateBusinessLogic(cfg)
	if !hasError(errs, "links", "warning") {
		t.Error("expected warning for uncommon link type")
	}
}

func TestValidateBusinessLogic_BlockedByIsCommon(t *testing.T) {
	cfg := validConfig()
	cfg.Issues = append(cfg.Issues, config.Issue{
		ID:        "task-2",
		IssueType: "Task",
		Summary:   "Task 2",
	})
	cfg.Issues[0].Links = []config.IssueLink{
		{Type: "blocked by", Target: "task-2"},
	}

	errs := validateBusinessLogic(cfg)
	if hasError(errs, "links", "warning") {
		t.Errorf("should not warn for 'blocked by' link type, got %v", errs)
	}
}

func TestValidateBusinessLogic_ExternalJiraKeyLinkTarget(t *testing.T) {
	cfg := validConfig()
	cfg.Issues[0].Links = []config.IssueLink{
		{Type: "blocked by", Target: "POM-1052"},
	}

	errs := validateBusinessLogic(cfg)
	if hasError(errs, "links", "error") {
		t.Errorf("should not error for external Jira key link target, got %v", errs)
	}
}

func TestValidateBusinessLogic_InvalidLinkTargetStillErrors(t *testing.T) {
	cfg := validConfig()
	cfg.Issues[0].Links = []config.IssueLink{
		{Type: "blocks", Target: "some-internal-id"},
	}

	errs := validateBusinessLogic(cfg)
	if !hasError(errs, "links", "error") {
		t.Error("expected error for non-existent internal link target")
	}
}

// --- jira.IsJiraKey ---

func TestIsJiraKey_Valid(t *testing.T) {
	keys := []string{"POM-1", "POM-1052", "ABC-123", "A1-1", "PROJ-99999"}
	for _, k := range keys {
		if !jira.IsJiraKey(k) {
			t.Errorf("jira.IsJiraKey(%q) = false, want true", k)
		}
	}
}

func TestIsJiraKey_Invalid(t *testing.T) {
	notKeys := []string{"task-1", "my-story", "STORY-PLAT-005", "pom-1", "POM", "POM-", "-123", "123", ""}
	for _, k := range notKeys {
		if jira.IsJiraKey(k) {
			t.Errorf("jira.IsJiraKey(%q) = true, want false", k)
		}
	}
}

// --- checkCircularDependencies ---

func TestCheckCircularDependencies_NoCycle(t *testing.T) {
	issues := []config.Issue{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b"},
	}

	errs := checkCircularDependencies(issues)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestCheckCircularDependencies_DirectCycle(t *testing.T) {
	issues := []config.Issue{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	}

	errs := checkCircularDependencies(issues)
	if len(errs) == 0 {
		t.Error("expected error for circular dependency")
	}
}

func TestCheckCircularDependencies_IndirectCycle(t *testing.T) {
	issues := []config.Issue{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"c"}},
		{ID: "c", DependsOn: []string{"a"}},
	}

	errs := checkCircularDependencies(issues)
	if len(errs) == 0 {
		t.Error("expected error for indirect circular dependency")
	}
}

func TestCheckCircularDependencies_SelfCycle(t *testing.T) {
	issues := []config.Issue{
		{ID: "a", DependsOn: []string{"a"}},
	}

	errs := checkCircularDependencies(issues)
	if len(errs) == 0 {
		t.Error("expected error for self-referencing dependency")
	}
}

func TestCheckCircularDependencies_ParentCycle(t *testing.T) {
	issues := []config.Issue{
		{ID: "a", Parent: "b"},
		{ID: "b", Parent: "a"},
	}

	errs := checkCircularDependencies(issues)
	if len(errs) == 0 {
		t.Error("expected error for parent circular dependency")
	}
}

// --- isCommonLinkType ---

func TestIsCommonLinkType(t *testing.T) {
	common := []string{
		"blocks", "Blocks",
		"is blocked by",
		"relates to",
		"duplicates",
		"clones",
		"depends on",
	}
	for _, lt := range common {
		if !isCommonLinkType(lt) {
			t.Errorf("isCommonLinkType(%q) = false, want true", lt)
		}
	}

	uncommon := []string{
		"custom-link",
		"causes",
		"",
	}
	for _, lt := range uncommon {
		if isCommonLinkType(lt) {
			t.Errorf("isCommonLinkType(%q) = true, want false", lt)
		}
	}
}

// --- validateParentChild ---

func TestValidateParentChild_ValidStoryUnderEpic(t *testing.T) {
	parent := config.Issue{IssueType: "Epic"}
	child := config.Issue{IssueType: "Story"}

	if err := validateParentChild(parent, child); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateParentChild_SubtaskUnderStory(t *testing.T) {
	parent := config.Issue{IssueType: "Story"}
	child := config.Issue{IssueType: "Subtask"}

	if err := validateParentChild(parent, child); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateParentChild_SubtaskUnderTask(t *testing.T) {
	parent := config.Issue{IssueType: "Task"}
	child := config.Issue{IssueType: "Subtask"}

	if err := validateParentChild(parent, child); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateParentChild_SubtaskUnderBug(t *testing.T) {
	parent := config.Issue{IssueType: "Bug"}
	child := config.Issue{IssueType: "Subtask"}

	if err := validateParentChild(parent, child); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateParentChild_SubtaskUnderEpic(t *testing.T) {
	parent := config.Issue{IssueType: "Epic"}
	child := config.Issue{IssueType: "Subtask"}

	if err := validateParentChild(parent, child); err == nil {
		t.Error("expected error: Subtask cannot have Epic as parent")
	}
}

func TestValidateParentChild_SubtaskParent(t *testing.T) {
	parent := config.Issue{IssueType: "Subtask"}
	child := config.Issue{IssueType: "Task"}

	if err := validateParentChild(parent, child); err == nil {
		t.Error("expected error: Subtask cannot be a parent")
	}
}

func TestValidateParentChild_SubtaskUnderSubtask(t *testing.T) {
	parent := config.Issue{IssueType: "Subtask"}
	child := config.Issue{IssueType: "Subtask"}

	if err := validateParentChild(parent, child); err == nil {
		t.Error("expected error: Subtask cannot be parent of Subtask")
	}
}

// --- validateTemplates ---

func TestValidateTemplates_MissingRequiredVar(t *testing.T) {
	cfg := validConfig()
	cfg.Defaults.DescriptionTemplate = "Goal: {goal}"
	cfg.Validation = &config.Validation{
		RequiredFields: map[string][]string{
			"Task": {"goal", "owner"},
		},
	}
	cfg.Issues[0].IssueType = "Task"
	cfg.Issues[0].TemplateVars = map[string]string{
		"goal": "ship it",
		// "owner" is missing
	}

	errs := validateTemplates(cfg)
	if !hasError(errs, "templateVars", "error") {
		t.Error("expected error for missing required template variable")
	}
}

func TestValidateTemplates_AllRequiredVarsPresent(t *testing.T) {
	cfg := validConfig()
	cfg.Defaults.DescriptionTemplate = "Goal: {goal}"
	cfg.Validation = &config.Validation{
		RequiredFields: map[string][]string{
			"Task": {"goal"},
		},
	}
	cfg.Issues[0].IssueType = "Task"
	cfg.Issues[0].TemplateVars = map[string]string{
		"goal": "ship it",
	}

	errs := validateTemplates(cfg)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateTemplates_InheritedIssueType(t *testing.T) {
	// Issue inherits type "Task" from defaults (issue.IssueType == "").
	// The required "goal" var is absent but another var is present,
	// so the template-vars check is triggered.
	cfg := validConfig()
	cfg.Defaults.DescriptionTemplate = "Owner: {owner}\nGoal: {goal}"
	cfg.Defaults.IssueType = "Task"
	cfg.Validation = &config.Validation{
		RequiredFields: map[string][]string{
			"Task": {"goal", "owner"},
		},
	}
	cfg.Issues[0].IssueType = "" // inherited from defaults
	cfg.Issues[0].TemplateVars = map[string]string{
		"owner": "alice",
		// "goal" is intentionally missing
	}

	errs := validateTemplates(cfg)
	if !hasError(errs, "templateVars", "error") {
		t.Error("expected error for missing required template variable with inherited issue type")
	}
}

func TestValidateTemplates_NoTemplateConfigured(t *testing.T) {
	cfg := validConfig()
	cfg.Defaults.DescriptionTemplate = ""

	errs := validateTemplates(cfg)
	if len(errs) != 0 {
		t.Errorf("expected no errors when no template configured, got %v", errs)
	}
}

func TestValidateTemplates_SkipsSummaryAndDescription(t *testing.T) {
	cfg := validConfig()
	cfg.Defaults.DescriptionTemplate = "Template"
	cfg.Validation = &config.Validation{
		RequiredFields: map[string][]string{
			"Task": {"summary", "description"},
		},
	}
	cfg.Issues[0].IssueType = "Task"
	cfg.Issues[0].TemplateVars = map[string]string{} // no vars needed for summary/description

	errs := validateTemplates(cfg)
	if len(errs) != 0 {
		t.Errorf("summary/description should be skipped in template validation, got %v", errs)
	}
}

// --- ValidationError.String ---

func TestValidationError_String_WithIssueID(t *testing.T) {
	e := ValidationError{
		IssueID:  "task-1",
		Field:    "summary",
		Message:  "is required",
		Severity: "error",
	}
	got := e.String()
	want := "[error] Issue task-1: summary - is required"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestValidationError_String_WithoutIssueID(t *testing.T) {
	e := ValidationError{
		Field:    "schemaVersion",
		Message:  "is required",
		Severity: "error",
	}
	got := e.String()
	want := "[error] schemaVersion - is required"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// --- helpers ---

func validConfig() *config.Config {
	return &config.Config{
		SchemaVersion: "1.0",
		Defaults: config.Defaults{
			ProjectKey: "PROJ",
			IssueType:  "Task",
		},
		Options:    &config.Options{},
		Validation: &config.Validation{},
		Issues: []config.Issue{
			{
				ID:        "task-1",
				IssueType: "Task",
				Summary:   "A task",
			},
		},
	}
}

func hasError(errs []ValidationError, field, severity string) bool {
	for _, e := range errs {
		if e.Field == field && e.Severity == severity {
			return true
		}
	}
	return false
}
