// SPDX-License-Identifier: Apache-2.0
package apply_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kengou/jira-ticket-creator/internal/apply"
	"github.com/kengou/jira-ticket-creator/internal/config"
	"github.com/kengou/jira-ticket-creator/internal/jira"
)

// writeBytes writes data to w and fails the test if the write fails.
func writeBytes(t *testing.T, w http.ResponseWriter, data []byte) {
	t.Helper()
	if _, err := w.Write(data); err != nil {
		t.Errorf("w.Write: %v", err)
	}
}

// readBody reads and returns the full request body, failing the test on error.
func readBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("read request body: %v", err)
	}
	return body
}

// captureOutput redirects os.Stdout and os.Stderr while fn runs and returns
// everything written to either stream.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout, oldStderr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout, os.Stderr = wOut, wErr

	fn()

	if err := wOut.Close(); err != nil {
		t.Errorf("close stdout pipe: %v", err)
	}
	if err := wErr.Close(); err != nil {
		t.Errorf("close stderr pipe: %v", err)
	}
	os.Stdout, os.Stderr = oldStdout, oldStderr

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rOut); err != nil {
		t.Errorf("copy captured stdout: %v", err)
	}
	if _, err := io.Copy(&buf, rErr); err != nil {
		t.Errorf("copy captured stderr: %v", err)
	}
	return buf.String()
}

// capturedBody records the JSON body of the last create-issue request.
type capturedBody struct {
	Fields map[string]any `json:"fields"`
}

// newFakeJira returns an httptest server that records create-issue payloads and
// responds with a fixed key. The captured pointer is populated on each POST /issue.
func newFakeJira(t *testing.T, captured *capturedBody) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readBody(t, r)
		switch {
		case r.Method == http.MethodPost && pathHasSuffix(r.URL.Path, "/issue"):
			if err := json.Unmarshal(body, captured); err != nil {
				t.Errorf("unmarshal create body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			writeBytes(t, w, []byte(`{"key":"TEST-1"}`))
		default:
			w.WriteHeader(http.StatusOK)
			writeBytes(t, w, []byte(`{}`))
		}
	}))
}

func pathHasSuffix(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}

func integrationConfig() *config.Config {
	return &config.Config{
		Defaults: config.Defaults{ProjectKey: "TEST"},
		Issues: []config.Issue{
			{
				ID:          "STORY-1",
				IssueType:   "Story",
				Summary:     "DRAFT: rich text",
				Description: "h1. Title\n\nThis is *bold* text.",
			},
		},
	}
}

func TestIntegration_CloudDescriptionIsADF(t *testing.T) {
	t.Chdir(t.TempDir()) // isolate the state file written by Apply

	var captured capturedBody
	server := newFakeJira(t, &captured)
	defer server.Close()

	client, err := jira.NewClientWithMode(server.URL, jira.ModeCloud, jira.Credentials{
		Email: "test@example.com",
		Token: "token",
	})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}

	applier := apply.NewApplier(integrationConfig(), client, jira.ModeCloud, false, false, "cfg.yaml")
	if err := applier.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	desc := captured.Fields["description"]
	m, ok := desc.(map[string]any)
	if !ok {
		t.Fatalf("Cloud description should be an ADF object, got %T (%v)", desc, desc)
	}
	if m["type"] != "doc" {
		t.Errorf("ADF root type = %v, want \"doc\"", m["type"])
	}
}

func TestIntegration_DataCenterDescriptionIsString(t *testing.T) {
	t.Chdir(t.TempDir()) // isolate the state file written by Apply

	var captured capturedBody
	server := newFakeJira(t, &captured)
	defer server.Close()

	client, err := jira.NewClientWithMode(server.URL, jira.ModeDataCenter, jira.Credentials{
		Token: "token",
	})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}

	applier := apply.NewApplier(integrationConfig(), client, jira.ModeDataCenter, false, false, "cfg.yaml")
	if err := applier.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	desc := captured.Fields["description"]
	s, ok := desc.(string)
	if !ok {
		t.Fatalf("Data Center description should be a string, got %T (%v)", desc, desc)
	}
	if s != "h1. Title\n\nThis is *bold* text." {
		t.Errorf("Data Center description = %q, want the raw wiki markup unchanged", s)
	}
}
