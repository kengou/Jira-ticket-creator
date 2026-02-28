// SPDX-License-Identifier: Apache-2.0
//
// Package apply orchestrates issue creation in Jira.
package apply

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kengou/Jira-ticket-creator/internal/config"
	"github.com/kengou/Jira-ticket-creator/internal/jira"
	"github.com/kengou/Jira-ticket-creator/internal/state"
)

// jiraKeyPattern matches Jira issue keys like "POM-1052", "ABC-1", etc.
var jiraKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)

// Applier creates issues in Jira according to a configuration.
type Applier struct {
	config     *config.Config
	client     *jira.Client
	state      *state.State
	configFile string

	// Options
	verbose         bool
	dryRun          bool
	continueOnError bool

	// Runtime state: internal ID -> Jira key
	createdIssues map[string]string
}

// NewApplier creates a new Applier.
func NewApplier(cfg *config.Config, client *jira.Client, verbose, dryRun bool, configFile string) *Applier {
	continueOnError := false
	if cfg.Options != nil {
		continueOnError = cfg.Options.ContinueOnError
	}

	return &Applier{
		config:          cfg,
		client:          client,
		verbose:         verbose,
		dryRun:          dryRun,
		continueOnError: continueOnError,
		configFile:      configFile,
		createdIssues:   make(map[string]string),
	}
}

// Apply creates all issues in dependency order.
func (a *Applier) Apply() error {
	// Always load state for duplicate detection (unless dry-run).
	// The state file is placed next to the YAML config file so that re-running
	// the same config from any working directory finds the same state.
	if !a.dryRun {
		st, err := state.Load(a.config.Defaults.ProjectKey, a.configFile)
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}
		a.state = st

		if a.state.Count() > 0 {
			fmt.Printf("📂 Loaded state: %d issues already tracked in %s\n", a.state.Count(), a.state.Path())
		}
	}

	// Build dependency order
	ordered, err := BuildDependencyGraph(a.config.Issues)
	if err != nil {
		return fmt.Errorf("build dependency graph: %w", err)
	}

	fmt.Printf("📋 Creating %d issues in dependency order...\n\n", len(ordered))

	// Create issues
	var failedCount, skippedCount int
	for i, issue := range ordered {
		skipped, err := a.createIssue(&issue, i+1, len(ordered))
		if err != nil {
			if a.continueOnError {
				fmt.Printf("❌ Error creating %s: %v (continuing...)\n", issue.ID, err)
				failedCount++
				continue
			}
			return fmt.Errorf("create issue %s: %w", issue.ID, err)
		}
		if skipped {
			skippedCount++
		}
	}

	// Final state save — state is also saved after each issue creation (in
	// createIssue) so that partial runs don't lose progress, but we do a
	// final save here to capture the up-to-date timestamp.
	if !a.dryRun && a.state != nil {
		if err := a.state.Save(); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
		if a.verbose {
			fmt.Printf("\n💾 State saved to: %s\n", a.state.Path())
		}
	}

	// Create issue links
	if err := a.createIssueLinks(); err != nil {
		return fmt.Errorf("create issue links: %w", err)
	}

	// Summary
	createdCount := len(a.createdIssues) - skippedCount
	if skippedCount > 0 && failedCount > 0 {
		fmt.Printf("\n⚠️  Created %d, skipped %d (already exist), failed %d of %d issues\n",
			createdCount, skippedCount, failedCount, len(ordered))
	} else if skippedCount > 0 {
		fmt.Printf("\n✅ Created %d, skipped %d (already exist) of %d issues\n",
			createdCount, skippedCount, len(ordered))
	} else if failedCount > 0 {
		fmt.Printf("\n⚠️  Created %d of %d issues (%d failed)\n",
			createdCount, len(ordered), failedCount)
	} else {
		fmt.Printf("\n✅ Successfully created %d issues!\n", createdCount)
	}
	return nil
}

// createIssue creates a single issue. Returns (skipped, error).
// skipped is true when the issue already exists in state and was not re-created.
func (a *Applier) createIssue(issue *config.Issue, index, total int) (bool, error) {
	fmt.Printf("[%d/%d] %s: %s (%s)\n",
		index, total, issue.ID, truncate(issue.Summary, 50), issue.IssueType)

	// Always check state for duplicates (unless dry-run)
	if a.state != nil {
		if existing, ok := a.state.GetRecord(issue.ID); ok {
			// Internal ID matches — check if summary also matches
			if existing.Summary == issue.Summary {
				fmt.Printf("  ⏭️  Already exists: %s — %s (skipping)\n", existing.JiraKey, existing.Summary)
			} else {
				fmt.Printf("  ⏭️  Already exists: %s (skipping)\n", existing.JiraKey)
				fmt.Printf("      ⚠️  Summary changed: %q -> %q (state not updated)\n",
					existing.Summary, issue.Summary)
			}
			a.createdIssues[issue.ID] = existing.JiraKey
			return true, nil
		}
	}

	// Build fields
	fields, err := a.buildIssueFields(issue)
	if err != nil {
		return false, fmt.Errorf("build fields: %w", err)
	}

	if a.verbose {
		fmt.Printf("  📝 Fields: %+v\n", fields)
	}

	// Dry run
	if a.dryRun {
		fmt.Printf("  🔍 [DRY RUN] Would create issue with %d fields\n", len(fields))
		a.createdIssues[issue.ID] = fmt.Sprintf("DRY-RUN-%s", issue.ID)
		return false, nil
	}

	// Create in Jira
	resp, err := a.client.CreateIssue(&jira.CreateIssueRequest{Fields: fields})
	if err != nil {
		return false, err
	}

	// Record in memory and persist to disk immediately so that a subsequent
	// failure does not cause already-created issues to be duplicated on re-run.
	a.createdIssues[issue.ID] = resp.Key
	if a.state != nil {
		configFileName := filepath.Base(a.configFile)
		a.state.AddIssue(issue.ID, resp.Key, issue.IssueType, issue.Summary, issue.EpicLink, configFileName)
		if err := a.state.Save(); err != nil {
			// Log but don't fail — the issue was already created in Jira.
			fmt.Printf("  ⚠️  Warning: failed to save state: %v\n", err)
		}
	}

	fmt.Printf("  ✅ Created: %s\n", resp.Key)
	return false, nil
}

// buildIssueFields builds the Jira API fields map.
func (a *Applier) buildIssueFields(issue *config.Issue) (map[string]any, error) {
	fields := make(map[string]any)

	// Issue type
	fields["issuetype"] = jira.FormatIssueType(a.config.EffectiveIssueType(issue))

	// Summary
	fields["summary"] = issue.Summary

	// Description
	desc, err := a.renderDescription(issue)
	if err != nil {
		return nil, fmt.Errorf("render description: %w", err)
	}
	if desc != "" {
		fields["description"] = desc
	}

	// Priority
	if priority := a.config.EffectivePriority(issue); priority != "" {
		fields["priority"] = jira.FormatPriority(priority)
	}

	// Assignee
	if assignee := a.config.EffectiveAssignee(issue); assignee != nil {
		fields["assignee"] = jira.FormatUser(*assignee)
	}

	// Reporter
	if reporter := a.config.EffectiveReporter(issue); reporter != "" {
		fields["reporter"] = jira.FormatUser(reporter)
	}

	// Labels
	if labels := a.config.EffectiveLabels(issue); len(labels) > 0 {
		fields["labels"] = labels
	}

	// Components
	if components := a.config.EffectiveComponents(issue); len(components) > 0 {
		compList := make([]any, len(components))
		for i, c := range components {
			compList[i] = jira.FormatComponent(c)
		}
		fields["components"] = compList
	}

	// Fix versions
	if fixVersions := a.config.EffectiveFixVersions(issue); len(fixVersions) > 0 {
		versionList := make([]any, len(fixVersions))
		for i, v := range fixVersions {
			versionList[i] = jira.FormatVersion(v)
		}
		fields["fixVersions"] = versionList
	}

	// Parent (for sub-tasks) or Epic Link (for stories/tasks under epics)
	// On Jira Data Center, the "parent" field only works for sub-task types.
	// When a non-sub-task (Story, Task, Bug) references an Epic as parent,
	// automatically use the epic link custom field instead.
	if issue.Parent != "" {
		parentKey, ok := a.createdIssues[issue.Parent]
		if !ok {
			return nil, fmt.Errorf("parent %q not yet created", issue.Parent)
		}
		if a.isEpic(issue.Parent) {
			// Parent is an Epic → use epic link custom field
			fields[a.getEpicLinkFieldID()] = parentKey
		} else {
			// Parent is a regular issue → use parent field (sub-tasks)
			fields["parent"] = map[string]any{"key": parentKey}
		}
	}

	// Epic name (required for Epic issue type)
	if issue.IssueType == "Epic" && issue.EpicName != "" {
		fields[a.getEpicNameFieldID()] = issue.EpicName
	}

	// Epic link (link to existing epic) — Jira Data Center expects a plain string (the epic key)
	if issue.EpicLink != "" {
		fields[a.getEpicLinkFieldID()] = issue.EpicLink
	}

	// Custom fields
	for k, v := range a.config.EffectiveCustomFields(issue) {
		fields[k] = v
	}

	return jira.BuildIssueFields(a.config.Defaults.ProjectKey, fields), nil
}

// renderDescription renders the description using template if configured.
func (a *Applier) renderDescription(issue *config.Issue) (string, error) {
	// Direct description takes precedence
	if issue.Description != "" {
		return issue.Description, nil
	}

	// No template configured
	if a.config.Defaults.DescriptionTemplate == "" {
		return "", nil
	}

	// No template vars provided
	if len(issue.TemplateVars) == 0 {
		return "", nil
	}

	// Build variables map
	vars := make(map[string]string)
	vars["summary"] = issue.Summary
	for k, v := range issue.TemplateVars {
		vars[k] = v
	}

	// Simple placeholder replacement: {var} -> value
	result := a.config.Defaults.DescriptionTemplate
	for key, value := range vars {
		placeholder := "{" + key + "}"
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result, nil
}

// createIssueLinks creates all issue links defined in the configuration.
func (a *Applier) createIssueLinks() error {
	// Collect all issues that have links
	var hasLinks bool
	for _, issue := range a.config.Issues {
		if len(issue.Links) > 0 {
			hasLinks = true
			break
		}
	}
	if !hasLinks {
		return nil
	}

	// Fetch available link types from Jira (unless dry-run)
	var linkTypes []jira.IssueLinkTypeInfo
	if !a.dryRun {
		var err error
		linkTypes, err = a.client.FetchIssueLinkTypes()
		if err != nil {
			return fmt.Errorf("fetch issue link types: %w", err)
		}
	}

	var linkCount int

	for _, issue := range a.config.Issues {
		if len(issue.Links) == 0 {
			continue
		}

		sourceKey, ok := a.createdIssues[issue.ID]
		if !ok {
			continue // Issue wasn't created
		}

		for _, link := range issue.Links {
			targetKey, ok := a.createdIssues[link.Target]
			if !ok {
				// If the target looks like an existing Jira key (e.g. POM-1052),
				// use it directly — this allows linking to issues that already exist in Jira.
				if jiraKeyPattern.MatchString(link.Target) {
					targetKey = link.Target
				} else {
					fmt.Printf("⚠️  Cannot link %s to %s: target not created\n", issue.ID, link.Target)
					continue
				}
			}

			if a.dryRun {
				fmt.Printf("  🔍 [DRY RUN] Would link %s -[%s]-> %s\n", sourceKey, link.Type, targetKey)
				linkCount++
				continue
			}

			resolved, err := resolveLinkType(link.Type, linkTypes)
			if err != nil {
				fmt.Printf("⚠️  Failed to link %s -> %s: %v\n", sourceKey, targetKey, err)
				continue
			}

			// Determine direction based on how the user's text matched:
			//   - matched outward description → source is the outward issue (source "blocks" target)
			//   - matched inward description  → source is the inward issue  (source "is blocked by" target)
			//   - matched name                → default: source is outward
			var inward, outward string
			switch resolved.matchedVia {
			case matchedViaInward:
				inward = sourceKey
				outward = targetKey
			default: // matchedViaOutward, matchedViaName
				inward = targetKey
				outward = sourceKey
			}

			req := &jira.IssueLinkRequest{
				Type:         jira.IssueLinkType{Name: resolved.name},
				InwardIssue:  jira.IssueRef{Key: inward},
				OutwardIssue: jira.IssueRef{Key: outward},
			}

			if link.Comment != "" {
				req.Comment = &jira.LinkComment{Body: link.Comment}
			}

			if err := a.client.CreateIssueLink(req); err != nil {
				fmt.Printf("⚠️  Failed to link %s -> %s: %v\n", sourceKey, targetKey, err)
				continue
			}

			linkCount++
			if a.verbose {
				fmt.Printf("  🔗 Linked: %s -[%s]-> %s\n", sourceKey, link.Type, targetKey)
			}
		}
	}

	if linkCount > 0 {
		fmt.Printf("\n🔗 Created %d issue links\n", linkCount)
	}

	return nil
}

// matchType indicates how a user's link type text matched a Jira link type.
type matchType int

const (
	matchedViaName    matchType = iota // matched the link type's Name
	matchedViaInward                   // matched the link type's Inward description
	matchedViaOutward                  // matched the link type's Outward description
)

// resolvedLinkType holds the resolved Jira link type name and match info.
type resolvedLinkType struct {
	name       string    // the official Jira link type Name to send in the API request
	matchedVia matchType // how the user's text matched
}

// resolveLinkType matches a user-provided link type string against the available
// Jira link types. It checks (case-insensitive) against Outward, Inward, and Name
// in that order — outward/inward are checked first because they carry directional
// meaning (e.g. "blocks" vs "is blocked by"), while the Name is a fallback.
func resolveLinkType(userType string, available []jira.IssueLinkTypeInfo) (resolvedLinkType, error) {
	lower := strings.ToLower(userType)

	for _, lt := range available {
		if strings.EqualFold(lt.Outward, lower) {
			return resolvedLinkType{name: lt.Name, matchedVia: matchedViaOutward}, nil
		}
		if strings.EqualFold(lt.Inward, lower) {
			return resolvedLinkType{name: lt.Name, matchedVia: matchedViaInward}, nil
		}
	}

	// Fall back to matching by Name (direction defaults to outward)
	for _, lt := range available {
		if strings.EqualFold(lt.Name, lower) {
			return resolvedLinkType{name: lt.Name, matchedVia: matchedViaName}, nil
		}
	}

	// Build a helpful error listing available types
	var names []string
	for _, lt := range available {
		names = append(names, fmt.Sprintf("%q (inward: %q, outward: %q)", lt.Name, lt.Inward, lt.Outward))
	}
	return resolvedLinkType{}, fmt.Errorf("unknown link type %q; available types:\n    %s",
		userType, strings.Join(names, "\n    "))
}

// PrintPlan prints what would be created without making API calls.
func PrintPlan(cfg *config.Config) error {
	fmt.Println("📋 Issue Creation Plan")
	fmt.Println("======================")
	fmt.Println()

	ordered, err := BuildDependencyGraph(cfg.Issues)
	if err != nil {
		return fmt.Errorf("build dependency graph: %w", err)
	}

	fmt.Printf("Will create %d issues in the following order:\n\n", len(ordered))

	for i, issue := range ordered {
		fmt.Printf("%d. [%s] %s: %s\n", i+1, issue.IssueType, issue.ID, issue.Summary)

		if issue.Parent != "" {
			fmt.Printf("   └─ Parent: %s\n", issue.Parent)
		}

		if issue.EpicLink != "" {
			fmt.Printf("   └─ Epic Link: %s\n", issue.EpicLink)
		}

		if len(issue.DependsOn) > 0 {
			fmt.Printf("   └─ Depends on: %v\n", issue.DependsOn)
		}

		if len(issue.Links) > 0 {
			fmt.Printf("   └─ Links: ")
			for j, link := range issue.Links {
				if j > 0 {
					fmt.Printf(", ")
				}
				fmt.Printf("%s %s", link.Type, link.Target)
			}
			fmt.Println()
		}

		fmt.Println()
	}

	return nil
}

// truncate truncates a string to maxLen runes, adding "..." if needed.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

// getEpicNameFieldID returns the field ID for epic name (configurable).
func (a *Applier) getEpicNameFieldID() string {
	if a.config.Defaults.EpicNameField != "" {
		return a.config.Defaults.EpicNameField
	}
	// Data Center default
	return "customfield_10011"
}

// getEpicLinkFieldID returns the field ID for epic link (configurable).
func (a *Applier) getEpicLinkFieldID() string {
	if a.config.Defaults.EpicLinkField != "" {
		return a.config.Defaults.EpicLinkField
	}
	// Data Center default
	return "customfield_10009"
}

// isEpic returns true if the given internal ID refers to an Epic in the config.
func (a *Applier) isEpic(internalID string) bool {
	for i := range a.config.Issues {
		if a.config.Issues[i].ID == internalID {
			return strings.EqualFold(a.config.EffectiveIssueType(&a.config.Issues[i]), "Epic")
		}
	}
	return false
}

// BuildDependencyGraph builds a dependency graph and returns issues in creation order.
func BuildDependencyGraph(issues []config.Issue) ([]config.Issue, error) {
	issueMap := make(map[string]*config.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	// Build dependency graph: issue -> dependencies
	dependencies := make(map[string][]string)
	for _, issue := range issues {
		deps := make([]string, 0)
		if issue.Parent != "" {
			deps = append(deps, issue.Parent)
		}
		deps = append(deps, issue.DependsOn...)
		dependencies[issue.ID] = deps
	}

	// Topological sort using DFS
	visited := make(map[string]bool)
	temp := make(map[string]bool)
	var result []string

	var visit func(id string) error
	visit = func(id string) error {
		if temp[id] {
			return fmt.Errorf("circular dependency detected involving %q", id)
		}
		if visited[id] {
			return nil
		}

		temp[id] = true

		for _, dep := range dependencies[id] {
			if err := visit(dep); err != nil {
				return err
			}
		}

		temp[id] = false
		visited[id] = true
		result = append(result, id)
		return nil
	}

	for _, issue := range issues {
		if !visited[issue.ID] {
			if err := visit(issue.ID); err != nil {
				return nil, err
			}
		}
	}

	// Build ordered result
	ordered := make([]config.Issue, 0, len(result))
	for _, id := range result {
		ordered = append(ordered, *issueMap[id])
	}

	return ordered, nil
}
