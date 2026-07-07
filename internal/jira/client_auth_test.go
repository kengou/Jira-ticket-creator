// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClientWithMode_DataCenter_Bearer(t *testing.T) {
	var gotAuth, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
		encodeJSON(t, w, CreateIssueResponse{Key: "DC-1"})
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeDataCenter, Credentials{Token: "pat"})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}
	c.MaxRetries = 0

	if _, err := c.CreateIssue(context.Background(), &CreateIssueRequest{Fields: map[string]any{"summary": "x"}}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if gotAuth != "Bearer pat" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer pat")
	}
	if gotPath != "/rest/api/2/issue" {
		t.Errorf("path = %q, want /rest/api/2/issue", gotPath)
	}
}

func TestNewClientWithMode_Cloud_Basic(t *testing.T) {
	var gotAuth, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
		encodeJSON(t, w, CreateIssueResponse{Key: "CLOUD-1"})
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeCloud, Credentials{Email: "u@acme.com", Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}
	c.MaxRetries = 0

	if _, err := c.CreateIssue(context.Background(), &CreateIssueRequest{Fields: map[string]any{"summary": "x"}}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("u@acme.com:tok"))
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
	if gotPath != "/rest/api/3/issue" {
		t.Errorf("path = %q, want /rest/api/3/issue", gotPath)
	}
}

func TestNewClientWithMode_SetsModeAndAPIVersion(t *testing.T) {
	dc, err := NewClientWithMode(testBaseURL, ModeDataCenter, Credentials{Token: "t"})
	if err != nil {
		t.Fatalf("NewClientWithMode DC: %v", err)
	}
	if dc.mode != ModeDataCenter {
		t.Errorf("dc.mode = %v, want ModeDataCenter", dc.mode)
	}
	if dc.apiVersion != "2" {
		t.Errorf("dc.apiVersion = %q, want 2", dc.apiVersion)
	}

	cloud, err := NewClientWithMode("https://acme.atlassian.net", ModeCloud, Credentials{Email: "e", Token: "t"})
	if err != nil {
		t.Fatalf("NewClientWithMode Cloud: %v", err)
	}
	if cloud.mode != ModeCloud {
		t.Errorf("cloud.mode = %v, want ModeCloud", cloud.mode)
	}
	if cloud.apiVersion != "3" {
		t.Errorf("cloud.apiVersion = %q, want 3", cloud.apiVersion)
	}
}

// TestNewClient_LegacyWrapper_DataCenterUnchanged proves the legacy NewClient
// signature still produces byte-for-byte Data Center behavior (Bearer, v2).
func TestNewClient_LegacyWrapper_DataCenterUnchanged(t *testing.T) {
	var gotAuth, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
		encodeJSON(t, w, CreateIssueResponse{Key: "DC-1"})
	}))
	defer server.Close()

	c := mustNewClient(t, server.URL, "legacy-token", false)
	c.MaxRetries = 0

	if _, err := c.CreateIssue(context.Background(), &CreateIssueRequest{Fields: map[string]any{"summary": "x"}}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if gotAuth != "Bearer legacy-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer legacy-token")
	}
	if gotPath != "/rest/api/2/issue" {
		t.Errorf("path = %q, want /rest/api/2/issue", gotPath)
	}
}

func TestNewClientWithMode_CloudRequiresEmail(t *testing.T) {
	_, err := NewClientWithMode(testBaseURL, ModeCloud, Credentials{Token: "tok"})
	if err == nil {
		t.Fatal("expected error for Cloud mode without email")
	}
	if !strings.Contains(err.Error(), "email") {
		t.Errorf("error = %q, want it to mention email", err.Error())
	}
}

func TestNewClient_LegacyCloudReturnsError(t *testing.T) {
	_, err := NewClient(testBaseURL, "tok", true)
	if err == nil {
		t.Fatal("expected error: legacy NewClient cannot supply the email Cloud requires")
	}
}
