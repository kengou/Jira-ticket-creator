// SPDX-License-Identifier: Apache-2.0
//
// Package config provides configuration types and loading for jira-ai-creator.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the root configuration file.
type Config struct {
	SchemaVersion string      `yaml:"schemaVersion"`
	Defaults      Defaults    `yaml:"defaults"`
	Validation    *Validation `yaml:"validation,omitempty"`
	Options       *Options    `yaml:"options,omitempty"`
	Issues        []Issue     `yaml:"issues"`
}

// Defaults contains global default values applied to all issues.
type Defaults struct {
	ProjectKey          string         `yaml:"projectKey"`
	IssueType           string         `yaml:"issueType,omitempty"`
	Priority            string         `yaml:"priority,omitempty"`
	Reporter            string         `yaml:"reporter,omitempty"`
	Assignee            *string        `yaml:"assignee,omitempty"`
	Labels              []string       `yaml:"labels,omitempty"`
	Components          []string       `yaml:"components,omitempty"`
	FixVersions         []string       `yaml:"fixVersions,omitempty"`
	CustomFields        map[string]any `yaml:"customFields,omitempty"`
	DescriptionTemplate string         `yaml:"descriptionTemplate,omitempty"`
	// Jira field IDs for epic fields (customizable; defaults set in config)
	EpicNameField string `yaml:"epicNameField,omitempty"`
	EpicLinkField string `yaml:"epicLinkField,omitempty"`
}

// Validation defines validation rules for the configuration.
type Validation struct {
	StrictMode        bool                `yaml:"strictMode,omitempty"`
	AllowedIssueTypes []string            `yaml:"allowedIssueTypes,omitempty"`
	AllowedPriorities []string            `yaml:"allowedPriorities,omitempty"`
	RequiredFields    map[string][]string `yaml:"requiredFields,omitempty"`
}

// Options controls execution behavior.
type Options struct {
	ContinueOnError    bool  `yaml:"continueOnError,omitempty"`
	IdempotencyEnabled *bool `yaml:"idempotencyEnabled,omitempty"`
}

// IsIdempotencyEnabled returns whether idempotency is enabled (defaults to true).
func (o *Options) IsIdempotencyEnabled() bool {
	if o == nil || o.IdempotencyEnabled == nil {
		return true
	}
	return *o.IdempotencyEnabled
}

// Issue represents a single Jira issue to create.
type Issue struct {
	// Required fields
	ID        string `yaml:"id"`
	IssueType string `yaml:"issueType"`
	Summary   string `yaml:"summary"`

	// Optional fields
	Description string `yaml:"description,omitempty"`
	Parent      string `yaml:"parent,omitempty"`
	EpicName    string `yaml:"epicName,omitempty"`
	EpicLink    string `yaml:"epicLink,omitempty"`
	Priority    string `yaml:"priority,omitempty"`
	Assignee    string `yaml:"assignee,omitempty"`
	Reporter    string `yaml:"reporter,omitempty"`

	// Collections
	Labels       []string       `yaml:"labels,omitempty"`
	Components   []string       `yaml:"components,omitempty"`
	FixVersions  []string       `yaml:"fixVersions,omitempty"`
	CustomFields map[string]any `yaml:"customFields,omitempty"`

	// Relationships
	DependsOn []string    `yaml:"dependsOn,omitempty"`
	Links     []IssueLink `yaml:"links,omitempty"`

	// Templating
	TemplateVars map[string]string `yaml:"templateVars,omitempty"`

	// Arbitrary metadata (not sent to Jira)
	Metadata map[string]any `yaml:"metadata,omitempty"`
}

// IssueLink represents a link between two issues.
type IssueLink struct {
	Type    string `yaml:"type"`
	Target  string `yaml:"target"`
	Comment string `yaml:"comment,omitempty"`
}

// LoadConfig loads and parses a YAML configuration file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML in %q: %w", path, err)
	}

	// Apply defaults for options
	config.applyDefaults()

	return &config, nil
}

// applyDefaults sets default values for optional fields.
func (c *Config) applyDefaults() {
	if c.Options == nil {
		c.Options = &Options{}
	}
	if c.Validation == nil {
		c.Validation = &Validation{}
	}
}

// EffectiveIssueType returns the issue type with defaults applied.
func (c *Config) EffectiveIssueType(issue *Issue) string {
	if issue.IssueType != "" {
		return issue.IssueType
	}
	return c.Defaults.IssueType
}

// EffectivePriority returns the priority with defaults applied.
// Returns "" when no priority is explicitly configured, so the Jira
// server default is used and the field is omitted from the create request
// (avoids 400 errors when the priority field is not on the create screen).
func (c *Config) EffectivePriority(issue *Issue) string {
	if issue.Priority != "" {
		return issue.Priority
	}
	return c.Defaults.Priority
}

// EffectiveAssignee returns the assignee with defaults applied.
func (c *Config) EffectiveAssignee(issue *Issue) *string {
	if issue.Assignee != "" {
		return &issue.Assignee
	}
	return c.Defaults.Assignee
}

// EffectiveReporter returns the reporter with defaults applied.
func (c *Config) EffectiveReporter(issue *Issue) string {
	if issue.Reporter != "" {
		return issue.Reporter
	}
	return c.Defaults.Reporter
}

// EffectiveLabels returns labels with defaults merged.
func (c *Config) EffectiveLabels(issue *Issue) []string {
	seen := make(map[string]bool)
	var result []string

	// Add defaults first
	for _, label := range c.Defaults.Labels {
		if !seen[label] {
			seen[label] = true
			result = append(result, label)
		}
	}

	// Add issue-specific labels
	for _, label := range issue.Labels {
		if !seen[label] {
			seen[label] = true
			result = append(result, label)
		}
	}

	return result
}

// EffectiveComponents returns components with defaults applied.
func (c *Config) EffectiveComponents(issue *Issue) []string {
	if len(issue.Components) > 0 {
		return issue.Components
	}
	return c.Defaults.Components
}

// EffectiveFixVersions returns fix versions with defaults applied.
func (c *Config) EffectiveFixVersions(issue *Issue) []string {
	if len(issue.FixVersions) > 0 {
		return issue.FixVersions
	}
	return c.Defaults.FixVersions
}

// EffectiveCustomFields returns custom fields with defaults merged.
func (c *Config) EffectiveCustomFields(issue *Issue) map[string]any {
	merged := make(map[string]any)

	// Copy defaults
	for k, v := range c.Defaults.CustomFields {
		merged[k] = v
	}

	// Override with issue-specific
	for k, v := range issue.CustomFields {
		merged[k] = v
	}

	return merged
}
