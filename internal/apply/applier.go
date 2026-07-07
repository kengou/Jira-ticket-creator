// SPDX-License-Identifier: Apache-2.0
//
// Package apply orchestrates issue creation in Jira.
package apply

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kengou/jira-ticket-creator/internal/config"
	"github.com/kengou/jira-ticket-creator/internal/jira"
	"github.com/kengou/jira-ticket-creator/internal/state"
	"github.com/kengou/jira-ticket-creator/internal/ui"
)

// JiraClient is the apply package's broad "role" interface: it enumerates every
// Jira API method the Applier needs, so a single dependency covers issue
// creation, linking, link-type discovery, and user search. This wide shape is
// intentional here because the Applier genuinely exercises all of it. Elsewhere,
// prefer narrow consumer-local interfaces for single-purpose consumers (for
// example cmd's preflightClient), which pin exactly the one or two methods that
// caller uses and keep test doubles minimal.
type JiraClient interface {
	CreateIssue(ctx context.Context, req *jira.CreateIssueRequest) (*jira.CreateIssueResponse, error)
	CreateIssueLink(ctx context.Context, req *jira.IssueLinkRequest) error
	FetchIssueLinkTypes(ctx context.Context) ([]jira.IssueLinkTypeInfo, error)
	SearchUsers(ctx context.Context, query string) ([]jira.User, error)
}

// Applier creates issues in Jira according to a configuration.
type Applier struct {
	config     *config.Config
	client     JiraClient
	state      *state.State
	configFile string

	// Options
	mode            jira.Mode
	enc             fieldEncoder // platform field-value encoder, selected from mode
	verbose         bool
	dryRun          bool
	continueOnError bool

	// degradedIssues collects internal IDs whose markup degraded during ADF
	// conversion (Cloud mode only), for verbose reporting.
	degradedIssues []string

	// droppedEpicNames collects internal IDs whose epicName was dropped on Cloud
	// (Cloud epics use the summary as their name), for verbose reporting.
	droppedEpicNames []string

	// userCache maps a resolved user reference (email) to its Cloud account ID,
	// so each distinct email is looked up at most once per apply run (Cloud only).
	userCache map[string]string

	// Runtime state: internal ID -> Jira key
	createdIssues map[string]string
	// epicIDs is a pre-built set of internal IDs whose effective type is "Epic".
	epicIDs map[string]bool
}

// NewApplier creates a new Applier. mode is the resolved Jira platform mode; the
// applier derives cloud-ness from it and selects the matching field encoder once.
func NewApplier(cfg *config.Config, client JiraClient, mode jira.Mode, verbose, dryRun bool, configFile string) *Applier {
	continueOnError := false
	if cfg.Options != nil {
		continueOnError = cfg.Options.ContinueOnError
	}

	// Pre-build epic ID set for O(1) lookup in buildIssueFields.
	epicIDs := make(map[string]bool, len(cfg.Issues))
	for i := range cfg.Issues {
		if strings.EqualFold(cfg.EffectiveIssueType(&cfg.Issues[i]), "Epic") {
			epicIDs[cfg.Issues[i].ID] = true
		}
	}

	return &Applier{
		config:          cfg,
		client:          client,
		mode:            mode,
		enc:             newFieldEncoder(mode),
		verbose:         verbose,
		dryRun:          dryRun,
		continueOnError: continueOnError,
		configFile:      configFile,
		userCache:       make(map[string]string),
		createdIssues:   make(map[string]string, len(cfg.Issues)),
		epicIDs:         epicIDs,
	}
}

// Apply creates all issues in dependency order.
func (a *Applier) Apply(ctx context.Context) error {
	// Always load state for duplicate detection (unless dry-run).
	// The state file is placed in the current working directory so that
	// re-running from the project root always uses the same state file.
	if !a.dryRun {
		st, err := state.Load(a.config.Defaults.ProjectKey)
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
		skipped, err := a.createIssue(ctx, &issue, i+1, len(ordered))
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

	// Name issues whose markup degraded during ADF conversion (Cloud mode).
	if a.verbose && len(a.degradedIssues) > 0 {
		fmt.Fprintf(os.Stderr, "\n%s ADF conversion degraded markup for %d issue(s): %s\n",
			ui.Emoji("⚠️", "[WARN]"), len(a.degradedIssues), strings.Join(a.degradedIssues, ", "))
	}

	// Note epicName values dropped on Cloud (Cloud epics use the summary as name).
	if a.verbose && len(a.droppedEpicNames) > 0 {
		fmt.Fprintf(os.Stderr, "\n%s epicName is unused on Jira Cloud; dropped for %d issue(s): %s\n",
			ui.Emoji("ℹ️", "[INFO]"), len(a.droppedEpicNames), strings.Join(a.droppedEpicNames, ", "))
	}

	// Create issue links
	if err := a.createIssueLinks(ctx); err != nil {
		return fmt.Errorf("create issue links: %w", err)
	}

	// Summary (suppressed in dry-run — the caller prints its own dry-run message)
	if !a.dryRun {
		createdCount := len(a.createdIssues) - skippedCount
		switch {
		case skippedCount > 0 && failedCount > 0:
			fmt.Printf("\n⚠️  Created %d, skipped %d (already exist), failed %d of %d issues\n",
				createdCount, skippedCount, failedCount, len(ordered))
		case skippedCount > 0:
			fmt.Printf("\n✅ Created %d, skipped %d (already exist) of %d issues\n",
				createdCount, skippedCount, len(ordered))
		case failedCount > 0:
			fmt.Printf("\n⚠️  Created %d of %d issues (%d failed)\n",
				createdCount, len(ordered), failedCount)
		default:
			fmt.Printf("\n✅ Successfully created %d issues!\n", createdCount)
		}
	}
	return nil
}

// createIssue creates a single issue. Returns (skipped, error).
// skipped is true when the issue already exists in state and was not re-created.
func (a *Applier) createIssue(ctx context.Context, issue *config.Issue, index, total int) (bool, error) {
	fmt.Printf("[%d/%d] %s: %s (%s)\n",
		index, total, issue.ID, truncate(issue.Summary, 50), issue.IssueType)

	// Always check state for duplicates (unless dry-run)
	if a.state != nil {
		if existing, ok := a.state.GetRecord(issue.ID); ok {
			// Internal ID matches — check if summary also matches
			if existing.Summary != issue.Summary {
				fmt.Printf("  ⏭️  Already exists: %s — skipping creation, updating local state\n", existing.JiraKey)
				fmt.Printf("      summary: %q -> %q\n", existing.Summary, issue.Summary)
				configFileName := filepath.Base(a.configFile)
				a.state.AddIssue(issue.ID, existing.JiraKey, issue.IssueType, issue.Summary,
					existing.EpicLink, configFileName)
				if err := a.state.Save(); err != nil {
					fmt.Printf("      ⚠️  Warning: failed to update state: %v\n", err)
				}
			} else {
				fmt.Printf("  ⏭️  Already exists: %s — %s (skipping)\n", existing.JiraKey, existing.Summary)
			}
			a.createdIssues[issue.ID] = existing.JiraKey
			return true, nil
		}
	}

	// Build fields
	fields, err := a.buildIssueFields(ctx, issue)
	if err != nil {
		return false, fmt.Errorf("build fields: %w", err)
	}

	if a.verbose {
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		fmt.Printf("  📝 Fields (%d): %v\n", len(fields), keys)
	}

	// Dry run
	if a.dryRun {
		fmt.Printf("  🔍 [DRY RUN] Would create issue with %d fields\n", len(fields))
		a.createdIssues[issue.ID] = "DRY-RUN-" + issue.ID
		return false, nil
	}

	// Create in Jira
	resp, err := a.client.CreateIssue(ctx, &jira.CreateIssueRequest{Fields: fields})
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

// buildIssueFields builds the Jira API fields map. It takes ctx because Cloud
// user resolution (assignee/reporter) may perform user-search network calls.
func (a *Applier) buildIssueFields(ctx context.Context, issue *config.Issue) (map[string]any, error) {
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
		// The encoder shapes the value per platform: Cloud (API v3) emits an ADF
		// document (recording degraded markup for verbose output); Data Center
		// (API v2) passes the raw wiki-markup string through unchanged.
		value, degraded := a.encoder().description(desc)
		fields["description"] = value
		if degraded {
			a.degradedIssues = append(a.degradedIssues, issue.ID)
		}
	}

	// Priority
	if priority := a.config.EffectivePriority(issue); priority != "" {
		fields["priority"] = jira.FormatPriority(priority)
	}

	// Assignee — resolved per platform (Cloud: email→accountId; DC: {"name":...}).
	if assignee := a.config.EffectiveAssignee(issue); assignee != nil {
		resolved, err := a.resolveUserField(ctx, *assignee)
		if err != nil {
			return nil, fmt.Errorf("resolve assignee: %w", err)
		}
		fields["assignee"] = resolved
	}

	// Reporter — resolved per platform. A Cloud project-permission rejection of
	// the reporter field surfaces later as the raw *jira.APIError from CreateIssue
	// (no automatic retry-without-reporter).
	if reporter := a.config.EffectiveReporter(issue); reporter != "" {
		resolved, err := a.resolveUserField(ctx, reporter)
		if err != nil {
			return nil, fmt.Errorf("resolve reporter: %w", err)
		}
		fields["reporter"] = resolved
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

	// Parent / Epic Link translation.
	// Data Center: the "parent" field only works for sub-task types, so a
	// non-sub-task referencing an Epic parent is routed through the epic-link
	// custom field, and epicLink/epicName go to their custom field IDs.
	// Cloud: the unified "parent" field works uniformly, so epic parents and
	// epicLink both emit parent:{key:...}, and epicName is dropped entirely
	// (Cloud epics use the summary as their name).
	if issue.Parent != "" {
		parentKey, ok := a.createdIssues[issue.Parent]
		if !ok {
			return nil, fmt.Errorf("issue %q: parent %q not in dependency order or not yet created",
				issue.ID, issue.Parent)
		}
		if a.epicIDs[issue.Parent] {
			// An epic parent: the encoder decides the shape — Cloud uses the
			// unified parent field, Data Center the epic-link custom field.
			a.encoder().epicParent(fields, parentKey, a.getEpicLinkFieldID())
		} else {
			// Regular (non-epic) parent → parent field (sub-tasks) on both platforms.
			fields["parent"] = map[string]any{"key": parentKey}
		}
	}

	// Epic name (required for Epic issue type on Data Center; dropped on Cloud,
	// where epics use the summary as their name). The encoder writes the field
	// (Data Center) or reports the drop, which we record for verbose output.
	if strings.EqualFold(a.config.EffectiveIssueType(issue), "Epic") && issue.EpicName != "" {
		if dropped := a.encoder().epicName(fields, issue.EpicName, a.getEpicNameFieldID()); dropped {
			a.droppedEpicNames = append(a.droppedEpicNames, issue.ID)
		}
	}

	// Epic link (link to an epic). EpicLink can be either an explicit key (e.g.,
	// "POM-945") or an internal ID (e.g., "epic-1"). Only Cloud resolves an
	// internal ID to its created key (behavior — not shape — differs here, so
	// this is a mode check, not an encoder concern); Data Center uses the raw
	// value verbatim to preserve its byte-for-byte payload. The encoder then
	// shapes the resolved key — Data Center writes the epic-link custom field,
	// Cloud uses the unified parent field.
	if issue.EpicLink != "" {
		epicKey := issue.EpicLink
		if a.mode == jira.ModeCloud {
			if createdKey, ok := a.createdIssues[issue.EpicLink]; ok {
				epicKey = createdKey
			}
		}
		a.encoder().epicLink(fields, epicKey, a.getEpicLinkFieldID())
	}

	// Custom fields
	maps.Copy(fields, a.config.EffectiveCustomFields(issue))

	return jira.BuildIssueFields(a.config.Defaults.ProjectKey, fields), nil
}

// resolveUserField converts a user reference (from assignee/reporter config) into
// the Jira API field value for the current platform.
//
// Data Center: always {"name": value} — byte-for-byte identical to prior behavior;
// no lookup is ever performed.
//
// Cloud: an email-shaped value (contains "@") is resolved to a Cloud account ID
// via SearchUsers and returned as {"id": accountId}; each distinct email is looked
// up at most once per run (userCache). A value that is not email-shaped is treated
// as an already-resolved account ID and returned as {"id": value} with no lookup.
// A lookup returning zero matches fails with a "not found" error naming the value;
// two or more matches fails with an "ambiguous" error naming the value. In dry-run,
// no lookup is performed: an email yields {"id": value} (the raw email) so that
// field summaries still render without any network call.
func (a *Applier) resolveUserField(ctx context.Context, value string) (map[string]any, error) {
	// Whether we resolve users at all is a behavior difference (Data Center never
	// looks a user up), so it stays a mode check on the one stored field. The
	// final VALUE SHAPE, however, is delegated to the encoder's userRef.
	if a.mode != jira.ModeCloud {
		return a.encoder().userRef(value), nil
	}

	// Cloud: values that don't look like emails are already account IDs.
	if !strings.Contains(value, "@") {
		return a.encoder().userRef(value), nil
	}

	// Dry-run performs no lookups; keep the field id-shaped for summaries.
	if a.dryRun {
		return a.encoder().userRef(value), nil
	}

	// Per-run cache: look up each distinct email at most once.
	if accountID, ok := a.userCache[value]; ok {
		return a.encoder().userRef(accountID), nil
	}

	users, err := a.client.SearchUsers(ctx, value)
	if err != nil {
		return nil, fmt.Errorf("user search for %q: %w", value, err)
	}
	switch len(users) {
	case 0:
		return nil, fmt.Errorf("user %q not found among Cloud users", value)
	case 1:
		accountID := users[0].AccountID
		a.userCache[value] = accountID
		return a.encoder().userRef(accountID), nil
	default:
		return nil, fmt.Errorf("user %q is ambiguous across multiple Cloud users (%d matches)", value, len(users))
	}
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
	maps.Copy(vars, issue.TemplateVars)

	// Two-pass placeholder replacement to prevent overlap injection.
	// Pass 1: Replace all {key} placeholders with unique sentinel tokens.
	// Pass 2: Replace sentinels with final values.
	// This ensures that a value containing "{other_key}" is not re-expanded.
	result := a.config.Defaults.DescriptionTemplate
	type replacement struct {
		sentinel string
		value    string
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	replacements := make([]replacement, 0, len(keys))
	for i, key := range keys {
		value := vars[key]
		placeholder := "{" + key + "}"
		sentinel := fmt.Sprintf("\x00TMPL_%d\x00", i)
		result = strings.ReplaceAll(result, placeholder, sentinel)
		replacements = append(replacements, replacement{sentinel: sentinel, value: value})
	}
	for _, r := range replacements {
		result = strings.ReplaceAll(result, r.sentinel, r.value)
	}

	return result, nil
}

// createIssueLinks creates all issue links defined in the configuration.
func (a *Applier) createIssueLinks(ctx context.Context) error {
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
		linkTypes, err = a.client.FetchIssueLinkTypes(ctx)
		if err != nil {
			return fmt.Errorf("fetch issue link types: %w", err)
		}
	}

	var linkCount int
	var errs []error

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
				// If the target looks like an existing Jira key (e.g. DEMO-2679),
				// use it directly — this allows linking to issues that already exist in Jira.
				if jira.IsJiraKey(link.Target) {
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
				errs = append(errs, fmt.Errorf("link %s -> %s: %w", sourceKey, targetKey, err))
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
				// The encoder shapes the comment body per platform (Cloud ADF vs
				// Data Center raw string) and reports degraded markup for verbose output.
				body, degraded := a.encoder().linkComment(link.Comment)
				req.Comment = &jira.LinkComment{Body: body}
				if degraded {
					a.degradedIssues = append(a.degradedIssues, issue.ID)
				}
			}

			if err := a.client.CreateIssueLink(ctx, req); err != nil {
				fmt.Printf("⚠️  Failed to link %s -> %s: %v\n", sourceKey, targetKey, err)
				errs = append(errs, fmt.Errorf("link %s -> %s: %w", sourceKey, targetKey, err))
				continue
			}

			linkCount++
			if a.verbose {
				fmt.Printf("  🔗 Linked: %s -[%s]-> %s\n", sourceKey, link.Type, targetKey)
			}
		}
	}

	if linkCount > 0 {
		if a.dryRun {
			fmt.Printf("\n🔍 [DRY RUN] Would create %d issue links\n", linkCount)
		} else {
			fmt.Printf("\n🔗 Created %d issue links\n", linkCount)
		}
	}

	return errors.Join(errs...)
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
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// getEpicNameFieldID returns the field ID for epic name (configurable).
func (a *Applier) getEpicNameFieldID() string {
	if a.config.Defaults.EpicNameField != "" {
		return a.config.Defaults.EpicNameField
	}
	return jira.DefaultEpicNameField
}

// getEpicLinkFieldID returns the field ID for epic link (configurable).
func (a *Applier) getEpicLinkFieldID() string {
	if a.config.Defaults.EpicLinkField != "" {
		return a.config.Defaults.EpicLinkField
	}
	return jira.DefaultEpicLinkField
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

	// Build ordered result — skip any IDs that aren't in the config (e.g.
	// unknown dependsOn references that slipped past validation).
	ordered := make([]config.Issue, 0, len(result))
	for _, id := range result {
		if issue, ok := issueMap[id]; ok {
			ordered = append(ordered, *issue)
		}
	}

	return ordered, nil
}
