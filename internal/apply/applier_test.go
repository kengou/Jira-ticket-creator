package apply

import (
	"errors"
	"strings"
	"testing"

	"github.com/kengou/Jira-ticket-creator/internal/config"
	"github.com/kengou/Jira-ticket-creator/internal/jira"
)

// mockClient implements JiraClient for testing.
type mockClient struct {
	createIssueFn     func(*jira.CreateIssueRequest) (*jira.CreateIssueResponse, error)
	createIssueLinkFn func(*jira.IssueLinkRequest) error
	fetchLinkTypesFn  func() ([]jira.IssueLinkTypeInfo, error)
}

func (m *mockClient) CreateIssue(r *jira.CreateIssueRequest) (*jira.CreateIssueResponse, error) {
	return m.createIssueFn(r)
}

func (m *mockClient) CreateIssueLink(r *jira.IssueLinkRequest) error {
	return m.createIssueLinkFn(r)
}

func (m *mockClient) FetchIssueLinkTypes() ([]jira.IssueLinkTypeInfo, error) {
	return m.fetchLinkTypesFn()
}

// --- BuildDependencyGraph ---

func TestBuildDependencyGraph_NoDependencies(t *testing.T) {
	issues := []config.Issue{
		{ID: "a", Summary: "A"},
		{ID: "b", Summary: "B"},
		{ID: "c", Summary: "C"},
	}

	ordered, err := BuildDependencyGraph(issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ordered) != 3 {
		t.Fatalf("len = %d, want 3", len(ordered))
	}

	// Without dependencies, order should match input
	for i, issue := range ordered {
		if issue.ID != issues[i].ID {
			t.Errorf("ordered[%d].ID = %q, want %q", i, issue.ID, issues[i].ID)
		}
	}
}

func TestBuildDependencyGraph_LinearChain(t *testing.T) {
	issues := []config.Issue{
		{ID: "c", Summary: "C", DependsOn: []string{"b"}},
		{ID: "b", Summary: "B", DependsOn: []string{"a"}},
		{ID: "a", Summary: "A"},
	}

	ordered, err := BuildDependencyGraph(issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// a must come before b, b before c
	idxA := indexOf(ordered, "a")
	idxB := indexOf(ordered, "b")
	idxC := indexOf(ordered, "c")

	if idxA >= idxB {
		t.Errorf("a (idx %d) must come before b (idx %d)", idxA, idxB)
	}
	if idxB >= idxC {
		t.Errorf("b (idx %d) must come before c (idx %d)", idxB, idxC)
	}
}

func TestBuildDependencyGraph_ParentDependency(t *testing.T) {
	issues := []config.Issue{
		{ID: "story", Summary: "Story", Parent: "epic"},
		{ID: "epic", Summary: "Epic"},
	}

	ordered, err := BuildDependencyGraph(issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	idxEpic := indexOf(ordered, "epic")
	idxStory := indexOf(ordered, "story")

	if idxEpic >= idxStory {
		t.Errorf("epic (idx %d) must come before story (idx %d)", idxEpic, idxStory)
	}
}

func TestBuildDependencyGraph_CircularDependency(t *testing.T) {
	issues := []config.Issue{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	}

	_, err := BuildDependencyGraph(issues)
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
}

func TestBuildDependencyGraph_DiamondDependency(t *testing.T) {
	//   a
	//  / \
	// b   c
	//  \ /
	//   d
	issues := []config.Issue{
		{ID: "d", DependsOn: []string{"b", "c"}},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "a"},
	}

	ordered, err := BuildDependencyGraph(issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	idxA := indexOf(ordered, "a")
	idxB := indexOf(ordered, "b")
	idxC := indexOf(ordered, "c")
	idxD := indexOf(ordered, "d")

	if idxA >= idxB {
		t.Errorf("a must come before b")
	}
	if idxA >= idxC {
		t.Errorf("a must come before c")
	}
	if idxB >= idxD {
		t.Errorf("b must come before d")
	}
	if idxC >= idxD {
		t.Errorf("c must come before d")
	}
}

func TestBuildDependencyGraph_SingleIssue(t *testing.T) {
	issues := []config.Issue{
		{ID: "solo", Summary: "Solo issue"},
	}

	ordered, err := BuildDependencyGraph(issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ordered) != 1 || ordered[0].ID != "solo" {
		t.Errorf("unexpected result: %v", ordered)
	}
}

func TestBuildDependencyGraph_Empty(t *testing.T) {
	ordered, err := BuildDependencyGraph(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ordered) != 0 {
		t.Errorf("expected empty, got %d", len(ordered))
	}
}

// --- resolveLinkType ---

func TestResolveLinkType_MatchByName(t *testing.T) {
	available := []jira.IssueLinkTypeInfo{
		{ID: "1", Name: "Problem/Incident", Inward: "is caused by", Outward: "causes"},
	}
	resolved, err := resolveLinkType("Problem/Incident", available)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.name != "Problem/Incident" {
		t.Errorf("name = %q, want %q", resolved.name, "Problem/Incident")
	}
	if resolved.matchedVia != matchedViaName {
		t.Errorf("matchedVia = %v, want matchedViaName", resolved.matchedVia)
	}
}

func TestResolveLinkType_MatchByOutward(t *testing.T) {
	available := []jira.IssueLinkTypeInfo{
		{ID: "1", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
	}
	resolved, err := resolveLinkType("blocks", available)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.name != "Blocks" {
		t.Errorf("name = %q, want %q", resolved.name, "Blocks")
	}
	if resolved.matchedVia != matchedViaOutward {
		t.Errorf("matchedVia = %v, want matchedViaOutward", resolved.matchedVia)
	}
}

func TestResolveLinkType_MatchByInward(t *testing.T) {
	available := []jira.IssueLinkTypeInfo{
		{ID: "1", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
	}
	resolved, err := resolveLinkType("is blocked by", available)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.name != "Blocks" {
		t.Errorf("name = %q, want %q", resolved.name, "Blocks")
	}
	if resolved.matchedVia != matchedViaInward {
		t.Errorf("matchedVia = %v, want matchedViaInward", resolved.matchedVia)
	}
}

func TestResolveLinkType_CaseInsensitive(t *testing.T) {
	available := []jira.IssueLinkTypeInfo{
		{ID: "1", Name: "Problem/Incident", Inward: "is caused by", Outward: "causes"},
	}
	// Match against name with different case
	resolved, err := resolveLinkType("problem/incident", available)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.name != "Problem/Incident" {
		t.Errorf("name = %q, want %q", resolved.name, "Problem/Incident")
	}
	// Match against outward with different case
	resolved2, err := resolveLinkType("CAUSES", available)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved2.name != "Problem/Incident" {
		t.Errorf("name = %q, want %q", resolved2.name, "Problem/Incident")
	}
}

func TestResolveLinkType_BlockedBy(t *testing.T) {
	available := []jira.IssueLinkTypeInfo{
		{ID: "1", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
		{ID: "2", Name: "Duplicate", Inward: "is duplicated by", Outward: "duplicates"},
	}
	resolved, err := resolveLinkType("is blocked by", available)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.name != "Blocks" {
		t.Errorf("name = %q, want %q", resolved.name, "Blocks")
	}
	if resolved.matchedVia != matchedViaInward {
		t.Errorf("matchedVia = %v, want matchedViaInward", resolved.matchedVia)
	}
}

func TestResolveLinkType_NotFound(t *testing.T) {
	available := []jira.IssueLinkTypeInfo{
		{ID: "1", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
	}
	_, err := resolveLinkType("nonexistent", available)
	if err == nil {
		t.Fatal("expected error for unknown link type, got nil")
	}
	if !strings.Contains(err.Error(), "unknown link type") {
		t.Errorf("error = %q, want it to contain 'unknown link type'", err.Error())
	}
	// Should include available types in the error message
	if !strings.Contains(err.Error(), "Blocks") {
		t.Errorf("error = %q, want it to list available types", err.Error())
	}
}

func TestResolveLinkType_EmptyAvailable(t *testing.T) {
	_, err := resolveLinkType("blocks", nil)
	if err == nil {
		t.Fatal("expected error for empty available types, got nil")
	}
}

func TestResolveLinkType_DirectionFromOutward(t *testing.T) {
	// When user writes "blocks", source should be outward (source blocks target)
	available := []jira.IssueLinkTypeInfo{
		{ID: "1", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
	}
	resolved, err := resolveLinkType("blocks", available)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With matchedViaOutward, source goes to outward, target goes to inward
	source, target := "POM-1", "POM-2"
	var inward, outward string
	switch resolved.matchedVia {
	case matchedViaInward:
		inward = source
		outward = target
	default:
		inward = target
		outward = source
	}
	if outward != "POM-1" || inward != "POM-2" {
		t.Errorf("direction: outward=%q inward=%q, want outward=POM-1 inward=POM-2", outward, inward)
	}
}

func TestResolveLinkType_DirectionFromInward(t *testing.T) {
	// When user writes "is blocked by", source should be inward (source is blocked by target)
	available := []jira.IssueLinkTypeInfo{
		{ID: "1", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
	}
	resolved, err := resolveLinkType("is blocked by", available)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	source, target := "POM-1", "POM-2"
	var inward, outward string
	switch resolved.matchedVia {
	case matchedViaInward:
		inward = source
		outward = target
	default:
		inward = target
		outward = source
	}
	if inward != "POM-1" || outward != "POM-2" {
		t.Errorf("direction: inward=%q outward=%q, want inward=POM-1 outward=POM-2", inward, outward)
	}
}

// --- truncate ---

func TestTruncate_Short(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate('hello', 10) = %q, want 'hello'", got)
	}
}

func TestTruncate_ExactLength(t *testing.T) {
	if got := truncate("hello", 5); got != "hello" {
		t.Errorf("truncate('hello', 5) = %q, want 'hello'", got)
	}
}

func TestTruncate_TooLong(t *testing.T) {
	got := truncate("hello world", 8)
	if got != "hello..." {
		t.Errorf("truncate('hello world', 8) = %q, want 'hello...'", got)
	}
}

func TestTruncate_Unicode(t *testing.T) {
	// 5 Japanese characters, truncate to 4 runes
	input := "\u3053\u3093\u306b\u3061\u306f" // konnichiwa
	got := truncate(input, 4)
	// Should be first rune + "..."
	want := "\u3053..."
	if got != want {
		t.Errorf("truncate(unicode, 4) = %q, want %q", got, want)
	}
}

func TestTruncate_EmptyString(t *testing.T) {
	if got := truncate("", 5); got != "" {
		t.Errorf("truncate('', 5) = %q, want ''", got)
	}
}

// --- renderDescription ---

func TestRenderDescription_DirectDescription(t *testing.T) {
	a := &Applier{
		config: &config.Config{
			Defaults: config.Defaults{
				DescriptionTemplate: "Template: {summary}",
			},
		},
	}

	issue := &config.Issue{
		Description: "My direct description",
		TemplateVars: map[string]string{
			"summary": "ignored",
		},
	}

	got, err := a.renderDescription(issue)
	if err != nil {
		t.Fatalf("renderDescription: %v", err)
	}
	if got != "My direct description" {
		t.Errorf("got %q, want direct description to take precedence", got)
	}
}

func TestRenderDescription_Template(t *testing.T) {
	a := &Applier{
		config: &config.Config{
			Defaults: config.Defaults{
				DescriptionTemplate: "Summary: {summary}\nGoal: {goal}",
			},
		},
	}

	issue := &config.Issue{
		Summary: "Build feature",
		TemplateVars: map[string]string{
			"goal": "Ship by Friday",
		},
	}

	got, err := a.renderDescription(issue)
	if err != nil {
		t.Fatalf("renderDescription: %v", err)
	}
	want := "Summary: Build feature\nGoal: Ship by Friday"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderDescription_NoTemplateNoDescription(t *testing.T) {
	a := &Applier{
		config: &config.Config{},
	}

	got, err := a.renderDescription(&config.Issue{})
	if err != nil {
		t.Fatalf("renderDescription: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestRenderDescription_TemplateButNoVars(t *testing.T) {
	a := &Applier{
		config: &config.Config{
			Defaults: config.Defaults{
				DescriptionTemplate: "Goal: {goal}",
			},
		},
	}

	got, err := a.renderDescription(&config.Issue{Summary: "Test"})
	if err != nil {
		t.Fatalf("renderDescription: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty (no template vars)", got)
	}
}

// --- buildIssueFields ---

func TestBuildIssueFields_FullFields(t *testing.T) {
	assignee := "default-user"
	cfg := &config.Config{
		Defaults: config.Defaults{
			ProjectKey:  "PROJ",
			IssueType:   "Story",
			Priority:    "High",
			Assignee:    &assignee,
			Reporter:    "reporter-user",
			Labels:      []string{"team-a"},
			Components:  []string{"backend"},
			FixVersions: []string{"v1.0"},
		},
		Options: &config.Options{},
	}

	a := &Applier{
		config:        cfg,
		createdIssues: make(map[string]string),
	}

	issue := &config.Issue{
		ID:        "story-1",
		IssueType: "Story",
		Summary:   "My story",
	}

	fields, err := a.buildIssueFields(issue)
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}

	// Check project
	proj, ok := fields["project"].(map[string]any)
	if !ok {
		t.Fatal("project field not set")
	}
	if proj["key"] != "PROJ" {
		t.Errorf("project.key = %v, want PROJ", proj["key"])
	}

	// Check summary
	if fields["summary"] != "My story" {
		t.Errorf("summary = %v", fields["summary"])
	}

	// Check issuetype
	it, ok := fields["issuetype"].(map[string]any)
	if !ok {
		t.Fatal("issuetype not set")
	}
	if it["name"] != "Story" {
		t.Errorf("issuetype.name = %v", it["name"])
	}

	// Check priority
	pri, ok := fields["priority"].(map[string]any)
	if !ok {
		t.Fatal("priority not set")
	}
	if pri["name"] != "High" {
		t.Errorf("priority.name = %v", pri["name"])
	}

	// Check assignee
	asgn, ok := fields["assignee"].(map[string]any)
	if !ok {
		t.Fatal("assignee not set")
	}
	if asgn["name"] != "default-user" {
		t.Errorf("assignee.name = %v", asgn["name"])
	}

	// Check reporter
	rep, ok := fields["reporter"].(map[string]any)
	if !ok {
		t.Fatal("reporter not set")
	}
	if rep["name"] != "reporter-user" {
		t.Errorf("reporter.name = %v", rep["name"])
	}

	// Check labels
	labels, ok := fields["labels"].([]string)
	if !ok {
		t.Fatal("labels not set")
	}
	if len(labels) != 1 || labels[0] != "team-a" {
		t.Errorf("labels = %v", labels)
	}

	// Check components
	components, ok := fields["components"].([]any)
	if !ok {
		t.Fatal("components not set")
	}
	if len(components) != 1 {
		t.Errorf("components = %v", components)
	}

	// Check fixVersions
	fixVersions, ok := fields["fixVersions"].([]any)
	if !ok {
		t.Fatal("fixVersions not set")
	}
	if len(fixVersions) != 1 {
		t.Errorf("fixVersions = %v", fixVersions)
	}
}

func TestBuildIssueFields_ParentEpic_UsesEpicLink(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{
			ProjectKey: "PROJ",
			IssueType:  "Story",
		},
		Options: &config.Options{},
		Issues: []config.Issue{
			{ID: "epic-1", IssueType: "Epic", Summary: "My Epic"},
			{ID: "story-1", IssueType: "Story", Summary: "Story under epic", Parent: "epic-1"},
		},
	}

	a := &Applier{
		config: cfg,
		createdIssues: map[string]string{
			"epic-1": "PROJ-100",
		},
	}

	fields, err := a.buildIssueFields(&cfg.Issues[1])
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}

	// Should use epic link field, NOT parent field
	if _, ok := fields["parent"]; ok {
		t.Error("parent field should NOT be set when parent is an Epic (Data Center)")
	}
	epicLink, ok := fields["customfield_10009"].(string)
	if !ok {
		t.Fatal("epic link field (customfield_10009) not set")
	}
	if epicLink != "PROJ-100" {
		t.Errorf("epicLink = %v, want PROJ-100", epicLink)
	}
}

func TestBuildIssueFields_ParentNonEpic_UsesParentField(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{
			ProjectKey: "PROJ",
			IssueType:  "Task",
		},
		Options: &config.Options{},
		Issues: []config.Issue{
			{ID: "task-1", IssueType: "Task", Summary: "Parent Task"},
			{ID: "subtask-1", IssueType: "Sub-task", Summary: "Sub-task", Parent: "task-1"},
		},
	}

	a := &Applier{
		config: cfg,
		createdIssues: map[string]string{
			"task-1": "PROJ-200",
		},
	}

	fields, err := a.buildIssueFields(&cfg.Issues[1])
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}

	// Should use parent field for sub-tasks
	parent, ok := fields["parent"].(map[string]any)
	if !ok {
		t.Fatal("parent field not set for sub-task")
	}
	if parent["key"] != "PROJ-200" {
		t.Errorf("parent.key = %v, want PROJ-200", parent["key"])
	}
	// Should NOT use epic link
	if _, ok := fields["customfield_10009"]; ok {
		t.Error("epic link field should NOT be set for sub-task parent")
	}
}

func TestBuildIssueFields_ParentNotCreated(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{
			ProjectKey: "PROJ",
			IssueType:  "Story",
		},
		Options: &config.Options{},
		Issues: []config.Issue{
			{ID: "missing-epic", IssueType: "Epic", Summary: "Missing Epic"},
			{ID: "story-1", IssueType: "Story", Summary: "Orphaned story", Parent: "missing-epic"},
		},
	}

	a := &Applier{
		config:        cfg,
		createdIssues: make(map[string]string),
	}

	_, err := a.buildIssueFields(&cfg.Issues[1])
	if err == nil {
		t.Fatal("expected error for missing parent")
	}
}

func TestBuildIssueFields_EpicWithEpicName(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{
			ProjectKey: "PROJ",
			IssueType:  "Epic",
		},
		Options: &config.Options{},
	}

	a := &Applier{
		config:        cfg,
		createdIssues: make(map[string]string),
	}

	issue := &config.Issue{
		ID:        "epic-1",
		IssueType: "Epic",
		Summary:   "My Epic",
		EpicName:  "Epic Name",
	}

	fields, err := a.buildIssueFields(issue)
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}

	if fields["customfield_10011"] != "Epic Name" {
		t.Errorf("epicNameField = %v, want 'Epic Name'", fields["customfield_10011"])
	}
}

func TestBuildIssueFields_CustomEpicFieldIDs(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{
			ProjectKey:    "PROJ",
			EpicNameField: "customfield_99999",
			EpicLinkField: "customfield_88888",
		},
		Options: &config.Options{},
	}

	a := &Applier{
		config:        cfg,
		createdIssues: make(map[string]string),
	}

	// Test epic name with custom field
	issue := &config.Issue{
		ID:        "epic-1",
		IssueType: "Epic",
		Summary:   "My Epic",
		EpicName:  "Custom Epic Name",
	}

	fields, err := a.buildIssueFields(issue)
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}

	if fields["customfield_99999"] != "Custom Epic Name" {
		t.Errorf("custom epicNameField = %v, want 'Custom Epic Name'", fields["customfield_99999"])
	}

	// Test epic link with custom field
	issue2 := &config.Issue{
		ID:        "story-1",
		IssueType: "Story",
		Summary:   "Story",
		EpicLink:  "PROJ-100",
	}

	fields2, err := a.buildIssueFields(issue2)
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}

	link, ok := fields2["customfield_88888"].(string)
	if !ok {
		t.Fatalf("custom epicLinkField not set or not a string: %v", fields2["customfield_88888"])
	}
	if link != "PROJ-100" {
		t.Errorf("custom epicLinkField = %v, want PROJ-100", link)
	}
}

func TestBuildIssueFields_CustomFieldsMerged(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{
			ProjectKey: "PROJ",
			IssueType:  "Task",
			CustomFields: map[string]any{
				"cf_team": "alpha",
			},
		},
		Options: &config.Options{},
	}

	a := &Applier{
		config:        cfg,
		createdIssues: make(map[string]string),
	}

	issue := &config.Issue{
		ID:        "task-1",
		IssueType: "Task",
		Summary:   "Task",
		CustomFields: map[string]any{
			"cf_region": "eu",
		},
	}

	fields, err := a.buildIssueFields(issue)
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}

	if fields["cf_team"] != "alpha" {
		t.Errorf("cf_team = %v, want alpha", fields["cf_team"])
	}
	if fields["cf_region"] != "eu" {
		t.Errorf("cf_region = %v, want eu", fields["cf_region"])
	}
}

// --- NewApplier ---

func TestNewApplier_SetsFields(t *testing.T) {
	cfg := &config.Config{
		Options: &config.Options{
			ContinueOnError: true,
		},
	}

	a := NewApplier(cfg, nil, true, true, "config.yaml")

	if !a.verbose {
		t.Error("verbose should be true")
	}
	if !a.dryRun {
		t.Error("dryRun should be true")
	}
	if !a.continueOnError {
		t.Error("continueOnError should be true")
	}
	if a.configFile != "config.yaml" {
		t.Errorf("configFile = %q", a.configFile)
	}
}

func TestNewApplier_NilOptions(t *testing.T) {
	cfg := &config.Config{}

	a := NewApplier(cfg, nil, false, false, "config.yaml")

	if a.continueOnError {
		t.Error("continueOnError should default to false")
	}
}

// --- getEpicNameFieldID / getEpicLinkFieldID ---

func TestGetEpicNameFieldID_Default(t *testing.T) {
	a := &Applier{config: &config.Config{}}
	if got := a.getEpicNameFieldID(); got != "customfield_10011" {
		t.Errorf("getEpicNameFieldID() = %q, want customfield_10011", got)
	}
}

func TestGetEpicNameFieldID_Custom(t *testing.T) {
	a := &Applier{
		config: &config.Config{
			Defaults: config.Defaults{EpicNameField: "customfield_99999"},
		},
	}
	if got := a.getEpicNameFieldID(); got != "customfield_99999" {
		t.Errorf("getEpicNameFieldID() = %q, want customfield_99999", got)
	}
}

func TestGetEpicLinkFieldID_Default(t *testing.T) {
	a := &Applier{config: &config.Config{}}
	if got := a.getEpicLinkFieldID(); got != "customfield_10009" {
		t.Errorf("getEpicLinkFieldID() = %q, want customfield_10009", got)
	}
}

func TestGetEpicLinkFieldID_Custom(t *testing.T) {
	a := &Applier{
		config: &config.Config{
			Defaults: config.Defaults{EpicLinkField: "customfield_88888"},
		},
	}
	if got := a.getEpicLinkFieldID(); got != "customfield_88888" {
		t.Errorf("getEpicLinkFieldID() = %q, want customfield_88888", got)
	}
}

// --- isEpic tests ---

func TestIsEpic_ExplicitEpicType(t *testing.T) {
	a := &Applier{
		config: &config.Config{
			Issues: []config.Issue{
				{ID: "epic-1", IssueType: "Epic"},
			},
		},
	}
	if !a.isEpic("epic-1") {
		t.Error("isEpic('epic-1') = false, want true")
	}
}

func TestIsEpic_CaseInsensitive(t *testing.T) {
	a := &Applier{
		config: &config.Config{
			Issues: []config.Issue{
				{ID: "epic-1", IssueType: "epic"},
			},
		},
	}
	if !a.isEpic("epic-1") {
		t.Error("isEpic should be case-insensitive, got false for 'epic'")
	}
}

func TestIsEpic_DefaultIssueType(t *testing.T) {
	a := &Applier{
		config: &config.Config{
			Defaults: config.Defaults{IssueType: "Epic"},
			Issues: []config.Issue{
				{ID: "epic-1"}, // no explicit type, inherits default
			},
		},
	}
	if !a.isEpic("epic-1") {
		t.Error("isEpic should use default issue type, got false")
	}
}

func TestIsEpic_NonEpicType(t *testing.T) {
	a := &Applier{
		config: &config.Config{
			Issues: []config.Issue{
				{ID: "story-1", IssueType: "Story"},
			},
		},
	}
	if a.isEpic("story-1") {
		t.Error("isEpic('story-1') = true, want false for Story type")
	}
}

func TestIsEpic_UnknownID(t *testing.T) {
	a := &Applier{
		config: &config.Config{
			Issues: []config.Issue{
				{ID: "epic-1", IssueType: "Epic"},
			},
		},
	}
	if a.isEpic("nonexistent") {
		t.Error("isEpic('nonexistent') = true, want false for unknown ID")
	}
}

func TestIsEpic_EmptyConfig(t *testing.T) {
	a := &Applier{
		config: &config.Config{},
	}
	if a.isEpic("anything") {
		t.Error("isEpic should return false when no issues in config")
	}
}

// --- createIssueLinks ---

func TestCreateIssueLinks_NoLinks(t *testing.T) {
	a := &Applier{
		config: &config.Config{
			Issues: []config.Issue{
				{ID: "S1", Summary: "Story 1"},
			},
		},
		createdIssues: map[string]string{"S1": "PROJ-1"},
	}
	if err := a.createIssueLinks(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCreateIssueLinks_DryRun(t *testing.T) {
	fetchCalled := false
	linkCalled := false
	mc := &mockClient{
		fetchLinkTypesFn: func() ([]jira.IssueLinkTypeInfo, error) {
			fetchCalled = true
			return nil, nil
		},
		createIssueLinkFn: func(*jira.IssueLinkRequest) error {
			linkCalled = true
			return nil
		},
	}
	a := &Applier{
		config: &config.Config{
			Issues: []config.Issue{
				{ID: "S1", Summary: "S1", Links: []config.IssueLink{{Type: "blocks", Target: "S2"}}},
				{ID: "S2", Summary: "S2"},
			},
		},
		client:        mc,
		dryRun:        true,
		createdIssues: map[string]string{"S1": "PROJ-1", "S2": "PROJ-2"},
	}
	if err := a.createIssueLinks(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fetchCalled {
		t.Error("FetchIssueLinkTypes should NOT be called in dry-run")
	}
	if linkCalled {
		t.Error("CreateIssueLink should NOT be called in dry-run")
	}
}

func TestCreateIssueLinks_Success(t *testing.T) {
	var capturedReq *jira.IssueLinkRequest
	mc := &mockClient{
		fetchLinkTypesFn: func() ([]jira.IssueLinkTypeInfo, error) {
			return []jira.IssueLinkTypeInfo{
				{ID: "1", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
			}, nil
		},
		createIssueLinkFn: func(r *jira.IssueLinkRequest) error {
			capturedReq = r
			return nil
		},
	}
	a := &Applier{
		config: &config.Config{
			Issues: []config.Issue{
				{ID: "S1", Summary: "S1", Links: []config.IssueLink{{Type: "blocks", Target: "S2"}}},
				{ID: "S2", Summary: "S2"},
			},
		},
		client:        mc,
		createdIssues: map[string]string{"S1": "PROJ-1", "S2": "PROJ-2"},
	}
	if err := a.createIssueLinks(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("CreateIssueLink was not called")
	}
	if capturedReq.Type.Name != "Blocks" {
		t.Errorf("link type name = %q, want Blocks", capturedReq.Type.Name)
	}
	// "blocks" matched via outward → source is outward, target is inward
	if capturedReq.OutwardIssue.Key != "PROJ-1" {
		t.Errorf("outward = %q, want PROJ-1", capturedReq.OutwardIssue.Key)
	}
	if capturedReq.InwardIssue.Key != "PROJ-2" {
		t.Errorf("inward = %q, want PROJ-2", capturedReq.InwardIssue.Key)
	}
}

func TestCreateIssueLinks_TargetIsJiraKey(t *testing.T) {
	var capturedReq *jira.IssueLinkRequest
	mc := &mockClient{
		fetchLinkTypesFn: func() ([]jira.IssueLinkTypeInfo, error) {
			return []jira.IssueLinkTypeInfo{
				{ID: "1", Name: "Relates", Inward: "relates to", Outward: "relates to"},
			}, nil
		},
		createIssueLinkFn: func(r *jira.IssueLinkRequest) error {
			capturedReq = r
			return nil
		},
	}
	a := &Applier{
		config: &config.Config{
			Issues: []config.Issue{
				{ID: "S1", Summary: "S1", Links: []config.IssueLink{{Type: "relates to", Target: "EXT-999"}}},
			},
		},
		client:        mc,
		createdIssues: map[string]string{"S1": "PROJ-1"},
		// EXT-999 is not in createdIssues but is a valid Jira key
	}
	if err := a.createIssueLinks(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("CreateIssueLink was not called")
	}
	// EXT-999 should be used directly as the target
	if capturedReq.InwardIssue.Key != "EXT-999" && capturedReq.OutwardIssue.Key != "EXT-999" {
		t.Errorf("EXT-999 not used as link target; inward=%q outward=%q",
			capturedReq.InwardIssue.Key, capturedReq.OutwardIssue.Key)
	}
}

func TestCreateIssueLinks_TargetNotCreated(t *testing.T) {
	mc := &mockClient{
		fetchLinkTypesFn: func() ([]jira.IssueLinkTypeInfo, error) {
			return []jira.IssueLinkTypeInfo{
				{ID: "1", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
			}, nil
		},
		createIssueLinkFn: func(*jira.IssueLinkRequest) error {
			return nil
		},
	}
	a := &Applier{
		config: &config.Config{
			Issues: []config.Issue{
				{ID: "S1", Summary: "S1", Links: []config.IssueLink{{Type: "blocks", Target: "MISSING"}}},
			},
		},
		client:        mc,
		createdIssues: map[string]string{"S1": "PROJ-1"},
		// MISSING is not in createdIssues and not a Jira key
	}
	// Should warn but return nil (not an error)
	if err := a.createIssueLinks(); err != nil {
		t.Fatalf("expected nil error for uncreated non-jira target, got %v", err)
	}
}

func TestCreateIssueLinks_ResolveFails(t *testing.T) {
	mc := &mockClient{
		fetchLinkTypesFn: func() ([]jira.IssueLinkTypeInfo, error) {
			return []jira.IssueLinkTypeInfo{
				{ID: "1", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
			}, nil
		},
		createIssueLinkFn: func(*jira.IssueLinkRequest) error {
			return nil
		},
	}
	a := &Applier{
		config: &config.Config{
			Issues: []config.Issue{
				{ID: "S1", Summary: "S1", Links: []config.IssueLink{{Type: "nonexistent-type", Target: "S2"}}},
				{ID: "S2", Summary: "S2"},
			},
		},
		client:        mc,
		createdIssues: map[string]string{"S1": "PROJ-1", "S2": "PROJ-2"},
	}
	err := a.createIssueLinks()
	if err == nil {
		t.Fatal("expected error for unknown link type, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent-type") {
		t.Errorf("error %q should mention unknown link type", err.Error())
	}
}

func TestCreateIssueLinks_APIFails(t *testing.T) {
	apiErr := errors.New("API error 500")
	mc := &mockClient{
		fetchLinkTypesFn: func() ([]jira.IssueLinkTypeInfo, error) {
			return []jira.IssueLinkTypeInfo{
				{ID: "1", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
			}, nil
		},
		createIssueLinkFn: func(*jira.IssueLinkRequest) error {
			return apiErr
		},
	}
	a := &Applier{
		config: &config.Config{
			Issues: []config.Issue{
				{ID: "S1", Summary: "S1", Links: []config.IssueLink{{Type: "blocks", Target: "S2"}}},
				{ID: "S2", Summary: "S2"},
			},
		},
		client:        mc,
		createdIssues: map[string]string{"S1": "PROJ-1", "S2": "PROJ-2"},
	}
	err := a.createIssueLinks()
	if err == nil {
		t.Fatal("expected error from CreateIssueLink, got nil")
	}
	if !strings.Contains(err.Error(), "API error 500") {
		t.Errorf("error %q should contain API error message", err.Error())
	}
}

func TestCreateIssueLinks_MultipleErrors(t *testing.T) {
	callCount := 0
	mc := &mockClient{
		fetchLinkTypesFn: func() ([]jira.IssueLinkTypeInfo, error) {
			return []jira.IssueLinkTypeInfo{
				{ID: "1", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
			}, nil
		},
		createIssueLinkFn: func(*jira.IssueLinkRequest) error {
			callCount++
			return errors.New("link failed")
		},
	}
	a := &Applier{
		config: &config.Config{
			Issues: []config.Issue{
				{ID: "S1", Summary: "S1", Links: []config.IssueLink{
					{Type: "blocks", Target: "S2"},
					{Type: "blocks", Target: "S3"},
				}},
				{ID: "S2", Summary: "S2"},
				{ID: "S3", Summary: "S3"},
			},
		},
		client:        mc,
		createdIssues: map[string]string{"S1": "PROJ-1", "S2": "PROJ-2", "S3": "PROJ-3"},
	}
	err := a.createIssueLinks()
	if err == nil {
		t.Fatal("expected joined error, got nil")
	}
	if callCount != 2 {
		t.Errorf("CreateIssueLink called %d times, want 2", callCount)
	}
	// errors.Join produces a multi-line error; both "link failed" should appear
	if strings.Count(err.Error(), "link failed") < 2 {
		t.Errorf("expected both errors in joined result, got: %v", err)
	}
}

// --- helpers ---

func indexOf(issues []config.Issue, id string) int {
	for i, issue := range issues {
		if issue.ID == id {
			return i
		}
	}
	return -1
}
