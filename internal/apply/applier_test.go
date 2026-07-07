package apply

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kengou/jira-ticket-creator/internal/config"
	"github.com/kengou/jira-ticket-creator/internal/jira"
)

// mockClient implements JiraClient for testing.
type mockClient struct {
	createIssueFn     func(*jira.CreateIssueRequest) (*jira.CreateIssueResponse, error)
	createIssueLinkFn func(*jira.IssueLinkRequest) error
	fetchLinkTypesFn  func() ([]jira.IssueLinkTypeInfo, error)
	searchUsersFn     func(string) ([]jira.User, error)
}

func (m *mockClient) CreateIssue(_ context.Context, r *jira.CreateIssueRequest) (*jira.CreateIssueResponse, error) {
	return m.createIssueFn(r)
}

func (m *mockClient) CreateIssueLink(_ context.Context, r *jira.IssueLinkRequest) error {
	return m.createIssueLinkFn(r)
}

func (m *mockClient) FetchIssueLinkTypes(_ context.Context) ([]jira.IssueLinkTypeInfo, error) {
	return m.fetchLinkTypesFn()
}

func (m *mockClient) SearchUsers(_ context.Context, query string) ([]jira.User, error) {
	if m.searchUsersFn == nil {
		return nil, nil
	}
	return m.searchUsersFn(query)
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
	source, target := "DEMO-5374", "DEMO-2169"
	var inward, outward string
	switch resolved.matchedVia {
	case matchedViaInward:
		inward = source
		outward = target
	default:
		inward = target
		outward = source
	}
	if outward != "DEMO-5374" || inward != "DEMO-2169" {
		t.Errorf("direction: outward=%q inward=%q, want outward=DEMO-5374 inward=DEMO-2169", outward, inward)
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

	source, target := "DEMO-5374", "DEMO-2169"
	var inward, outward string
	switch resolved.matchedVia {
	case matchedViaInward:
		inward = source
		outward = target
	default:
		inward = target
		outward = source
	}
	if inward != "DEMO-5374" || outward != "DEMO-2169" {
		t.Errorf("direction: inward=%q outward=%q, want inward=DEMO-5374 outward=DEMO-2169", inward, outward)
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

// --- renderDescription security regression tests ---

func TestRenderDescription_NoDoubleExpansion(t *testing.T) {
	// Security regression: a template variable value containing "{other_key}"
	// must not be re-expanded. Two-pass sentinel approach should prevent this.
	a := &Applier{
		config: &config.Config{
			Defaults: config.Defaults{
				DescriptionTemplate: "Goal: {goal}\nSummary: {summary}",
			},
		},
	}
	issue := &config.Issue{
		Summary: "{goal}", // value contains another placeholder
		TemplateVars: map[string]string{
			"goal": "exploit",
		},
	}
	got, err := a.renderDescription(issue)
	if err != nil {
		t.Fatalf("renderDescription: %v", err)
	}
	// {summary} is replaced with the literal string "{goal}" — it must NOT be
	// further expanded to "exploit". The output should contain "{goal}" literally.
	if got == "Goal: exploit\nSummary: exploit" {
		t.Error("double expansion occurred: value of {summary} was re-expanded as {goal}")
	}
	// The correct output has the literal {goal} from the summary value
	if got != "Goal: exploit\nSummary: {goal}" {
		t.Errorf("unexpected output: %q", got)
	}
}

func TestRenderDescription_DeterministicOrder(t *testing.T) {
	// Verify that replacement order does not depend on map iteration order
	a := &Applier{
		config: &config.Config{
			Defaults: config.Defaults{
				DescriptionTemplate: "{a}-{b}-{c}",
			},
		},
	}
	issue := &config.Issue{
		Summary: "ignored",
		TemplateVars: map[string]string{
			"a": "alpha",
			"b": "beta",
			"c": "gamma",
		},
	}
	want := "alpha-beta-gamma"
	for range 10 {
		got, err := a.renderDescription(issue)
		if err != nil {
			t.Fatalf("renderDescription: %v", err)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
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

	fields, err := a.buildIssueFields(context.Background(), issue)
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
		epicIDs: map[string]bool{"epic-1": true},
	}

	fields, err := a.buildIssueFields(context.Background(), &cfg.Issues[1])
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

	fields, err := a.buildIssueFields(context.Background(), &cfg.Issues[1])
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

	_, err := a.buildIssueFields(context.Background(), &cfg.Issues[1])
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

	fields, err := a.buildIssueFields(context.Background(), issue)
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

	fields, err := a.buildIssueFields(context.Background(), issue)
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

	fields2, err := a.buildIssueFields(context.Background(), issue2)
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

	fields, err := a.buildIssueFields(context.Background(), issue)
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

	a := NewApplier(cfg, nil, jira.ModeDataCenter, true, true, "config.yaml")

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

	a := NewApplier(cfg, nil, jira.ModeDataCenter, false, false, "config.yaml")

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

// --- epicIDs set (replaces isEpic method) ---

func TestEpicIDs_ExplicitEpicType(t *testing.T) {
	a := NewApplier(&config.Config{
		Issues: []config.Issue{{ID: "epic-1", IssueType: "Epic"}},
	}, nil, jira.ModeDataCenter, false, false, "config.yaml")
	if !a.epicIDs["epic-1"] {
		t.Error("epicIDs should contain 'epic-1'")
	}
}

func TestEpicIDs_CaseInsensitive(t *testing.T) {
	a := NewApplier(&config.Config{
		Issues: []config.Issue{{ID: "epic-1", IssueType: "epic"}},
	}, nil, jira.ModeDataCenter, false, false, "config.yaml")
	if !a.epicIDs["epic-1"] {
		t.Error("epicIDs should be case-insensitive for 'epic'")
	}
}

func TestEpicIDs_DefaultIssueType(t *testing.T) {
	a := NewApplier(&config.Config{
		Defaults: config.Defaults{IssueType: "Epic"},
		Issues:   []config.Issue{{ID: "epic-1"}}, // inherits default
	}, nil, jira.ModeDataCenter, false, false, "config.yaml")
	if !a.epicIDs["epic-1"] {
		t.Error("epicIDs should use default issue type")
	}
}

func TestEpicIDs_NonEpicType(t *testing.T) {
	a := NewApplier(&config.Config{
		Issues: []config.Issue{{ID: "story-1", IssueType: "Story"}},
	}, nil, jira.ModeDataCenter, false, false, "config.yaml")
	if a.epicIDs["story-1"] {
		t.Error("epicIDs should not contain Story type")
	}
}

func TestEpicIDs_UnknownID(t *testing.T) {
	a := NewApplier(&config.Config{
		Issues: []config.Issue{{ID: "epic-1", IssueType: "Epic"}},
	}, nil, jira.ModeDataCenter, false, false, "config.yaml")
	if a.epicIDs["nonexistent"] {
		t.Error("epicIDs should not contain unknown ID")
	}
}

func TestEpicIDs_EmptyConfig(t *testing.T) {
	a := NewApplier(&config.Config{}, nil, jira.ModeDataCenter, false, false, "config.yaml")
	if a.epicIDs["anything"] {
		t.Error("epicIDs should be empty for empty config")
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
	if err := a.createIssueLinks(context.Background()); err != nil {
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
	if err := a.createIssueLinks(context.Background()); err != nil {
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
	if err := a.createIssueLinks(context.Background()); err != nil {
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
	if err := a.createIssueLinks(context.Background()); err != nil {
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
	if err := a.createIssueLinks(context.Background()); err != nil {
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
	err := a.createIssueLinks(context.Background())
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
	err := a.createIssueLinks(context.Background())
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
	err := a.createIssueLinks(context.Background())
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

// --- buildIssueFields: Cloud epic translation ---

func TestBuildIssueFields_Cloud_EpicLink_UsesParentField(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{ProjectKey: "PROJ"},
		Options:  &config.Options{},
	}
	a := &Applier{
		config:        cfg,
		mode:          jira.ModeCloud,
		enc:           newFieldEncoder(jira.ModeCloud),
		createdIssues: make(map[string]string),
	}

	issue := &config.Issue{ID: "story-1", IssueType: "Story", Summary: "Story", EpicLink: "PROJ-100"}

	fields, err := a.buildIssueFields(context.Background(), issue)
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}

	parent, ok := fields["parent"].(map[string]any)
	if !ok {
		t.Fatalf("Cloud epicLink should set parent field, got %v (%T)", fields["parent"], fields["parent"])
	}
	if parent["key"] != "PROJ-100" {
		t.Errorf("parent.key = %v, want PROJ-100", parent["key"])
	}
	if _, ok := fields["customfield_10009"]; ok {
		t.Error("Cloud: epic-link custom field customfield_10009 must NOT be set")
	}
}

// TestBuildIssueFields_Cloud_EpicLink_InternalID_ResolvesToCreatedKey pins the
// Cloud internal-ID lookup path: an EpicLink referencing an internal ID present
// in createdIssues must resolve to the created Jira key on the parent field.
// (TestBuildIssueFields_Cloud_EpicLink_UsesParentField above only exercises the
// raw-key fallback where EpicLink is already a Jira key.)
func TestBuildIssueFields_Cloud_EpicLink_InternalID_ResolvesToCreatedKey(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{ProjectKey: "PROJ"},
		Options:  &config.Options{},
	}
	a := &Applier{
		config:        cfg,
		mode:          jira.ModeCloud,
		enc:           newFieldEncoder(jira.ModeCloud),
		createdIssues: map[string]string{"epic-1": "PROJ-100"},
	}

	issue := &config.Issue{ID: "story-1", IssueType: "Story", Summary: "Story", EpicLink: "epic-1"}

	fields, err := a.buildIssueFields(context.Background(), issue)
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}

	parent, ok := fields["parent"].(map[string]any)
	if !ok {
		t.Fatalf("Cloud epicLink should set parent field, got %v (%T)", fields["parent"], fields["parent"])
	}
	if parent["key"] != "PROJ-100" {
		t.Errorf("parent.key = %v, want PROJ-100 (internal ID epic-1 resolved to created key)", parent["key"])
	}
}

func TestBuildIssueFields_Cloud_EpicParent_UsesParentField(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{ProjectKey: "PROJ", IssueType: "Story"},
		Options:  &config.Options{},
		Issues: []config.Issue{
			{ID: "epic-1", IssueType: "Epic", Summary: "My Epic"},
			{ID: "story-1", IssueType: "Story", Summary: "Story under epic", Parent: "epic-1"},
		},
	}
	a := &Applier{
		config:        cfg,
		mode:          jira.ModeCloud,
		enc:           newFieldEncoder(jira.ModeCloud),
		createdIssues: map[string]string{"epic-1": "PROJ-100"},
		epicIDs:       map[string]bool{"epic-1": true},
	}

	fields, err := a.buildIssueFields(context.Background(), &cfg.Issues[1])
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}

	parent, ok := fields["parent"].(map[string]any)
	if !ok {
		t.Fatalf("Cloud epic parent should set parent field, got %v (%T)", fields["parent"], fields["parent"])
	}
	if parent["key"] != "PROJ-100" {
		t.Errorf("parent.key = %v, want PROJ-100", parent["key"])
	}
	if _, ok := fields["customfield_10009"]; ok {
		t.Error("Cloud: epic-link custom field customfield_10009 must NOT be set for epic parent")
	}
}

func TestBuildIssueFields_Cloud_EpicName_DroppedAndRecorded(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{ProjectKey: "PROJ", IssueType: "Epic"},
		Options:  &config.Options{},
	}
	a := &Applier{
		config:        cfg,
		mode:          jira.ModeCloud,
		enc:           newFieldEncoder(jira.ModeCloud),
		verbose:       true,
		createdIssues: make(map[string]string),
	}

	issue := &config.Issue{ID: "epic-1", IssueType: "Epic", Summary: "My Epic", EpicName: "Epic Display Name"}

	fields, err := a.buildIssueFields(context.Background(), issue)
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}

	if _, ok := fields["customfield_10011"]; ok {
		t.Error("Cloud: epic-name custom field customfield_10011 must NOT be set")
	}
	if len(a.droppedEpicNames) != 1 || a.droppedEpicNames[0] != "epic-1" {
		t.Errorf("droppedEpicNames = %v, want [epic-1]", a.droppedEpicNames)
	}
}

func TestBuildIssueFields_Cloud_CustomEpicFieldIDs_Ignored(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{
			ProjectKey:    "PROJ",
			EpicNameField: "customfield_77777",
			EpicLinkField: "customfield_88888",
		},
		Options: &config.Options{},
	}
	a := &Applier{
		config:        cfg,
		mode:          jira.ModeCloud,
		enc:           newFieldEncoder(jira.ModeCloud),
		createdIssues: make(map[string]string),
	}

	epic := &config.Issue{ID: "epic-1", IssueType: "Epic", Summary: "Epic", EpicName: "Name"}
	epicFields, err := a.buildIssueFields(context.Background(), epic)
	if err != nil {
		t.Fatalf("buildIssueFields epic: %v", err)
	}
	if _, ok := epicFields["customfield_77777"]; ok {
		t.Error("Cloud: custom epic-name field customfield_77777 must NOT appear")
	}

	story := &config.Issue{ID: "story-1", IssueType: "Story", Summary: "Story", EpicLink: "PROJ-100"}
	storyFields, err := a.buildIssueFields(context.Background(), story)
	if err != nil {
		t.Fatalf("buildIssueFields story: %v", err)
	}
	if _, ok := storyFields["customfield_88888"]; ok {
		t.Error("Cloud: custom epic-link field customfield_88888 must NOT appear")
	}
	parent, ok := storyFields["parent"].(map[string]any)
	if !ok || parent["key"] != "PROJ-100" {
		t.Errorf("Cloud epicLink should route to parent {key: PROJ-100}, got %v", storyFields["parent"])
	}
}

func TestBuildIssueFields_Cloud_NonEpicParent_StillUsesParentField(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{ProjectKey: "PROJ", IssueType: "Task"},
		Options:  &config.Options{},
		Issues: []config.Issue{
			{ID: "task-1", IssueType: "Task", Summary: "Parent Task"},
			{ID: "subtask-1", IssueType: "Sub-task", Summary: "Sub-task", Parent: "task-1"},
		},
	}
	a := &Applier{
		config:        cfg,
		mode:          jira.ModeCloud,
		enc:           newFieldEncoder(jira.ModeCloud),
		createdIssues: map[string]string{"task-1": "PROJ-200"},
	}

	fields, err := a.buildIssueFields(context.Background(), &cfg.Issues[1])
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}
	parent, ok := fields["parent"].(map[string]any)
	if !ok {
		t.Fatal("Cloud: non-epic parent should still set parent field")
	}
	if parent["key"] != "PROJ-200" {
		t.Errorf("parent.key = %v, want PROJ-200", parent["key"])
	}
}

// --- resolveUserField / assignee-reporter resolution ---

// stubSearchUsers records queries and returns canned results, so cache and
// error-path tests can drive resolveUserField deterministically.
type stubSearchUsers struct {
	results map[string][]jira.User // query -> matches
	calls   map[string]int         // query -> number of SearchUsers calls
}

func newStubSearchUsers(results map[string][]jira.User) *stubSearchUsers {
	return &stubSearchUsers{results: results, calls: make(map[string]int)}
}

func (s *stubSearchUsers) CreateIssue(_ context.Context, _ *jira.CreateIssueRequest) (*jira.CreateIssueResponse, error) {
	return &jira.CreateIssueResponse{Key: "TEST-1"}, nil
}
func (s *stubSearchUsers) CreateIssueLink(_ context.Context, _ *jira.IssueLinkRequest) error {
	return nil
}
func (s *stubSearchUsers) FetchIssueLinkTypes(_ context.Context) ([]jira.IssueLinkTypeInfo, error) {
	return nil, nil
}
func (s *stubSearchUsers) SearchUsers(_ context.Context, query string) ([]jira.User, error) {
	s.calls[query]++
	return s.results[query], nil
}

func newCloudApplier(client JiraClient) *Applier {
	return &Applier{
		config:        &config.Config{Defaults: config.Defaults{ProjectKey: "PROJ"}, Options: &config.Options{}},
		client:        client,
		mode:          jira.ModeCloud,
		enc:           newFieldEncoder(jira.ModeCloud),
		createdIssues: make(map[string]string),
		userCache:     make(map[string]string),
	}
}

func TestResolveUserField_DataCenter_UsesNameNoLookup(t *testing.T) {
	stub := newStubSearchUsers(nil)
	a := &Applier{
		config:        &config.Config{Defaults: config.Defaults{ProjectKey: "PROJ"}, Options: &config.Options{}},
		client:        stub,
		mode:          jira.ModeDataCenter,
		enc:           newFieldEncoder(jira.ModeDataCenter),
		createdIssues: make(map[string]string),
		userCache:     make(map[string]string),
	}
	got, err := a.resolveUserField(context.Background(), "jdoe@example.com")
	if err != nil {
		t.Fatalf("resolveUserField: %v", err)
	}
	if got["name"] != "jdoe@example.com" {
		t.Errorf("DC field = %v, want {\"name\":\"jdoe@example.com\"}", got)
	}
	if _, ok := got["id"]; ok {
		t.Error("DC field must NOT carry an id")
	}
	if len(stub.calls) != 0 {
		t.Errorf("DC must not call SearchUsers; calls = %v", stub.calls)
	}
}

func TestResolveUserField_Cloud_EmailResolvedToID(t *testing.T) {
	stub := newStubSearchUsers(map[string][]jira.User{
		"alice@example.com": {{AccountID: "acc-alice"}},
	})
	a := newCloudApplier(stub)
	got, err := a.resolveUserField(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("resolveUserField: %v", err)
	}
	if got["id"] != "acc-alice" {
		t.Errorf("Cloud field = %v, want {\"id\":\"acc-alice\"}", got)
	}
	if _, ok := got["name"]; ok {
		t.Error("Cloud field must NOT carry a name")
	}
}

func TestResolveUserField_Cloud_AccountIDPassthroughNoLookup(t *testing.T) {
	stub := newStubSearchUsers(nil)
	a := newCloudApplier(stub)
	got, err := a.resolveUserField(context.Background(), "5b10ac8d82e05b22cc7d4ef5")
	if err != nil {
		t.Fatalf("resolveUserField: %v", err)
	}
	if got["id"] != "5b10ac8d82e05b22cc7d4ef5" {
		t.Errorf("Cloud field = %v, want {\"id\":\"5b10ac8d82e05b22cc7d4ef5\"}", got)
	}
	if len(stub.calls) != 0 {
		t.Errorf("account-ID passthrough must not call SearchUsers; calls = %v", stub.calls)
	}
}

func TestResolveUserField_Cloud_CachesLookup(t *testing.T) {
	stub := newStubSearchUsers(map[string][]jira.User{
		"bob@example.com": {{AccountID: "acc-bob"}},
	})
	a := newCloudApplier(stub)
	for i := 0; i < 3; i++ {
		got, err := a.resolveUserField(context.Background(), "bob@example.com")
		if err != nil {
			t.Fatalf("resolveUserField (iter %d): %v", i, err)
		}
		if got["id"] != "acc-bob" {
			t.Errorf("iter %d: id = %v, want acc-bob", i, got["id"])
		}
	}
	if stub.calls["bob@example.com"] != 1 {
		t.Errorf("SearchUsers called %d times, want 1 (per-run cache)", stub.calls["bob@example.com"])
	}
}

func TestResolveUserField_Cloud_NotFound(t *testing.T) {
	stub := newStubSearchUsers(map[string][]jira.User{}) // no matches for any query
	a := newCloudApplier(stub)
	_, err := a.resolveUserField(context.Background(), "ghost@example.com")
	if err == nil {
		t.Fatal("expected an error for a zero-match email")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ghost@example.com") {
		t.Errorf("error %q should name the value", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "not found") {
		t.Errorf("error %q should state 'not found'", msg)
	}
}

func TestResolveUserField_Cloud_Ambiguous(t *testing.T) {
	stub := newStubSearchUsers(map[string][]jira.User{
		"dup@example.com": {{AccountID: "acc-1"}, {AccountID: "acc-2"}},
	})
	a := newCloudApplier(stub)
	_, err := a.resolveUserField(context.Background(), "dup@example.com")
	if err == nil {
		t.Fatal("expected an error for a multi-match email")
	}
	msg := err.Error()
	if !strings.Contains(msg, "dup@example.com") {
		t.Errorf("error %q should name the value", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "ambiguous") {
		t.Errorf("error %q should state 'ambiguous'", msg)
	}
}

func TestResolveUserField_Cloud_DryRunNoLookup(t *testing.T) {
	stub := newStubSearchUsers(map[string][]jira.User{
		"alice@example.com": {{AccountID: "acc-alice"}},
	})
	a := newCloudApplier(stub)
	a.dryRun = true
	got, err := a.resolveUserField(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("resolveUserField (dry-run): %v", err)
	}
	if len(stub.calls) != 0 {
		t.Errorf("dry-run must not call SearchUsers; calls = %v", stub.calls)
	}
	// Dry-run still emits a plausible id-shaped field so field summaries render.
	if _, ok := got["id"]; !ok {
		t.Errorf("dry-run Cloud field should be id-shaped, got %v", got)
	}
}

func TestBuildIssueFields_Cloud_RoutesAssigneeAndReporterThroughResolution(t *testing.T) {
	stub := newStubSearchUsers(map[string][]jira.User{
		"alice@example.com": {{AccountID: "acc-alice"}},
		"bob@example.com":   {{AccountID: "acc-bob"}},
	})
	a := newCloudApplier(stub)
	issue := &config.Issue{
		ID: "s1", IssueType: "Story", Summary: "Story",
		Assignee: "alice@example.com", Reporter: "bob@example.com",
	}
	fields, err := a.buildIssueFields(context.Background(), issue)
	if err != nil {
		t.Fatalf("buildIssueFields: %v", err)
	}
	assignee, ok := fields["assignee"].(map[string]any)
	if !ok || assignee["id"] != "acc-alice" {
		t.Errorf("assignee = %v, want {\"id\":\"acc-alice\"}", fields["assignee"])
	}
	reporter, ok := fields["reporter"].(map[string]any)
	if !ok || reporter["id"] != "acc-bob" {
		t.Errorf("reporter = %v, want {\"id\":\"acc-bob\"}", fields["reporter"])
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

// --- reporter permission rejection surfacing (Cloud) ---

func TestApply_Cloud_ReporterPermissionRejection_SurfacesAPIError(t *testing.T) {
	t.Chdir(t.TempDir())

	var createAttempts int
	reporterErr := &jira.APIError{
		StatusCode: 400,
		Body:       `{"errors":{"reporter":"Field 'reporter' cannot be set. It is not on the appropriate screen, or unknown."}}`,
	}
	client := &mockClient{
		createIssueFn: func(_ *jira.CreateIssueRequest) (*jira.CreateIssueResponse, error) {
			createAttempts++
			return nil, reporterErr
		},
		searchUsersFn: func(query string) ([]jira.User, error) {
			return []jira.User{{AccountID: "acc-" + query}}, nil
		},
	}

	cfg := &config.Config{
		Defaults: config.Defaults{ProjectKey: "PROJ"},
		Options:  &config.Options{},
		Issues: []config.Issue{
			{ID: "s1", IssueType: "Story", Summary: "Story", Reporter: "someone@example.com"},
		},
	}
	applier := NewApplier(cfg, client, jira.ModeCloud, false, false, "cfg.yaml")

	err := applier.Apply(context.Background())
	if err == nil {
		t.Fatal("Apply should return the reporter rejection error (no continueOnError)")
	}
	// The raw APIError must be recoverable from the chain (recognizable surfacing).
	var apiErr *jira.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want a *jira.APIError in the chain", err)
	}
	if !strings.Contains(apiErr.Body, "reporter") {
		t.Errorf("surfaced APIError body %q should mention the reporter field", apiErr.Body)
	}
	// Exactly one create attempt: no automatic retry-without-reporter.
	if createAttempts != 1 {
		t.Errorf("createAttempts = %d, want 1 (no retry-without-reporter)", createAttempts)
	}
}
