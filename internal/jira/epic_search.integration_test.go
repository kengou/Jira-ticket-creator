// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cloudEpicsServer serves the Cloud search/jql endpoint from a fixed sequence of
// raw JSON page bodies. Each POST/GET to a path ending in "/search/jql" returns
// the next page in order; requests carrying a nextPageToken advance the cursor.
// Any request to the legacy "/search" path (Data Center) returns 404 so a
// mis-routed Cloud implementation fails loudly.
func cloudEpicsServer(t *testing.T, pages []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/search/jql") {
			// Legacy /search (Data Center) must NOT be hit in Cloud mode.
			w.WriteHeader(http.StatusNotFound)
			writeBytes(t, w, []byte(`{"error":"legacy /search not available on Cloud"}`))
			return
		}
		// Determine which page to serve from the incoming nextPageToken.
		token := requestPageToken(t, r)
		idx := 0
		switch token {
		case "":
			idx = 0
		case "tok-1":
			idx = 1
		case "tok-2":
			idx = 2
		default:
			t.Errorf("unexpected nextPageToken %q", token)
		}
		if idx >= len(pages) {
			t.Errorf("no page for token %q (idx %d, have %d pages)", token, idx, len(pages))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		writeBytes(t, w, []byte(pages[idx]))
	}))
}

// requestPageToken extracts the nextPageToken from either the JSON POST body or
// the query string, so the helper works regardless of how the implementation
// sends the cursor.
func requestPageToken(t *testing.T, r *http.Request) string {
	t.Helper()
	if q := r.URL.Query().Get("nextPageToken"); q != "" {
		return q
	}
	if r.Body == nil {
		return ""
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("read request body: %v", err)
		return ""
	}
	if len(body) == 0 {
		return ""
	}
	var parsed struct {
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	return parsed.NextPageToken
}

// Scenario: Cloud epics listing returns all epics across multiple result pages.
// Two pages linked by nextPageToken; the second is the last (isLast true, no
// token). Every epic must appear exactly once.
func TestIntegration_CloudEpicSearch_MultiPage_AllEpicsOnce(t *testing.T) {
	pages := []string{
		`{"issues":[
			{"key":"PROJ-1","fields":{"summary":"Epic One","status":{"name":"To Do"}}},
			{"key":"PROJ-2","fields":{"summary":"Epic Two","status":{"name":"To Do"}}}
		],"nextPageToken":"tok-1","isLast":false}`,
		`{"issues":[
			{"key":"PROJ-3","fields":{"summary":"Epic Three","status":{"name":"Done"}}}
		],"isLast":true}`,
	}
	server := cloudEpicsServer(t, pages)
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeCloud, Credentials{Email: "e@x.com", Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode Cloud: %v", err)
	}
	c.MaxRetries = 0

	epics, err := c.FetchEpics(context.Background(), "PROJ", "")
	if err != nil {
		t.Fatalf("FetchEpics: %v", err)
	}

	seen := map[string]int{}
	for _, e := range epics {
		seen[e.Key]++
	}
	for _, want := range []string{"PROJ-1", "PROJ-2", "PROJ-3"} {
		if seen[want] != 1 {
			t.Errorf("epic %s appeared %d times, want exactly once", want, seen[want])
		}
	}
	if len(epics) != 3 {
		t.Errorf("len(epics) = %d, want 3 (%v)", len(epics), epics)
	}
}

// Scenario: Cloud epic descriptions are shown as plain text.
// The description arrives as an ADF object with a heading and bold text; it must
// be flattened to readable plain text with no JSON/markup artifacts.
func TestIntegration_CloudEpicSearch_ADFDescriptionFlattenedToText(t *testing.T) {
	pages := []string{
		`{"issues":[
			{"key":"PROJ-9","fields":{
				"summary":"Rich Epic",
				"status":{"name":"To Do"},
				"description":{
					"version":1,"type":"doc","content":[
						{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"Overview"}]},
						{"type":"paragraph","content":[
							{"type":"text","text":"This is "},
							{"type":"text","text":"bold","marks":[{"type":"strong"}]},
							{"type":"text","text":" text."}
						]}
					]
				}
			}}
		],"isLast":true}`,
	}
	server := cloudEpicsServer(t, pages)
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeCloud, Credentials{Email: "e@x.com", Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode Cloud: %v", err)
	}
	c.MaxRetries = 0

	epics, err := c.FetchEpics(context.Background(), "PROJ", "")
	if err != nil {
		t.Fatalf("FetchEpics: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("len(epics) = %d, want 1", len(epics))
	}

	desc := epics[0].Description
	// Must contain the flattened words and must NOT contain ADF/JSON artifacts.
	for _, want := range []string{"Overview", "This is", "bold", "text."} {
		if !strings.Contains(desc, want) {
			t.Errorf("description %q missing %q", desc, want)
		}
	}
	for _, bad := range []string{"\"type\"", "paragraph", "marks", "strong", "{"} {
		if strings.Contains(desc, bad) {
			t.Errorf("description %q contains ADF/JSON artifact %q", desc, bad)
		}
	}
}

// Scenario: Data Center epics listing is unchanged.
// The legacy /rest/api/2/search endpoint with startAt/total pagination and a
// plain-string description must behave exactly as before this change.
func TestIntegration_CloudEpicSearch_DataCenterUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/rest/api/2/search") {
			t.Errorf("Data Center must call /rest/api/2/search, got %q", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("Data Center search method = %s, want GET", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		encodeJSON(t, w, SearchResult{
			Total: 1,
			Issues: []SearchIssue{
				{Key: "DC-1", Fields: map[string]any{
					"summary":     "DC Epic",
					"status":      map[string]any{"name": "In Progress"},
					"description": "Plain string description",
				}},
			},
		})
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeDataCenter, Credentials{Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode DC: %v", err)
	}
	c.MaxRetries = 0

	epics, err := c.FetchEpics(context.Background(), "PROJ", "")
	if err != nil {
		t.Fatalf("FetchEpics: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("len(epics) = %d, want 1", len(epics))
	}
	if epics[0].Key != "DC-1" || epics[0].Summary != "DC Epic" ||
		epics[0].Status != "In Progress" || epics[0].Description != "Plain string description" {
		t.Errorf("DC epic = %+v, want {DC-1 DC Epic In Progress Plain string description}", epics[0])
	}
}
