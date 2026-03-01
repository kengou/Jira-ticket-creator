// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kengou/Jira-ticket-creator/internal/config"
)

// ValidationError represents a single validation error.
type ValidationError struct {
	IssueID  string
	Field    string
	Message  string
	Severity string // "error" or "warning"
}

func (e ValidationError) String() string {
	if e.IssueID != "" {
		return fmt.Sprintf("[%s] Issue %s: %s - %s", e.Severity, e.IssueID, e.Field, e.Message)
	}
	return fmt.Sprintf("[%s] %s - %s", e.Severity, e.Field, e.Message)
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the YAML configuration file",
	Long:  `Validates the YAML configuration file and reports any errors or warnings.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateFlags(false); err != nil {
			return err
		}
		return runValidate()
	},
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate() error {
	fmt.Printf("🔍 Validating: %s\n\n", configFile)

	// Load config
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate
	errors := validateConfig(cfg)

	if len(errors) == 0 {
		fmt.Println("✅ Configuration is valid!")
		printSummary(cfg)
		return nil
	}

	// Count errors and warnings
	var errorCount, warningCount int
	for _, e := range errors {
		if e.Severity == "error" {
			errorCount++
			fmt.Printf("❌ %s\n", e.String())
		} else {
			warningCount++
			fmt.Printf("⚠️  %s\n", e.String())
		}
	}

	fmt.Printf("\n📊 Validation Summary:\n")
	fmt.Printf("  - Errors: %d\n", errorCount)
	fmt.Printf("  - Warnings: %d\n", warningCount)

	if errorCount > 0 {
		return fmt.Errorf("validation failed with %d errors", errorCount)
	}

	if warningCount > 0 {
		fmt.Println("\n⚠️  Configuration has warnings but is valid.")
	}

	return nil
}

func validateConfig(cfg *config.Config) []ValidationError {
	var errors []ValidationError

	// Schema validation
	errors = append(errors, validateSchema(cfg)...)

	// Business logic validation
	errors = append(errors, validateBusinessLogic(cfg)...)

	// Template validation
	errors = append(errors, validateTemplates(cfg)...)

	return errors
}

func validateSchema(cfg *config.Config) []ValidationError {
	var errors []ValidationError

	if cfg.SchemaVersion == "" {
		errors = append(errors, ValidationError{
			Field:    "schemaVersion",
			Message:  "schema version is required",
			Severity: "error",
		})
	} else if cfg.SchemaVersion != "1.0" {
		errors = append(errors, ValidationError{
			Field:    "schemaVersion",
			Message:  "unsupported schema version: " + cfg.SchemaVersion,
			Severity: "warning",
		})
	}

	if cfg.Defaults.ProjectKey == "" {
		errors = append(errors, ValidationError{
			Field:    "defaults.projectKey",
			Message:  "project key is required",
			Severity: "error",
		})
	}

	if len(cfg.Issues) == 0 {
		errors = append(errors, ValidationError{
			Field:    "issues",
			Message:  "at least one issue is required",
			Severity: "error",
		})
	}

	for _, issue := range cfg.Issues {
		errors = append(errors, validateIssueSchema(cfg, &issue)...)
	}

	return errors
}

func validateIssueSchema(cfg *config.Config, issue *config.Issue) []ValidationError {
	var errors []ValidationError

	if issue.ID == "" {
		errors = append(errors, ValidationError{
			Field:    "id",
			Message:  "issue ID is required",
			Severity: "error",
		})
	}

	effectiveType := cfg.EffectiveIssueType(issue)
	if effectiveType == "" {
		errors = append(errors, ValidationError{
			IssueID:  issue.ID,
			Field:    "issueType",
			Message:  "issue type is required (set per-issue or in defaults)",
			Severity: "error",
		})
	}

	if issue.Summary == "" {
		errors = append(errors, ValidationError{
			IssueID:  issue.ID,
			Field:    "summary",
			Message:  "summary is required",
			Severity: "error",
		})
	}

	if len(issue.Summary) > 255 {
		errors = append(errors, ValidationError{
			IssueID:  issue.ID,
			Field:    "summary",
			Message:  "summary exceeds 255 characters",
			Severity: "warning",
		})
	}

	if effectiveType == "Epic" && issue.EpicName == "" {
		errors = append(errors, ValidationError{
			IssueID:  issue.ID,
			Field:    "epicName",
			Message:  "epicName is required for Epic issue type",
			Severity: "error",
		})
	}

	return errors
}

func validateBusinessLogic(cfg *config.Config) []ValidationError {
	var errors []ValidationError

	// Build ID map
	idMap := make(map[string]bool)
	issueMap := make(map[string]*config.Issue)
	for i, issue := range cfg.Issues {
		if idMap[issue.ID] {
			errors = append(errors, ValidationError{
				IssueID:  issue.ID,
				Field:    "id",
				Message:  "duplicate issue ID",
				Severity: "error",
			})
		}
		idMap[issue.ID] = true
		issueMap[issue.ID] = &cfg.Issues[i]
	}

	// Check allowed types and priorities
	if cfg.Validation != nil {
		for _, issue := range cfg.Issues {
			effectiveType := cfg.EffectiveIssueType(&issue)
			if len(cfg.Validation.AllowedIssueTypes) > 0 && effectiveType != "" {
				if !slices.Contains(cfg.Validation.AllowedIssueTypes, effectiveType) {
					errors = append(errors, ValidationError{
						IssueID:  issue.ID,
						Field:    "issueType",
						Message:  fmt.Sprintf("issue type %q not in allowed list: %v", effectiveType, cfg.Validation.AllowedIssueTypes),
						Severity: "error",
					})
				}
			}

			if issue.Priority != "" && len(cfg.Validation.AllowedPriorities) > 0 {
				if !slices.Contains(cfg.Validation.AllowedPriorities, issue.Priority) {
					errors = append(errors, ValidationError{
						IssueID:  issue.ID,
						Field:    "priority",
						Message:  fmt.Sprintf("priority %q not in allowed list: %v", issue.Priority, cfg.Validation.AllowedPriorities),
						Severity: "error",
					})
				}
			}
		}
	}

	// Validate parent references and parent-child type relationships
	for _, issue := range cfg.Issues {
		if issue.Parent != "" && !idMap[issue.Parent] {
			errors = append(errors, ValidationError{
				IssueID:  issue.ID,
				Field:    "parent",
				Message:  fmt.Sprintf("parent %q not found in issues list", issue.Parent),
				Severity: "error",
			})
		}

		// Validate parent-child type relationships
		if issue.Parent != "" {
			if parent, ok := issueMap[issue.Parent]; ok {
				if err := validateParentChild(*parent, issue); err != nil {
					errors = append(errors, ValidationError{
						IssueID:  issue.ID,
						Field:    "issueType",
						Message:  err.Error(),
						Severity: "warning",
					})
				}
			}
		}
	}

	// Validate dependsOn references
	for _, issue := range cfg.Issues {
		for _, depID := range issue.DependsOn {
			if !idMap[depID] {
				errors = append(errors, ValidationError{
					IssueID:  issue.ID,
					Field:    "dependsOn",
					Message:  fmt.Sprintf("dependency %q not found in issues list", depID),
					Severity: "error",
				})
			}
		}
	}

	// Validate link targets
	for _, issue := range cfg.Issues {
		for _, link := range issue.Links {
			// Allow both internal IDs (defined in this file) and external Jira keys (e.g. POM-1052)
			if !idMap[link.Target] && !isJiraKey(link.Target) {
				errors = append(errors, ValidationError{
					IssueID:  issue.ID,
					Field:    "links",
					Message:  fmt.Sprintf("link target %q not found in issues list", link.Target),
					Severity: "error",
				})
			}

			// Warn about uncommon link types (but don't error; custom types may exist)
			if !isCommonLinkType(link.Type) {
				errors = append(errors, ValidationError{
					IssueID:  issue.ID,
					Field:    "links",
					Message:  fmt.Sprintf("link type %q is uncommon; did you mean: blocks, relates to, duplicates, clones?", link.Type),
					Severity: "warning",
				})
			}
		}
	}

	// Check for circular dependencies
	errors = append(errors, checkCircularDependencies(cfg.Issues)...)

	return errors
}

func validateTemplates(cfg *config.Config) []ValidationError {
	var errors []ValidationError

	if cfg.Defaults.DescriptionTemplate == "" || cfg.Validation == nil {
		return errors
	}

	for _, issue := range cfg.Issues {
		requiredFields := cfg.Validation.RequiredFields[issue.IssueType]
		if len(requiredFields) == 0 {
			continue
		}

		if len(issue.TemplateVars) > 0 {
			for _, field := range requiredFields {
				if field == "summary" || field == "description" {
					continue
				}
				if _, ok := issue.TemplateVars[field]; !ok {
					errors = append(errors, ValidationError{
						IssueID:  issue.ID,
						Field:    "templateVars",
						Message:  fmt.Sprintf("required template variable %q missing for issue type %s", field, issue.IssueType),
						Severity: "error",
					})
				}
			}
		}
	}

	return errors
}

func checkCircularDependencies(issues []config.Issue) []ValidationError {
	var errors []ValidationError

	graph := make(map[string][]string)
	for _, issue := range issues {
		deps := make([]string, 0)
		if issue.Parent != "" {
			deps = append(deps, issue.Parent)
		}
		deps = append(deps, issue.DependsOn...)
		graph[issue.ID] = deps
	}

	visiting := make(map[string]bool)
	visited := make(map[string]bool)

	var visit func(id string, path []string) bool
	visit = func(id string, path []string) bool {
		if visiting[id] {
			cycle := make([]string, len(path)+1)
			copy(cycle, path)
			cycle[len(path)] = id
			errors = append(errors, ValidationError{
				IssueID:  id,
				Field:    "dependsOn",
				Message:  "circular dependency detected: " + strings.Join(cycle, " → "),
				Severity: "error",
			})
			return true
		}

		if visited[id] {
			return false
		}

		visiting[id] = true
		newPath := make([]string, len(path)+1)
		copy(newPath, path)
		newPath[len(path)] = id

		for _, depID := range graph[id] {
			if visit(depID, newPath) {
				return true
			}
		}

		visiting[id] = false
		visited[id] = true
		return false
	}

	for _, issue := range issues {
		if !visited[issue.ID] {
			visit(issue.ID, []string{})
		}
	}

	return errors
}

func printSummary(cfg *config.Config) {
	fmt.Printf("\n📊 Summary:\n")
	fmt.Printf("  - Schema version: %s\n", cfg.SchemaVersion)
	fmt.Printf("  - Project: %s\n", cfg.Defaults.ProjectKey)
	fmt.Printf("  - Issues: %d\n", len(cfg.Issues))

	// Count issue types
	typeCounts := make(map[string]int)
	for _, issue := range cfg.Issues {
		typeCounts[issue.IssueType]++
	}
	for t, count := range typeCounts {
		fmt.Printf("    - %s: %d\n", t, count)
	}
}

// isCommonLinkType checks if a link type is one of the standard Jira link types.
func isCommonLinkType(linkType string) bool {
	commonTypes := []string{
		"blocks", "is blocked by", "blocked by", "blockedby",
		"relates to", "relates",
		"duplicates", "is duplicated by",
		"clones", "is cloned by",
		"depends on", "is depended on by",
	}
	return slices.Contains(commonTypes, strings.ToLower(linkType))
}

// jiraKeyPattern matches Jira issue keys like "POM-1052", "ABC-1", etc.
var jiraKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)

// isJiraKey returns true if the string looks like an existing Jira issue key (e.g. "POM-1052").
func isJiraKey(s string) bool {
	return jiraKeyPattern.MatchString(s)
}

// validateParentChild checks if parent-child issue type combination is valid in Jira.
// Returns error (warning-level) if combination seems invalid.
func validateParentChild(parent, child config.Issue) error {
	// Typical Jira hierarchy: Epic -> Story/Task -> Subtask
	// Subtasks can only be children of Story/Task/Bug, not Epic or other Subtasks

	if strings.ToLower(child.IssueType) == "subtask" {
		validParents := []string{"story", "task", "bug"}
		if !slices.Contains(validParents, strings.ToLower(parent.IssueType)) {
			return fmt.Errorf("subtask can only have parent of type Story/Task/Bug, not %q", parent.IssueType)
		}
	}

	if strings.ToLower(parent.IssueType) == "subtask" {
		return errors.New("subtask cannot be a parent issue")
	}

	return nil
}
