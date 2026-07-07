// SPDX-License-Identifier: Apache-2.0
package apply_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/kengou/jira-ticket-creator/internal/apply"
	"github.com/kengou/jira-ticket-creator/internal/config"
	"github.com/kengou/jira-ticket-creator/internal/jira"
)

// userSearchServer is a fake Jira that serves the Cloud user-search endpoint and
// captures every create-issue payload. matches maps a query (email) to the list
// of account IDs the search returns for it; a query with no entry returns an
// empty array (zero matches). searchCounts records how many times each query was
// searched, so tests can assert the per-run cache (one lookup per distinct email).
type userSearchServer struct {
	mu           sync.Mutex
	matches      map[string][]string       // email -> accountIds
	searchCounts map[string]int            // email -> number of search calls
	payloads     map[string]map[string]any // issue summary -> create fields
}

func fakeJiraWithUsers(t *testing.T, matches map[string][]string) *userSearchServer {
	t.Helper()
	return &userSearchServer{
		matches:      matches,
		searchCounts: make(map[string]int),
		payloads:     make(map[string]map[string]any),
	}
}

func (s *userSearchServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/user/search") {
			query := r.URL.Query().Get("query")
			s.mu.Lock()
			s.searchCounts[query]++
			ids := s.matches[query]
			s.mu.Unlock()

			users := make([]map[string]any, 0, len(ids))
			for _, id := range ids {
				users = append(users, map[string]any{
					"accountId":    id,
					"emailAddress": query,
					"displayName":  id,
				})
			}
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(users); err != nil {
				t.Errorf("encode user-search response: %v", err)
			}
			return
		}
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
			s.mu.Lock()
			s.payloads[summary] = parsed.Fields
			s.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			writeBytes(t, w, []byte(`{"key":"TEST-1"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		writeBytes(t, w, []byte(`{}`))
	}))
	return srv
}

func (s *userSearchServer) searchCount(email string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.searchCounts[email]
}

func (s *userSearchServer) payload(summary string) (map[string]any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.payloads[summary]
	return f, ok
}

// Scenario: Email assignee is resolved to the matching Cloud user.
func TestIntegration_UserResolution_Cloud_EmailAssigneeResolved(t *testing.T) {
	t.Chdir(t.TempDir())

	fake := fakeJiraWithUsers(t, map[string][]string{
		"alice@example.com": {"acc-alice"},
	})
	server := fake.start(t)
	defer server.Close()

	cfg := &config.Config{
		Defaults: config.Defaults{ProjectKey: "TEST"},
		Options:  &config.Options{},
		Issues: []config.Issue{
			{ID: "story-1", IssueType: "Story", Summary: "Assigned story", Assignee: "alice@example.com"},
		},
	}
	applier := apply.NewApplier(cfg, cloudClient(t, server.URL), jira.ModeCloud, false, false, "cfg.yaml")
	if err := applier.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	fields, ok := fake.payload("Assigned story")
	if !ok {
		t.Fatalf("no create payload captured for the story")
	}
	assignee, ok := fields["assignee"].(map[string]any)
	if !ok {
		t.Fatalf("assignee = %v (%T), want map with id", fields["assignee"], fields["assignee"])
	}
	if assignee["id"] != "acc-alice" {
		t.Errorf("assignee.id = %v, want acc-alice", assignee["id"])
	}
	if _, hasName := assignee["name"]; hasName {
		t.Error("Cloud assignee must NOT carry a name field")
	}
	if got := fake.searchCount("alice@example.com"); got != 1 {
		t.Errorf("searchCount(alice) = %d, want 1", got)
	}
}

// Scenario: Account-ID-shaped assignee passes through unchanged (no lookup).
func TestIntegration_UserResolution_Cloud_AccountIDPassthroughNoLookup(t *testing.T) {
	t.Chdir(t.TempDir())

	fake := fakeJiraWithUsers(t, map[string][]string{})
	server := fake.start(t)
	defer server.Close()

	cfg := &config.Config{
		Defaults: config.Defaults{ProjectKey: "TEST"},
		Options:  &config.Options{},
		Issues: []config.Issue{
			{ID: "story-1", IssueType: "Story", Summary: "AccountID story", Assignee: "5b10ac8d82e05b22cc7d4ef5"},
		},
	}
	applier := apply.NewApplier(cfg, cloudClient(t, server.URL), jira.ModeCloud, false, false, "cfg.yaml")
	if err := applier.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	fields, ok := fake.payload("AccountID story")
	if !ok {
		t.Fatalf("no create payload captured")
	}
	assignee, ok := fields["assignee"].(map[string]any)
	if !ok {
		t.Fatalf("assignee = %v (%T), want map with id", fields["assignee"], fields["assignee"])
	}
	if assignee["id"] != "5b10ac8d82e05b22cc7d4ef5" {
		t.Errorf("assignee.id = %v, want the raw account ID", assignee["id"])
	}
	if got := fake.searchCount("5b10ac8d82e05b22cc7d4ef5"); got != 0 {
		t.Errorf("account-ID passthrough must NOT trigger a lookup; searchCount = %d, want 0", got)
	}
}

// Scenario: Repeated email across issues triggers exactly one lookup per run.
func TestIntegration_UserResolution_Cloud_RepeatedEmailLookedUpOnce(t *testing.T) {
	t.Chdir(t.TempDir())

	fake := fakeJiraWithUsers(t, map[string][]string{
		"bob@example.com": {"acc-bob"},
	})
	server := fake.start(t)
	defer server.Close()

	cfg := &config.Config{
		Defaults: config.Defaults{ProjectKey: "TEST"},
		Options:  &config.Options{},
		Issues: []config.Issue{
			{ID: "s1", IssueType: "Story", Summary: "One", Assignee: "bob@example.com"},
			{ID: "s2", IssueType: "Story", Summary: "Two", Assignee: "bob@example.com"},
			{ID: "s3", IssueType: "Story", Summary: "Three", Assignee: "bob@example.com"},
		},
	}
	applier := apply.NewApplier(cfg, cloudClient(t, server.URL), jira.ModeCloud, false, false, "cfg.yaml")
	if err := applier.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := fake.searchCount("bob@example.com"); got != 1 {
		t.Errorf("repeated email should be looked up once; searchCount = %d, want 1", got)
	}
}

// Scenario Outline: Unresolvable assignee (not found / ambiguous) fails only that
// issue with a clear error; continue-on-error still creates the others.
func TestIntegration_UserResolution_Cloud_UnresolvableFailsOnlyThatIssue(t *testing.T) {
	cases := []struct {
		name       string
		email      string
		matches    []string // account IDs returned for the email
		wantErrSub string
	}{
		{name: "not found", email: "ghost@example.com", matches: nil, wantErrSub: "not found"},
		{name: "ambiguous", email: "dup@example.com", matches: []string{"acc-1", "acc-2"}, wantErrSub: "ambiguous"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			fake := fakeJiraWithUsers(t, map[string][]string{
				"good@example.com": {"acc-good"},
				tc.email:           tc.matches,
			})
			server := fake.start(t)
			defer server.Close()

			cfg := &config.Config{
				Defaults: config.Defaults{ProjectKey: "TEST"},
				Options:  &config.Options{ContinueOnError: true},
				Issues: []config.Issue{
					{ID: "first", IssueType: "Story", Summary: "First", Assignee: "good@example.com"},
					{ID: "second", IssueType: "Story", Summary: "Second", Assignee: tc.email},
					{ID: "third", IssueType: "Story", Summary: "Third", Assignee: "good@example.com"},
				},
			}
			applier := apply.NewApplier(cfg, cloudClient(t, server.URL), jira.ModeCloud, false, false, "cfg.yaml")

			// Capture stdout+stderr so we can assert the per-issue failure notice.
			// With continueOnError, Apply returns nil overall and skips the failing issue.
			var applyErr error
			captured := captureOutput(t, func() {
				applyErr = applier.Apply(context.Background())
			})

			if applyErr != nil {
				t.Fatalf("Apply with continueOnError should not return a top-level error: %v", applyErr)
			}

			// The per-issue failure for the unresolvable assignee must surface the
			// reason ("not found" / "ambiguous") in the printed output.
			if !strings.Contains(captured, tc.wantErrSub) {
				t.Errorf("output should mention %q for the %s assignee; got:\n%s", tc.wantErrSub, tc.name, captured)
			}

			// First and third are created; second is not.
			if _, ok := fake.payload("First"); !ok {
				t.Error("First issue should have been created")
			}
			if _, ok := fake.payload("Third"); !ok {
				t.Error("Third issue should have been created")
			}
			if _, ok := fake.payload("Second"); ok {
				t.Errorf("Second issue must NOT be created when its assignee is %s", tc.name)
			}
		})
	}
}

// Scenario: Data Center keeps username-based assignment (no lookup, {"name": ...}).
func TestIntegration_UserResolution_DataCenter_UsernameUnchanged(t *testing.T) {
	t.Chdir(t.TempDir())

	fake := fakeJiraWithUsers(t, map[string][]string{})
	server := fake.start(t)
	defer server.Close()

	cfg := &config.Config{
		Defaults: config.Defaults{ProjectKey: "TEST"},
		Options:  &config.Options{},
		Issues: []config.Issue{
			{ID: "story-1", IssueType: "Story", Summary: "DC story", Assignee: "jdoe", Reporter: "jsmith"},
		},
	}
	applier := apply.NewApplier(cfg, dcClient(t, server.URL), jira.ModeDataCenter, false, false, "cfg.yaml")
	if err := applier.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	fields, ok := fake.payload("DC story")
	if !ok {
		t.Fatalf("no create payload captured")
	}
	assignee, ok := fields["assignee"].(map[string]any)
	if !ok || assignee["name"] != "jdoe" {
		t.Errorf("DC assignee = %v, want {\"name\":\"jdoe\"}", fields["assignee"])
	}
	reporter, ok := fields["reporter"].(map[string]any)
	if !ok || reporter["name"] != "jsmith" {
		t.Errorf("DC reporter = %v, want {\"name\":\"jsmith\"}", fields["reporter"])
	}
	if got := fake.searchCount("jdoe"); got != 0 {
		t.Errorf("Data Center must NOT perform user search; searchCount(jdoe) = %d, want 0", got)
	}
}

// Scenario: Dry-run performs no user-search requests.
func TestIntegration_UserResolution_Cloud_DryRunNoLookup(t *testing.T) {
	t.Chdir(t.TempDir())

	fake := fakeJiraWithUsers(t, map[string][]string{
		"alice@example.com": {"acc-alice"},
	})
	server := fake.start(t)
	defer server.Close()

	cfg := &config.Config{
		Defaults: config.Defaults{ProjectKey: "TEST"},
		Options:  &config.Options{},
		Issues: []config.Issue{
			{ID: "story-1", IssueType: "Story", Summary: "Dry story", Assignee: "alice@example.com"},
		},
	}
	applier := apply.NewApplier(cfg, cloudClient(t, server.URL), jira.ModeCloud, false, true /*dryRun*/, "cfg.yaml")
	if err := applier.Apply(context.Background()); err != nil {
		t.Fatalf("Apply (dry-run): %v", err)
	}
	if got := fake.searchCount("alice@example.com"); got != 0 {
		t.Errorf("dry-run must perform no user search; searchCount = %d, want 0", got)
	}
	// jira.User is referenced so the import is exercised; asserts the type exists.
	var _ jira.User
}
