// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckAuth_Cloud_SuccessHitsMyselfV3(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
		writeBytes(t, w, []byte(`{"accountId":"abc"}`))
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeCloud, Credentials{Email: "e@x.com", Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}
	c.MaxRetries = 0

	if err := c.CheckAuth(context.Background()); err != nil {
		t.Fatalf("CheckAuth: %v", err)
	}
	if gotPath != "/rest/api/3/myself" {
		t.Errorf("path = %q, want /rest/api/3/myself", gotPath)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
}

func TestCheckAuth_DataCenter_SuccessHitsMyselfV2(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		writeBytes(t, w, []byte(`{"name":"user"}`))
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeDataCenter, Credentials{Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}
	c.MaxRetries = 0

	if err := c.CheckAuth(context.Background()); err != nil {
		t.Fatalf("CheckAuth: %v", err)
	}
	if gotPath != "/rest/api/2/myself" {
		t.Errorf("path = %q, want /rest/api/2/myself", gotPath)
	}
}

func TestCheckAuth_401ReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		writeBytes(t, w, []byte(`{"errorMessages":["Unauthorized"]}`))
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeCloud, Credentials{Email: "e@x.com", Token: "bad"})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}
	c.MaxRetries = 0

	err = c.CheckAuth(context.Background())
	if err == nil {
		t.Fatal("CheckAuth: want error on 401, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
}

func TestCheckProjectAccess_SuccessHitsProjectEndpoint(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
		writeBytes(t, w, []byte(`{"key":"PROJ"}`))
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeCloud, Credentials{Email: "e@x.com", Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}
	c.MaxRetries = 0

	if err := c.CheckProjectAccess(context.Background(), "PROJ"); err != nil {
		t.Fatalf("CheckProjectAccess: %v", err)
	}
	if gotPath != "/rest/api/3/project/PROJ" {
		t.Errorf("path = %q, want /rest/api/3/project/PROJ", gotPath)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
}

func TestCheckProjectAccess_DataCenter_SuccessHitsProjectV2(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
		writeBytes(t, w, []byte(`{"key":"PROJ"}`))
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeDataCenter, Credentials{Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}
	c.MaxRetries = 0

	if err := c.CheckProjectAccess(context.Background(), "PROJ"); err != nil {
		t.Fatalf("CheckProjectAccess: %v", err)
	}
	if gotPath != "/rest/api/2/project/PROJ" {
		t.Errorf("path = %q, want /rest/api/2/project/PROJ", gotPath)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
}

func TestCheckProjectAccess_404ReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		writeBytes(t, w, []byte(`{"errorMessages":["No project could be found"]}`))
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeDataCenter, Credentials{Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}
	c.MaxRetries = 0

	err = c.CheckProjectAccess(context.Background(), "NOPE")
	if err == nil {
		t.Fatal("CheckProjectAccess: want error on 404, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}

func TestCheckProjectAccess_InvalidKeyRejectedBeforeNetwork(t *testing.T) {
	var hit bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c, err := NewClientWithMode(server.URL, ModeDataCenter, Credentials{Token: "tok"})
	if err != nil {
		t.Fatalf("NewClientWithMode: %v", err)
	}
	c.MaxRetries = 0

	err = c.CheckProjectAccess(context.Background(), "not-a-key")
	if err == nil {
		t.Fatal("CheckProjectAccess: want error for invalid key, got nil")
	}
	if !strings.Contains(err.Error(), "invalid project key") {
		t.Errorf("error = %q, want it to mention invalid project key", err.Error())
	}
	if hit {
		t.Error("invalid project key must be rejected before any network call")
	}
}

func TestMode_Accessor(t *testing.T) {
	cloud, err := NewClientWithMode(testBaseURL, ModeCloud, Credentials{Email: "e@x.com", Token: "t"})
	if err != nil {
		t.Fatalf("NewClientWithMode Cloud: %v", err)
	}
	if cloud.Mode() != ModeCloud {
		t.Errorf("Mode() = %v, want ModeCloud", cloud.Mode())
	}

	dc, err := NewClientWithMode(testBaseURL, ModeDataCenter, Credentials{Token: "t"})
	if err != nil {
		t.Fatalf("NewClientWithMode DC: %v", err)
	}
	if dc.Mode() != ModeDataCenter {
		t.Errorf("Mode() = %v, want ModeDataCenter", dc.Mode())
	}
}
