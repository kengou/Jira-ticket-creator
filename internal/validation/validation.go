// SPDX-License-Identifier: Apache-2.0
//
// Package validation provides configuration validation for jira-ai-creator.
package validation

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/kengou/jira-ticket-creator/internal/config"
)

// jiraKeyRE matches a valid Jira issue key (e.g. "DEMO-2679").
// Duplicated from internal/jira to break the import cycle between validation and jira.
var jiraKeyRE = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)

// commonLinkTypes is a package-level set of standard Jira link type names (lower-cased).
// Using a map avoids allocating a new slice on every IsCommonLinkType call.
var commonLinkTypes = map[string]bool{
	"blocks": true, "is blocked by": true, "blocked by": true, "blockedby": true,
	"relates to": true, "relates": true,
	"duplicates": true, "is duplicated by": true,
	"clones": true, "is cloned by": true,
	"depends on": true, "is depended on by": true,
}

// Severity is a type alias for string that classifies the impact of a finding.
// Using a type alias (not a named type) preserves backward compatibility with
// code that compares Severity values to plain string literals.
type Severity = string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Error represents a single validation finding.
type Error struct {
	IssueID  string
	Field    string
	Message  string
	Severity Severity
}

func (e Error) String() string {
	if e.IssueID != "" {
		return fmt.Sprintf("[%s] Issue %s: %s - %s", e.Severity, e.IssueID, e.Field, e.Message)
	}
	return fmt.Sprintf("[%s] %s - %s", e.Severity, e.Field, e.Message)
}

// ValidateConfig runs all validation checks against cfg and returns all findings.
func ValidateConfig(cfg *config.Config) []Error {
	var errs []Error
	errs = append(errs, ValidateRequiredFields(cfg)...)
	errs = append(errs, ValidateBusinessLogic(cfg)...)
	errs = append(errs, ValidateTemplates(cfg)...)
	return errs
}

// ValidateRequiredFields checks structural correctness of the configuration,
// ensuring required fields are present and have valid values.
func ValidateRequiredFields(cfg *config.Config) []Error {
	var errs []Error

	if cfg.SchemaVersion == "" {
		errs = append(errs, Error{
			Field:    "schemaVersion",
			Message:  "schema version is required",
			Severity: SeverityError,
		})
	} else if cfg.SchemaVersion != "1.0" {
		errs = append(errs, Error{
			Field:    "schemaVersion",
			Message:  "unsupported schema version: " + cfg.SchemaVersion,
			Severity: SeverityWarning,
		})
	}

	if cfg.Defaults.ProjectKey == "" {
		errs = append(errs, Error{
			Field:    "defaults.projectKey",
			Message:  "project key is required",
			Severity: SeverityError,
		})
	}

	if len(cfg.Issues) == 0 {
		errs = append(errs, Error{
			Field:    "issues",
			Message:  "at least one issue is required",
			Severity: SeverityError,
		})
	}

	for _, issue := range cfg.Issues {
		errs = append(errs, validateIssueSchema(cfg, &issue)...)
	}

	return errs
}

func validateIssueSchema(cfg *config.Config, issue *config.Issue) []Error {
	var errs []Error

	if issue.ID == "" {
		errs = append(errs, Error{
			Field:    "id",
			Message:  "issue ID is required",
			Severity: SeverityError,
		})
	}

	effectiveType := cfg.EffectiveIssueType(issue)
	if effectiveType == "" {
		errs = append(errs, Error{
			IssueID:  issue.ID,
			Field:    "issueType",
			Message:  "issue type is required (set per-issue or in defaults)",
			Severity: SeverityError,
		})
	}

	if issue.Summary == "" {
		errs = append(errs, Error{
			IssueID:  issue.ID,
			Field:    "summary",
			Message:  "summary is required",
			Severity: SeverityError,
		})
	}

	if len(issue.Summary) > 255 {
		errs = append(errs, Error{
			IssueID:  issue.ID,
			Field:    "summary",
			Message:  "summary exceeds 255 characters",
			Severity: SeverityWarning,
		})
	}

	if strings.EqualFold(effectiveType, "Epic") && issue.EpicName == "" {
		errs = append(errs, Error{
			IssueID:  issue.ID,
			Field:    "epicName",
			Message:  "epicName is required for Epic issue type",
			Severity: SeverityError,
		})
	}

	return errs
}

// ValidateBusinessLogic checks cross-issue constraints.
func ValidateBusinessLogic(cfg *config.Config) []Error {
	var errs []Error

	// Build ID map
	idMap := make(map[string]bool)
	issueMap := make(map[string]*config.Issue)
	for i, issue := range cfg.Issues {
		if idMap[issue.ID] {
			errs = append(errs, Error{
				IssueID:  issue.ID,
				Field:    "id",
				Message:  "duplicate issue ID",
				Severity: SeverityError,
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
					errs = append(errs, Error{
						IssueID:  issue.ID,
						Field:    "issueType",
						Message:  fmt.Sprintf("issue type %q not in allowed list: %v", effectiveType, cfg.Validation.AllowedIssueTypes),
						Severity: SeverityError,
					})
				}
			}

			if issue.Priority != "" && len(cfg.Validation.AllowedPriorities) > 0 {
				if !slices.Contains(cfg.Validation.AllowedPriorities, issue.Priority) {
					errs = append(errs, Error{
						IssueID:  issue.ID,
						Field:    "priority",
						Message:  fmt.Sprintf("priority %q not in allowed list: %v", issue.Priority, cfg.Validation.AllowedPriorities),
						Severity: SeverityError,
					})
				}
			}
		}
	}

	// Validate parent references and parent-child type relationships
	for _, issue := range cfg.Issues {
		if issue.Parent != "" && !idMap[issue.Parent] {
			errs = append(errs, Error{
				IssueID:  issue.ID,
				Field:    "parent",
				Message:  fmt.Sprintf("parent %q not found in issues list", issue.Parent),
				Severity: SeverityError,
			})
		}

		if issue.Parent != "" {
			if parent, ok := issueMap[issue.Parent]; ok {
				if err := ValidateParentChild(cfg.EffectiveIssueType(parent), cfg.EffectiveIssueType(&issue)); err != nil {
					errs = append(errs, Error{
						IssueID:  issue.ID,
						Field:    "issueType",
						Message:  err.Error(),
						Severity: SeverityWarning,
					})
				}

				// Warn if both parent (epic) and epicLink are set — epicLink will silently win
				parentType := cfg.EffectiveIssueType(parent)
				if strings.EqualFold(parentType, "Epic") && issue.EpicLink != "" {
					errs = append(errs, Error{
						IssueID:  issue.ID,
						Field:    "epicLink",
						Message:  fmt.Sprintf("both parent %q (Epic) and epicLink %q are set; epicLink will override the parent epic link", issue.Parent, issue.EpicLink),
						Severity: SeverityWarning,
					})
				}
			}
		}
	}

	// Validate dependsOn references
	for _, issue := range cfg.Issues {
		for _, depID := range issue.DependsOn {
			if !idMap[depID] {
				errs = append(errs, Error{
					IssueID:  issue.ID,
					Field:    "dependsOn",
					Message:  fmt.Sprintf("dependency %q not found in issues list", depID),
					Severity: SeverityError,
				})
			}
		}
	}

	// Validate link targets
	for _, issue := range cfg.Issues {
		for _, link := range issue.Links {
			// Allow both internal IDs (defined in this file) and external Jira keys (e.g. DEMO-2679)
			if !idMap[link.Target] && !jiraKeyRE.MatchString(link.Target) {
				errs = append(errs, Error{
					IssueID:  issue.ID,
					Field:    "links",
					Message:  fmt.Sprintf("link target %q not found in issues list", link.Target),
					Severity: SeverityError,
				})
			}

			// Warn about uncommon link types (but don't error; custom types may exist)
			if !IsCommonLinkType(link.Type) {
				errs = append(errs, Error{
					IssueID:  issue.ID,
					Field:    "links",
					Message:  fmt.Sprintf("link type %q is uncommon; did you mean: blocks, relates to, duplicates, clones?", link.Type),
					Severity: SeverityWarning,
				})
			}
		}
	}

	// Check for circular dependencies using DFS.
	// Note: internal/apply.BuildDependencyGraph also detects cycles via topological
	// sort. The duplication is intentional: the validate command must check cycles
	// independently of the apply path so users get errors before any API calls.
	errs = append(errs, CheckCircularDependencies(cfg.Issues)...)

	return errs
}

// ValidateTemplates checks that required template variables are present.
func ValidateTemplates(cfg *config.Config) []Error {
	var errs []Error

	if cfg.Defaults.DescriptionTemplate == "" || cfg.Validation == nil {
		return errs
	}

	for _, issue := range cfg.Issues {
		effectiveType := cfg.EffectiveIssueType(&issue)
		requiredFields := cfg.Validation.RequiredFields[effectiveType]
		if len(requiredFields) == 0 {
			continue
		}

		if len(issue.TemplateVars) > 0 {
			for _, field := range requiredFields {
				if field == "summary" || field == "description" {
					continue
				}
				if _, ok := issue.TemplateVars[field]; !ok {
					errs = append(errs, Error{
						IssueID:  issue.ID,
						Field:    "templateVars",
						Message:  fmt.Sprintf("required template variable %q missing for issue type %s", field, effectiveType),
						Severity: SeverityError,
					})
				}
			}
		}
	}

	return errs
}

// CheckCircularDependencies detects circular dependencies and parent cycles.
func CheckCircularDependencies(issues []config.Issue) []Error {
	var errs []Error

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
			errs = append(errs, Error{
				IssueID:  id,
				Field:    "dependsOn",
				Message:  "circular dependency detected: " + strings.Join(cycle, " → "),
				Severity: SeverityError,
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

	return errs
}

// IsCommonLinkType reports whether linkType is a standard Jira link type name.
func IsCommonLinkType(linkType string) bool {
	return commonLinkTypes[strings.ToLower(linkType)]
}

// ValidateParentChild checks if a parent-child issue type combination is valid.
// Returns an error (treated as a warning by callers) if the combination seems invalid.
func ValidateParentChild(parentType, childType string) error {
	// Typical Jira hierarchy: Epic -> Story/Task -> Subtask
	// Subtasks can only be children of Story/Task/Bug, not Epic or other Subtasks.

	if strings.EqualFold(childType, "subtask") {
		validParents := []string{"story", "task", "bug"}
		if !slices.Contains(validParents, strings.ToLower(parentType)) {
			return fmt.Errorf("subtask can only have parent of type Story/Task/Bug, not %q", parentType)
		}
	}

	if strings.EqualFold(parentType, "subtask") {
		return errors.New("subtask cannot be a parent issue")
	}

	return nil
}
