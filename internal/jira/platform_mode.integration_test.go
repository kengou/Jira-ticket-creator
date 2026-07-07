// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

// boolPtr returns a pointer to b, for passing explicit ResolveMode overrides.
func boolPtr(b bool) *bool { return &b }

// TestIntegration_Mode_AutoDetectAndOverride realizes the @mode scenario:
// atlassian.net auto-detects Cloud, company-hosted auto-detects Data Center,
// and an explicit override forces Cloud on a custom domain.
func TestIntegration_Mode_AutoDetectAndOverride(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		override *bool
		wantMode Mode
		wantAuto bool
	}{
		{"atlassian_net_auto_cloud", "https://acme.atlassian.net", nil, ModeCloud, true},
		{"company_hosted_auto_dc", "https://jira.company.com", nil, ModeDataCenter, true},
		{"override_forces_cloud", "https://jira.company.com", boolPtr(true), ModeCloud, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode, auto := ResolveMode(tc.url, tc.override)
			if mode != tc.wantMode {
				t.Errorf("ResolveMode(%q) mode = %v, want %v", tc.url, mode, tc.wantMode)
			}
			if auto != tc.wantAuto {
				t.Errorf("ResolveMode(%q) autoDetected = %v, want %v", tc.url, auto, tc.wantAuto)
			}
		})
	}
}

// TestIntegration_Auth_CloudBasic realizes the @auth scenario for Cloud:
// Cloud sends Authorization: Basic base64(email:token) to a v3 path.
func TestIntegration_Auth_CloudBasic(t *testing.T) {
	var gotAuth, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
		encodeJSON(t, w, CreateIssueResponse{Key: "CLOUD-1"})
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeCloud, Credentials{Email: "user@acme.com", Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}
	c.MaxRetries = 0

	if _, err := c.CreateIssue(context.Background(), &CreateIssueRequest{Fields: map[string]any{"summary": "x"}}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user@acme.com:tok"))
	if gotAuth != wantAuth {
		t.Errorf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
	if gotPath != "/rest/api/3/issue" {
		t.Errorf("path = %q, want /rest/api/3/issue", gotPath)
	}
}

// TestIntegration_Auth_DataCenterBearer realizes the @auth scenario for Data
// Center: Data Center sends Authorization: Bearer <token> to a v2 path.
func TestIntegration_Auth_DataCenterBearer(t *testing.T) {
	var gotAuth, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
		encodeJSON(t, w, CreateIssueResponse{Key: "DC-1"})
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeDataCenter, Credentials{Token: "pat-token"})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}
	c.MaxRetries = 0

	if _, err := c.CreateIssue(context.Background(), &CreateIssueRequest{Fields: map[string]any{"summary": "x"}}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	if gotAuth != "Bearer pat-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer pat-token")
	}
	if gotPath != "/rest/api/2/issue" {
		t.Errorf("path = %q, want /rest/api/2/issue", gotPath)
	}
}
