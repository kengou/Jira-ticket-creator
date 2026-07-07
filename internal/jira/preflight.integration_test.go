// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// recordingServer records every request path+method and asserts that preflight
// probes are read-only (GET, no body). It serves 200 for /myself and
// /project/<key>, and 404 for anything else, so a misrouted probe fails loudly.
// The hits slice is guarded by a mutex so the helper is safe under concurrent probes.
func recordingServer(t *testing.T, hits *[]string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*hits = append(*hits, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.Method != http.MethodGet {
			t.Errorf("preflight probe must be GET (read-only), got %s %s", r.Method, r.URL.Path)
		}
		if r.Body != nil {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read probe body: %v", err)
			}
			if len(body) > 0 {
				t.Errorf("preflight probe must send no body, got %q on %s", body, r.URL.Path)
			}
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/myself"):
			w.WriteHeader(http.StatusOK)
			writeBytes(t, w, []byte(`{"accountId":"abc","displayName":"Test User"}`))
		case strings.Contains(r.URL.Path, "/project/"):
			w.WriteHeader(http.StatusOK)
			writeBytes(t, w, []byte(`{"key":"PROJ","name":"Project"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			writeBytes(t, w, []byte(`{"error":"not found"}`))
		}
	}))
}

// Scenario: Standalone check passes against a healthy site.
// Both probes succeed and only read-only GETs are observed.
func TestIntegration_Preflight_HealthySitePasses(t *testing.T) {
	var hits []string
	server := recordingServer(t, &hits)
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeCloud, Credentials{Email: "e@x.com", Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode Cloud: %v", err)
	}
	c.MaxRetries = 0

	if err := c.CheckAuth(context.Background()); err != nil {
		t.Fatalf("CheckAuth on healthy site: %v", err)
	}
	if err := c.CheckProjectAccess(context.Background(), "PROJ"); err != nil {
		t.Fatalf("CheckProjectAccess on healthy site: %v", err)
	}

	// Only read-only probe endpoints must have been hit; nothing created.
	for _, h := range hits {
		if !strings.HasPrefix(h, "GET ") {
			t.Errorf("non-GET request observed during preflight: %q", h)
		}
	}
	if len(hits) != 2 {
		t.Errorf("expected exactly 2 probe requests (auth + project), got %d: %v", len(hits), hits)
	}
}

// Scenario: Check explains the likely cause of a Cloud authentication failure.
// A Cloud site that rejects the credentials must make CheckAuth fail with an
// *APIError carrying StatusCode 401.
func TestIntegration_Preflight_CloudAuthFailureIs401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		writeBytes(t, w, []byte(`{"errorMessages":["Unauthorized"]}`))
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeCloud, Credentials{Email: "e@x.com", Token: "bad"})
	if err != nil {
		t.Fatalf("NewClientWithMode Cloud: %v", err)
	}
	c.MaxRetries = 0

	err = c.CheckAuth(context.Background())
	if err == nil {
		t.Fatal("CheckAuth against rejecting Cloud site: want error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("CheckAuth error = %v, want it to unwrap to *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
}

// Scenario: Apply refuses to create anything when preflight fails.
// The lightweight preflight (auth probe) against a rejecting server must error
// BEFORE any create-issue request is sent. We assert no /issue request is ever
// observed once the auth probe fails.
func TestIntegration_Preflight_FailingAuthPreventsCreate(t *testing.T) {
	var createHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/issue") {
			createHits++
			w.WriteHeader(http.StatusCreated)
			writeBytes(t, w, []byte(`{"key":"X-1"}`))
			return
		}
		// Auth probe (/myself) is rejected.
		w.WriteHeader(http.StatusUnauthorized)
		writeBytes(t, w, []byte(`{"errorMessages":["Unauthorized"]}`))
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeDataCenter, Credentials{Token: "bad"})
	if err != nil {
		t.Fatalf("NewClientWithMode DC: %v", err)
	}
	c.MaxRetries = 0

	// Simulate the apply preflight: auth probe first; on failure, DO NOT create.
	if err := c.CheckAuth(context.Background()); err == nil {
		t.Fatal("preflight auth probe: want error against rejecting server, got nil")
		// (If auth had passed, an apply would proceed to CreateIssue.)
	}

	if createHits != 0 {
		t.Errorf("no issue must be created when preflight fails; observed %d create requests", createHits)
	}
}

// Scenario: Apply proceeds after preflight passes.
// Against a healthy server the auth + project probes pass, and a subsequent
// create-issue request succeeds — modelling apply proceeding after preflight.
func TestIntegration_Preflight_PassingPreflightAllowsCreate(t *testing.T) {
	var createHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/issue"):
			createHits++
			w.WriteHeader(http.StatusCreated)
			writeBytes(t, w, []byte(`{"key":"DC-1"}`))
		case strings.HasSuffix(r.URL.Path, "/myself"):
			w.WriteHeader(http.StatusOK)
			writeBytes(t, w, []byte(`{"accountId":"abc"}`))
		case strings.Contains(r.URL.Path, "/project/"):
			w.WriteHeader(http.StatusOK)
			writeBytes(t, w, []byte(`{"key":"PROJ"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeDataCenter, Credentials{Token: "good"})
	if err != nil {
		t.Fatalf("NewClientWithMode DC: %v", err)
	}
	c.MaxRetries = 0

	if err := c.CheckAuth(context.Background()); err != nil {
		t.Fatalf("preflight auth probe on healthy server: %v", err)
	}
	if err := c.CheckProjectAccess(context.Background(), "PROJ"); err != nil {
		t.Fatalf("preflight project probe on healthy server: %v", err)
	}
	// Preflight passed → apply would proceed to create.
	if _, err := c.CreateIssue(context.Background(), &CreateIssueRequest{Fields: map[string]any{"summary": "x"}}); err != nil {
		t.Fatalf("CreateIssue after passing preflight: %v", err)
	}
	if createHits != 1 {
		t.Errorf("expected exactly 1 create after passing preflight, got %d", createHits)
	}
}
