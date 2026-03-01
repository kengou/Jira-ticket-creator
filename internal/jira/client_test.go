package jira

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const testBaseURL = "https://jira.example.com"

// --- NewClient ---

func TestNewClient_DataCenter(t *testing.T) {
	c := NewClient(testBaseURL, "my-token", false)
	if c.apiVersion != "2" {
		t.Errorf("apiVersion = %q, want %q for Data Center", c.apiVersion, "2")
	}
	if c.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", c.MaxRetries)
	}
	if c.RetryDelay != time.Second {
		t.Errorf("RetryDelay = %v, want 1s", c.RetryDelay)
	}
}

func TestNewClient_Cloud(t *testing.T) {
	c := NewClient("https://jira.atlassian.net", "my-token", true)
	if c.apiVersion != "3" {
		t.Errorf("apiVersion = %q, want %q for Cloud", c.apiVersion, "3")
	}
}

func TestNewClient_WithOptions(t *testing.T) {
	c := NewClient(testBaseURL, "tok", false,
		WithMaxRetries(5),
		WithTimeout(60*time.Second),
	)
	if c.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", c.MaxRetries)
	}
	if c.httpClient.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", c.httpClient.Timeout)
	}
}

// --- normalizeURL ---

func TestNormalizeURL_AddsScheme(t *testing.T) {
	got, err := normalizeURL("jira.example.com")
	if err != nil || got != testBaseURL {
		t.Errorf("normalizeURL('jira.example.com') = %q, %v; want %q, nil", got, err, testBaseURL)
	}
}

func TestNormalizeURL_PreservesHTTPS(t *testing.T) {
	got, err := normalizeURL(testBaseURL)
	if err != nil || got != testBaseURL {
		t.Errorf("normalizeURL('https://jira.example.com') = %q, %v", got, err)
	}
}

func TestNormalizeURL_RejectsHTTPNonLoopback(t *testing.T) {
	_, err := normalizeURL("http://jira.example.com")
	if err == nil {
		t.Error("normalizeURL('http://jira.example.com') expected error, got nil")
	}
}

func TestNormalizeURL_AllowsHTTPLoopback(t *testing.T) {
	got, err := normalizeURL("http://localhost:8080")
	if err != nil || got != "http://localhost:8080" {
		t.Errorf("normalizeURL('http://localhost:8080') = %q, %v; want 'http://localhost:8080', nil", got, err)
	}
}

func TestNormalizeURL_StripsTrailingSlash(t *testing.T) {
	got, err := normalizeURL(testBaseURL + "/")
	if err != nil || got != testBaseURL {
		t.Errorf("normalizeURL('https://jira.example.com/') = %q, %v", got, err)
	}
}

func TestNormalizeURL_StripsMultipleTrailingSlashes(t *testing.T) {
	got, err := normalizeURL(testBaseURL + "///")
	if err != nil || got != testBaseURL {
		t.Errorf("normalizeURL('https://jira.example.com///') = %q, %v", got, err)
	}
}

func TestNormalizeURL_NoSchemeWithTrailingSlash(t *testing.T) {
	got, err := normalizeURL("jira.example.com/")
	if err != nil || got != testBaseURL {
		t.Errorf("normalizeURL('jira.example.com/') = %q, %v", got, err)
	}
}

func TestNormalizeURL_EmptyString(t *testing.T) {
	got, err := normalizeURL("")
	if err != nil || got != "" {
		t.Errorf("normalizeURL('') = %q, %v; want empty, nil", got, err)
	}
}

func TestNormalizeURL_WhitespaceOnly(t *testing.T) {
	got, err := normalizeURL("  ")
	if err != nil || got != "" {
		t.Errorf("normalizeURL('  ') = %q, %v; want empty, nil", got, err)
	}
}

func TestNormalizeURL_TrimsWhitespace(t *testing.T) {
	got, err := normalizeURL("  jira.example.com  ")
	if err != nil || got != testBaseURL {
		t.Errorf("normalizeURL('  jira.example.com  ') = %q, %v", got, err)
	}
}

func TestNormalizeURL_WithPath(t *testing.T) {
	got, err := normalizeURL("jira.example.com/context-path")
	if err != nil || got != "https://jira.example.com/context-path" {
		t.Errorf("normalizeURL('jira.example.com/context-path') = %q, %v", got, err)
	}
}

func TestNormalizeURL_WithPort(t *testing.T) {
	got, err := normalizeURL("jira.example.com:8443")
	if err != nil || got != "https://jira.example.com:8443" {
		t.Errorf("normalizeURL('jira.example.com:8443') = %q, %v", got, err)
	}
}

func TestNewClient_NormalizesURL(t *testing.T) {
	// Verify that NewClient applies normalizeURL to the base URL
	c := NewClient("jira.example.com/", "token", false)
	if c.baseURL != testBaseURL {
		t.Errorf("NewClient baseURL = %q, want 'https://jira.example.com'", c.baseURL)
	}
}

// --- Format helpers ---

func TestFormatIssueType(t *testing.T) {
	got := FormatIssueType("Story")
	if got["name"] != "Story" {
		t.Errorf("FormatIssueType = %v, want name=Story", got)
	}
}

func TestFormatPriority(t *testing.T) {
	got := FormatPriority("High")
	if got["name"] != "High" {
		t.Errorf("FormatPriority = %v, want name=High", got)
	}
}

func TestFormatUser(t *testing.T) {
	got := FormatUser("jdoe")
	if got["name"] != "jdoe" {
		t.Errorf("FormatUser = %v, want name=jdoe", got)
	}
}

func TestFormatComponent(t *testing.T) {
	got := FormatComponent("backend")
	if got["name"] != "backend" {
		t.Errorf("FormatComponent = %v, want name=backend", got)
	}
}

func TestFormatVersion(t *testing.T) {
	got := FormatVersion("v1.0")
	if got["name"] != "v1.0" {
		t.Errorf("FormatVersion = %v, want name=v1.0", got)
	}
}

// --- BuildIssueFields ---

func TestBuildIssueFields_SetsProject(t *testing.T) {
	fields := BuildIssueFields("PROJ", map[string]any{
		"summary": "Test",
	})

	proj, ok := fields["project"].(map[string]any)
	if !ok {
		t.Fatalf("project field not a map: %T", fields["project"])
	}
	if proj["key"] != "PROJ" {
		t.Errorf("project.key = %v, want PROJ", proj["key"])
	}
	if fields["summary"] != "Test" {
		t.Errorf("summary = %v, want Test", fields["summary"])
	}
}

func TestBuildIssueFields_PreservesAllFields(t *testing.T) {
	input := map[string]any{
		"summary":   "Title",
		"issuetype": FormatIssueType("Bug"),
		"labels":    []string{"critical"},
	}
	got := BuildIssueFields("KEY", input)

	if got["summary"] != "Title" {
		t.Error("summary not preserved")
	}
	if _, ok := got["project"]; !ok {
		t.Error("project not set")
	}
	if _, ok := got["labels"]; !ok {
		t.Error("labels not preserved")
	}
}

// --- parseRetryAfter ---

func TestParseRetryAfter_Empty(t *testing.T) {
	c := &Client{}
	if got := c.parseRetryAfter(""); got != 1 {
		t.Errorf("parseRetryAfter('') = %d, want 1", got)
	}
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	c := &Client{}
	if got := c.parseRetryAfter("30"); got != 30 {
		t.Errorf("parseRetryAfter('30') = %d, want 30", got)
	}
}

func TestParseRetryAfter_OutOfRange(t *testing.T) {
	c := &Client{}
	// Negative
	if got := c.parseRetryAfter("-5"); got != 1 {
		t.Errorf("parseRetryAfter('-5') = %d, want 1 (fallback)", got)
	}
	// Too large
	if got := c.parseRetryAfter("99999"); got != 1 {
		t.Errorf("parseRetryAfter('99999') = %d, want 1 (fallback)", got)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	c := &Client{}
	// Future date
	future := time.Now().Add(30 * time.Second).UTC().Format(time.RFC1123)
	got := c.parseRetryAfter(future)
	// Allow some tolerance (28-32 seconds)
	if got < 28 || got > 32 {
		t.Errorf("parseRetryAfter(future) = %d, want ~30", got)
	}
}

func TestParseRetryAfter_PastHTTPDate(t *testing.T) {
	c := &Client{}
	past := time.Now().Add(-10 * time.Second).UTC().Format(time.RFC1123)
	got := c.parseRetryAfter(past)
	if got != 1 {
		t.Errorf("parseRetryAfter(past) = %d, want 1 (past date)", got)
	}
}

func TestParseRetryAfter_InvalidString(t *testing.T) {
	c := &Client{}
	if got := c.parseRetryAfter("not-a-number"); got != 1 {
		t.Errorf("parseRetryAfter('not-a-number') = %d, want 1", got)
	}
}

// --- APIError ---

func TestAPIError_ErrorString(t *testing.T) {
	e := &APIError{StatusCode: 400, Body: `{"error":"bad request"}`}
	got := e.Error()
	if got != `API error (400): {"error":"bad request"}` {
		t.Errorf("Error() = %q", got)
	}
}

// --- HTTP integration tests using httptest ---

func TestCreateIssue_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/rest/api/2/issue" {
			t.Errorf("path = %s, want /rest/api/2/issue", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateIssueResponse{ //nolint:errcheck,gosec
			ID:   "10001",
			Key:  "PROJ-1",
			Self: "https://jira.example.com/rest/api/2/issue/10001",
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token", false)
	c.MaxRetries = 0

	resp, err := c.CreateIssue(&CreateIssueRequest{
		Fields: map[string]any{
			"summary":   "Test issue",
			"issuetype": FormatIssueType("Story"),
		},
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if resp.Key != "PROJ-1" {
		t.Errorf("Key = %q, want PROJ-1", resp.Key)
	}
}

func TestCreateIssue_ClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errors":{"summary":"required"}}`)) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	_, err := c.CreateIssue(&CreateIssueRequest{Fields: map[string]any{}})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
}

func TestCreateIssue_ServerError_RetriesAndFails(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error")) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 1
	c.RetryDelay = time.Millisecond // speed up test

	_, err := c.CreateIssue(&CreateIssueRequest{Fields: map[string]any{"summary": "test"}})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}

	got := atomic.LoadInt32(&attempts)
	if got != 2 { // initial + 1 retry
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestCreateIssue_ServerError_RetriesAndSucceeds(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("temporary failure")) //nolint:errcheck,gosec
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateIssueResponse{Key: "PROJ-1"}) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 2
	c.RetryDelay = time.Millisecond

	resp, err := c.CreateIssue(&CreateIssueRequest{Fields: map[string]any{"summary": "test"}})
	if err != nil {
		t.Fatalf("expected success on retry: %v", err)
	}
	if resp.Key != "PROJ-1" {
		t.Errorf("Key = %q, want PROJ-1", resp.Key)
	}
}

func TestSearchIssues_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		jql := r.URL.Query().Get("jql")
		if jql != `project = PROJ AND summary ~ "test"` {
			t.Errorf("jql = %q", jql)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SearchResult{ //nolint:errcheck,gosec
			Total: 1,
			Issues: []SearchIssue{
				{Key: "PROJ-1", Fields: map[string]any{"summary": "test"}},
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	result, err := c.SearchIssues(`project = PROJ AND summary ~ "test"`, 10)
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
	if result.Issues[0].Key != "PROJ-1" {
		t.Errorf("Key = %q, want PROJ-1", result.Issues[0].Key)
	}
}

func TestUpdateIssue_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/rest/api/2/issue/PROJ-1" {
			t.Errorf("path = %q, want /rest/api/2/issue/PROJ-1", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	err := c.UpdateIssue("PROJ-1", &UpdateIssueRequest{
		Fields: map[string]any{"summary": "updated"},
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
}

func TestCreateIssueLink_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/rest/api/2/issueLink" {
			t.Errorf("path = %q, want /rest/api/2/issueLink", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	err := c.CreateIssueLink(&IssueLinkRequest{
		Type:         IssueLinkType{Name: "Blocks"},
		InwardIssue:  IssueRef{Key: "PROJ-2"},
		OutwardIssue: IssueRef{Key: "PROJ-1"},
	})
	if err != nil {
		t.Fatalf("CreateIssueLink: %v", err)
	}
}

func TestCreateIssue_CloudUsesAPIv3(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue" {
			t.Errorf("path = %q, want /rest/api/3/issue (Cloud)", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateIssueResponse{Key: "CLOUD-1"}) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", true) // isCloud=true
	c.MaxRetries = 0

	resp, err := c.CreateIssue(&CreateIssueRequest{Fields: map[string]any{"summary": "cloud test"}})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if resp.Key != "CLOUD-1" {
		t.Errorf("Key = %q, want CLOUD-1", resp.Key)
	}
}

func TestDoRequest_RateLimit429(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateIssueResponse{Key: "PROJ-1"}) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 2
	c.RetryDelay = time.Millisecond

	resp, err := c.CreateIssue(&CreateIssueRequest{Fields: map[string]any{"summary": "test"}})
	if err != nil {
		t.Fatalf("expected success after 429 retry: %v", err)
	}

	got := atomic.LoadInt32(&attempts)
	if got < 2 {
		t.Errorf("expected at least 2 attempts, got %d", got)
	}
	_ = resp
}

// --- Benchmark parseRetryAfter ---

func BenchmarkParseRetryAfter_Seconds(b *testing.B) {
	c := &Client{}
	for range b.N {
		c.parseRetryAfter("30")
	}
}

func BenchmarkParseRetryAfter_HTTPDate(b *testing.B) {
	c := &Client{}
	future := time.Now().Add(30 * time.Second).UTC().Format(time.RFC1123)
	for range b.N {
		c.parseRetryAfter(future)
	}
}

// --- FetchEpics ---

func TestFetchEpics_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}

		jql := r.URL.Query().Get("jql")
		if jql != `project = "PROJ" AND issuetype = Epic ORDER BY created DESC` {
			t.Errorf("jql = %q", jql)
		}

		// Verify the fields parameter is set
		fields := r.URL.Query().Get("fields")
		if fields != "summary,status,description" {
			t.Errorf("fields = %q, want summary,status,description", fields)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SearchResult{ //nolint:errcheck,gosec
			Total: 2,
			Issues: []SearchIssue{
				{
					Key: "PROJ-10",
					Fields: map[string]any{
						"summary": "Epic Alpha",
						"status":  map[string]any{"name": "In Progress"},
					},
				},
				{
					Key: "PROJ-5",
					Fields: map[string]any{
						"summary": "Epic Beta",
						"status":  map[string]any{"name": "Done"},
					},
				},
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	epics, err := c.FetchEpics("PROJ", "")
	if err != nil {
		t.Fatalf("FetchEpics: %v", err)
	}
	if len(epics) != 2 {
		t.Fatalf("len(epics) = %d, want 2", len(epics))
	}

	if epics[0].Key != "PROJ-10" {
		t.Errorf("epics[0].Key = %q, want PROJ-10", epics[0].Key)
	}
	if epics[0].Summary != "Epic Alpha" {
		t.Errorf("epics[0].Summary = %q, want Epic Alpha", epics[0].Summary)
	}
	if epics[0].Status != "In Progress" {
		t.Errorf("epics[0].Status = %q, want In Progress", epics[0].Status)
	}

	if epics[1].Key != "PROJ-5" {
		t.Errorf("epics[1].Key = %q, want PROJ-5", epics[1].Key)
	}
	if epics[1].Summary != "Epic Beta" {
		t.Errorf("epics[1].Summary = %q", epics[1].Summary)
	}
	if epics[1].Status != "Done" {
		t.Errorf("epics[1].Status = %q", epics[1].Status)
	}
}

func TestFetchEpics_WithStatusFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jql := r.URL.Query().Get("jql")
		expectedJQL := `project = "PROJ" AND issuetype = Epic AND status = "In Progress" ORDER BY created DESC`
		if jql != expectedJQL {
			t.Errorf("jql = %q, want %q", jql, expectedJQL)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SearchResult{ //nolint:errcheck,gosec
			Total: 1,
			Issues: []SearchIssue{
				{
					Key: "PROJ-10",
					Fields: map[string]any{
						"summary": "Epic Alpha",
						"status":  map[string]any{"name": "In Progress"},
					},
				},
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	epics, err := c.FetchEpics("PROJ", "In Progress")
	if err != nil {
		t.Fatalf("FetchEpics: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("len(epics) = %d, want 1", len(epics))
	}
	if epics[0].Key != "PROJ-10" {
		t.Errorf("epics[0].Key = %q, want PROJ-10", epics[0].Key)
	}
	if epics[0].Status != "In Progress" {
		t.Errorf("epics[0].Status = %q, want In Progress", epics[0].Status)
	}
}

func TestFetchEpics_StatusFilterNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jql := r.URL.Query().Get("jql")
		expectedJQL := `project = "PROJ" AND issuetype = Epic AND status = "Done" ORDER BY created DESC`
		if jql != expectedJQL {
			t.Errorf("jql = %q, want %q", jql, expectedJQL)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SearchResult{Total: 0, Issues: nil}) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	epics, err := c.FetchEpics("PROJ", "Done")
	if err != nil {
		t.Fatalf("FetchEpics: %v", err)
	}
	if len(epics) != 0 {
		t.Errorf("expected 0 epics, got %d", len(epics))
	}
}

func TestFetchEpics_NegatedStatusFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jql := r.URL.Query().Get("jql")
		expectedJQL := `project = "PROJ" AND issuetype = Epic AND status != "Done" ORDER BY created DESC`
		if jql != expectedJQL {
			t.Errorf("jql = %q, want %q", jql, expectedJQL)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SearchResult{ //nolint:errcheck,gosec
			Total: 2,
			Issues: []SearchIssue{
				{
					Key: "PROJ-1",
					Fields: map[string]any{
						"summary": "Epic Alpha",
						"status":  map[string]any{"name": "To Do"},
					},
				},
				{
					Key: "PROJ-2",
					Fields: map[string]any{
						"summary": "Epic Beta",
						"status":  map[string]any{"name": "In Progress"},
					},
				},
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	epics, err := c.FetchEpics("PROJ", "NOT:Done")
	if err != nil {
		t.Fatalf("FetchEpics: %v", err)
	}
	if len(epics) != 2 {
		t.Fatalf("len(epics) = %d, want 2", len(epics))
	}
	if epics[0].Status != "To Do" {
		t.Errorf("epics[0].Status = %q, want To Do", epics[0].Status)
	}
	if epics[1].Status != "In Progress" {
		t.Errorf("epics[1].Status = %q, want In Progress", epics[1].Status)
	}
}

func TestFetchEpics_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SearchResult{Total: 0, Issues: nil}) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	epics, err := c.FetchEpics("EMPTY", "")
	if err != nil {
		t.Fatalf("FetchEpics: %v", err)
	}
	if len(epics) != 0 {
		t.Errorf("expected 0 epics, got %d", len(epics))
	}
}

func TestFetchEpics_Pagination(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := atomic.AddInt32(&requestCount, 1)
		startAt := r.URL.Query().Get("startAt")

		w.WriteHeader(http.StatusOK)

		switch {
		case page == 1 && startAt == "0":
			// First page: 50 results
			issues := make([]SearchIssue, 50)
			for i := range issues {
				issues[i] = SearchIssue{
					Key: fmt.Sprintf("PROJ-%d", i+1),
					Fields: map[string]any{
						"summary": fmt.Sprintf("Epic %d", i+1),
						"status":  map[string]any{"name": "To Do"},
					},
				}
			}
			json.NewEncoder(w).Encode(SearchResult{Total: 75, Issues: issues}) //nolint:errcheck,gosec

		case page == 2 && startAt == "50":
			// Second page: 25 results
			issues := make([]SearchIssue, 25)
			for i := range issues {
				issues[i] = SearchIssue{
					Key: fmt.Sprintf("PROJ-%d", 51+i),
					Fields: map[string]any{
						"summary": fmt.Sprintf("Epic %d", 51+i),
						"status":  map[string]any{"name": "To Do"},
					},
				}
			}
			json.NewEncoder(w).Encode(SearchResult{Total: 75, Issues: issues}) //nolint:errcheck,gosec

		default:
			t.Errorf("unexpected request: page=%d, startAt=%s", page, startAt)
			json.NewEncoder(w).Encode(SearchResult{Total: 0, Issues: nil}) //nolint:errcheck,gosec
		}
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	epics, err := c.FetchEpics("PROJ", "")
	if err != nil {
		t.Fatalf("FetchEpics: %v", err)
	}
	if len(epics) != 75 {
		t.Errorf("len(epics) = %d, want 75", len(epics))
	}

	pages := atomic.LoadInt32(&requestCount)
	if pages != 2 {
		t.Errorf("expected 2 paginated requests, got %d", pages)
	}

	// Verify first and last
	if epics[0].Key != "PROJ-1" {
		t.Errorf("first epic key = %q, want PROJ-1", epics[0].Key)
	}
	if epics[74].Key != "PROJ-75" {
		t.Errorf("last epic key = %q, want PROJ-75", epics[74].Key)
	}
}

func TestFetchEpics_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errorMessages":["invalid JQL"]}`)) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	_, err := c.FetchEpics("BAD", "")
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

func TestFetchEpics_MissingStatusField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SearchResult{ //nolint:errcheck,gosec
			Total: 1,
			Issues: []SearchIssue{
				{
					Key: "PROJ-1",
					Fields: map[string]any{
						"summary": "Epic without status",
						// no "status" field
					},
				},
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	epics, err := c.FetchEpics("PROJ", "")
	if err != nil {
		t.Fatalf("FetchEpics: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("len(epics) = %d, want 1", len(epics))
	}
	if epics[0].Status != "" {
		t.Errorf("Status = %q, want empty for missing status", epics[0].Status)
	}
	if epics[0].Summary != "Epic without status" {
		t.Errorf("Summary = %q", epics[0].Summary)
	}
}

func TestFetchEpics_CloudUsesV3(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !testing.Short() {
			// Verify path uses v3
			expectedPrefix := "/rest/api/3/search"
			if r.URL.Path != expectedPrefix {
				t.Errorf("path = %q, want %q (Cloud v3)", r.URL.Path, expectedPrefix)
			}
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SearchResult{Total: 0, Issues: nil}) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", true) // isCloud=true
	c.MaxRetries = 0

	_, err := c.FetchEpics("PROJ", "")
	if err != nil {
		t.Fatalf("FetchEpics: %v", err)
	}
}

// Test that doRequest doesn't retry on 4xx errors (except 429)
func TestDoRequest_NoRetryOn4xx(t *testing.T) {
	statusCodes := []int{400, 401, 403, 404, 409}
	for _, code := range statusCodes {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			var attempts int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&attempts, 1)
				w.WriteHeader(code)
				w.Write([]byte(`{"error":"client error"}`)) //nolint:errcheck,gosec
			}))
			defer server.Close()

			c := NewClient(server.URL, "token", false)
			c.MaxRetries = 3
			c.RetryDelay = time.Millisecond

			_, err := c.CreateIssue(&CreateIssueRequest{Fields: map[string]any{"summary": "test"}})
			if err == nil {
				t.Fatal("expected error")
			}

			got := atomic.LoadInt32(&attempts)
			if got != 1 {
				t.Errorf("4xx should not retry: attempts = %d, want 1", got)
			}
		})
	}
}

// --- FetchFields ---

func TestFetchFields_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/rest/api/2/field" {
			t.Errorf("path = %q, want /rest/api/2/field", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]Field{ //nolint:errcheck,gosec
			{ID: "summary", Name: "Summary", Custom: false},
			{ID: "customfield_10009", Name: "Epic Link", Custom: true, Schema: &FieldSchema{
				Type: "any", Custom: "com.pyxis.greenhopper.jira:gh-epic-link", CustomID: 10009,
			}},
			{ID: "customfield_10010", Name: "Epic Name", Custom: true, Schema: &FieldSchema{
				Type: "string", Custom: "com.pyxis.greenhopper.jira:gh-epic-label", CustomID: 10010,
			}},
			{ID: "priority", Name: "Priority", Custom: false},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	fields, err := c.FetchFields()
	if err != nil {
		t.Fatalf("FetchFields: %v", err)
	}
	if len(fields) != 4 {
		t.Fatalf("len(fields) = %d, want 4", len(fields))
	}

	if fields[0].ID != "summary" || fields[0].Name != "Summary" || fields[0].Custom != false {
		t.Errorf("fields[0] = %+v, unexpected", fields[0])
	}
	if fields[1].ID != "customfield_10009" || fields[1].Name != "Epic Link" || !fields[1].Custom {
		t.Errorf("fields[1] = %+v, unexpected", fields[1])
	}
	if fields[1].Schema == nil {
		t.Fatal("fields[1].Schema should not be nil")
	}
	if fields[1].Schema.Custom != "com.pyxis.greenhopper.jira:gh-epic-link" {
		t.Errorf("fields[1].Schema.Custom = %q, unexpected", fields[1].Schema.Custom)
	}
}

func TestFetchFields_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]Field{}) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	fields, err := c.FetchFields()
	if err != nil {
		t.Fatalf("FetchFields: %v", err)
	}
	if len(fields) != 0 {
		t.Errorf("len(fields) = %d, want 0", len(fields))
	}
}

func TestFetchFields_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errorMessages":["unauthorized"]}`)) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	_, err := c.FetchFields()
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestFetchFields_CloudUsesV3(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/field" {
			t.Errorf("path = %q, want /rest/api/3/field (Cloud v3)", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]Field{}) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", true) // isCloud=true
	c.MaxRetries = 0

	_, err := c.FetchFields()
	if err != nil {
		t.Fatalf("FetchFields: %v", err)
	}
}

func TestFetchFields_NoSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Some fields (like "issuekey") have no schema at all
		w.Write([]byte(`[{"id":"issuekey","name":"Key","custom":false}]`)) //nolint:errcheck,gosec
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", false)
	c.MaxRetries = 0

	fields, err := c.FetchFields()
	if err != nil {
		t.Fatalf("FetchFields: %v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("len(fields) = %d, want 1", len(fields))
	}
	if fields[0].Schema != nil {
		t.Errorf("expected nil Schema for issuekey field")
	}
}

// --- FetchIssueLinkTypes ---

func TestFetchIssueLinkTypes_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/issueLinkType" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
			"issueLinkTypes": []map[string]any{
				{"id": "10000", "name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
				{"id": "10001", "name": "Duplicate", "inward": "is duplicated by", "outward": "duplicates"},
				{"id": "10002", "name": "Relates", "inward": "relates to", "outward": "relates to"},
			},
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "tok", false)
	types, err := c.FetchIssueLinkTypes()
	if err != nil {
		t.Fatalf("FetchIssueLinkTypes: %v", err)
	}
	if len(types) != 3 {
		t.Fatalf("len(types) = %d, want 3", len(types))
	}
	if types[0].Name != "Blocks" {
		t.Errorf("types[0].Name = %q, want Blocks", types[0].Name)
	}
	if types[0].Inward != "is blocked by" {
		t.Errorf("types[0].Inward = %q, want 'is blocked by'", types[0].Inward)
	}
	if types[0].Outward != "blocks" {
		t.Errorf("types[0].Outward = %q, want 'blocks'", types[0].Outward)
	}
}

func TestFetchIssueLinkTypes_Empty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"issueLinkTypes": []any{}}) //nolint:errcheck,gosec
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "tok", false)
	types, err := c.FetchIssueLinkTypes()
	if err != nil {
		t.Fatalf("FetchIssueLinkTypes: %v", err)
	}
	if len(types) != 0 {
		t.Errorf("len(types) = %d, want 0", len(types))
	}
}

func TestFetchIssueLinkTypes_APIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errorMessages":["Forbidden"]}`)) //nolint:errcheck,gosec
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "tok", false)
	_, err := c.FetchIssueLinkTypes()
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestFetchIssueLinkTypes_CloudUsesV3(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issueLinkType" {
			t.Errorf("expected v3 path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"issueLinkTypes": []any{}}) //nolint:errcheck,gosec
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "tok", true) // isCloud = true
	_, err := c.FetchIssueLinkTypes()
	if err != nil {
		t.Fatalf("FetchIssueLinkTypes: %v", err)
	}
}
