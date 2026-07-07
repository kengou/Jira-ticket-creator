// SPDX-License-Identifier: Apache-2.0
package apply_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kengou/jira-ticket-creator/internal/apply"
	"github.com/kengou/jira-ticket-creator/internal/config"
	"github.com/kengou/jira-ticket-creator/internal/jira"
)

// fakeJiraCapturingAll returns a server that records EVERY create-issue payload
// (the shared newFakeJira helper only keeps the last one). Keyed by the summary
// so tests can pick the exact issue they care about regardless of creation order.
func fakeJiraCapturingAll(t *testing.T, bySummary map[string]map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issue") {
			body := readBody(t, r)
			var parsed struct {
				Fields map[string]any `json:"fields"`
			}
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Errorf("unmarshal create body: %v", err)
			}
			summary, ok := parsed.Fields["summary"].(string)
			if !ok {
				t.Errorf("create body summary = %v (%T), want string", parsed.Fields["summary"], parsed.Fields["summary"])
			}
			bySummary[summary] = parsed.Fields
			w.WriteHeader(http.StatusCreated)
			writeBytes(t, w, []byte(`{"key":"TEST-1"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		writeBytes(t, w, []byte(`{}`))
	}))
}

// epicWithStoriesConfig models the DC-style config a user already has: an Epic and
// two stories that attach to it — one via an epic-typed parent, one via epicLink.
func epicWithStoriesConfig() *config.Config {
	return &config.Config{
		Defaults: config.Defaults{ProjectKey: "TEST"},
		Options:  &config.Options{},
		Issues: []config.Issue{
			{ID: "epic-1", IssueType: "Epic", Summary: "The Epic", EpicName: "Epic Display Name"},
			{ID: "story-1", IssueType: "Story", Summary: "Story via parent", Parent: "epic-1"},
			{ID: "story-2", IssueType: "Story", Summary: "Story via epicLink", EpicLink: "epic-1"},
		},
	}
}

func cloudClient(t *testing.T, url string) apply.JiraClient {
	t.Helper()
	c, err := jira.NewClientWithMode(url, jira.ModeCloud, jira.Credentials{Email: "e@x.com", Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode Cloud: %v", err)
	}
	return c
}

func dcClient(t *testing.T, url string) apply.JiraClient {
	t.Helper()
	c, err := jira.NewClientWithMode(url, jira.ModeDataCenter, jira.Credentials{Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode DC: %v", err)
	}
	return c
}

// Scenario: Stories are attached to their epic on Cloud without config changes.
// Both the epic-typed parent story AND the epicLink story must carry
// parent: {key: <epicKey>}, and neither may carry the epic-link custom field.
func TestIntegration_EpicTranslation_Cloud_StoriesCarryParent(t *testing.T) {
	t.Chdir(t.TempDir())

	payloads := make(map[string]map[string]any)
	server := fakeJiraCapturingAll(t, payloads)
	defer server.Close()

	applier := apply.NewApplier(epicWithStoriesConfig(), cloudClient(t, server.URL), jira.ModeCloud, false, false, "cfg.yaml")
	if err := applier.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	for _, summary := range []string{"Story via parent", "Story via epicLink"} {
		fields, ok := payloads[summary]
		if !ok {
			t.Fatalf("no create payload captured for %q", summary)
		}
		parent, ok := fields["parent"].(map[string]any)
		if !ok {
			t.Errorf("%s: parent field = %v (%T), want map with key", summary, fields["parent"], fields["parent"])
			continue
		}
		if parent["key"] != "TEST-1" {
			t.Errorf("%s: parent.key = %v, want TEST-1 (the created epic key)", summary, parent["key"])
		}
		if _, ok := fields[jira.DefaultEpicLinkField]; ok {
			t.Errorf("%s: epic-link custom field %s must NOT appear on Cloud", summary, jira.DefaultEpicLinkField)
		}
	}
}

// Scenario: Epic name is dropped on Cloud with a verbose notice.
func TestIntegration_EpicTranslation_Cloud_EpicNameDroppedWithVerboseNotice(t *testing.T) {
	t.Chdir(t.TempDir())

	payloads := make(map[string]map[string]any)
	server := fakeJiraCapturingAll(t, payloads)
	defer server.Close()

	cfg := &config.Config{
		Defaults: config.Defaults{ProjectKey: "TEST"},
		Options:  &config.Options{},
		Issues: []config.Issue{
			{ID: "epic-1", IssueType: "Epic", Summary: "The Epic", EpicName: "Epic Display Name"},
		},
	}
	applier := apply.NewApplier(cfg, cloudClient(t, server.URL), jira.ModeCloud, true /*verbose*/, false, "cfg.yaml")

	// Capture stderr/stdout to observe the verbose "epicName dropped" notice.
	var applyErr error
	combined := captureOutput(t, func() {
		applyErr = applier.Apply(context.Background())
	})

	if applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}

	fields, ok := payloads["The Epic"]
	if !ok {
		t.Fatalf("no create payload captured for the epic")
	}
	if _, ok := fields[jira.DefaultEpicNameField]; ok {
		t.Errorf("epic-name custom field %s must NOT appear on Cloud", jira.DefaultEpicNameField)
	}
	if !strings.Contains(strings.ToLower(combined), "epicname") {
		t.Errorf("verbose output should note the dropped epicName; got:\n%s", combined)
	}
}

// Scenario: Custom epic field settings are ignored on Cloud.
func TestIntegration_EpicTranslation_Cloud_CustomEpicFieldIDsIgnored(t *testing.T) {
	t.Chdir(t.TempDir())

	payloads := make(map[string]map[string]any)
	server := fakeJiraCapturingAll(t, payloads)
	defer server.Close()

	cfg := &config.Config{
		Defaults: config.Defaults{
			ProjectKey:    "TEST",
			EpicNameField: "customfield_77777",
			EpicLinkField: "customfield_88888",
		},
		Options: &config.Options{},
		Issues: []config.Issue{
			{ID: "epic-1", IssueType: "Epic", Summary: "Custom Epic", EpicName: "Name"},
			{ID: "story-1", IssueType: "Story", Summary: "Custom Story", EpicLink: "epic-1"},
		},
	}
	applier := apply.NewApplier(cfg, cloudClient(t, server.URL), jira.ModeCloud, false, false, "cfg.yaml")
	if err := applier.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	for summary, fields := range payloads {
		for k := range fields {
			if strings.HasPrefix(k, "customfield_") {
				t.Errorf("%s: no customfield_* epic ID may appear on Cloud, found %q", summary, k)
			}
		}
	}
}

// Scenario: Data Center epic behavior is unchanged.
func TestIntegration_EpicTranslation_DataCenter_Unchanged(t *testing.T) {
	t.Chdir(t.TempDir())

	payloads := make(map[string]map[string]any)
	server := fakeJiraCapturingAll(t, payloads)
	defer server.Close()

	applier := apply.NewApplier(epicWithStoriesConfig(), dcClient(t, server.URL), jira.ModeDataCenter, false, false, "cfg.yaml")
	if err := applier.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Epic: epicName goes to the default epic-name custom field.
	epic, ok := payloads["The Epic"]
	if !ok {
		t.Fatalf("no create payload captured for the epic")
	}
	if epic[jira.DefaultEpicNameField] != "Epic Display Name" {
		t.Errorf("DC epic-name field %s = %v, want %q", jira.DefaultEpicNameField, epic[jira.DefaultEpicNameField], "Epic Display Name")
	}

	// Story via epic-typed parent: epic-link custom field carries the epic key,
	// and the parent field is NOT set.
	sp, ok := payloads["Story via parent"]
	if !ok {
		t.Fatalf("no create payload captured for the parent-linked story")
	}
	if sp[jira.DefaultEpicLinkField] != "TEST-1" {
		t.Errorf("DC epic-link field %s = %v, want TEST-1", jira.DefaultEpicLinkField, sp[jira.DefaultEpicLinkField])
	}
	if _, ok := sp["parent"]; ok {
		t.Error("DC: parent field must NOT be set when parent is an Epic")
	}
}
