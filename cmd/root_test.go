package cmd

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kengou/jira-ticket-creator/internal/jira"
)

// --- maskToken ---

func TestMaskToken(t *testing.T) {
	cases := []struct {
		token string
		want  string
	}{
		{"", "****"},
		{"abc", "****"},
		{"abcd", "****"},
		{"abcde", "****"},                // 5 chars < 12, fully masked
		{"abcdefgh1234", "****1234"},     // exactly 12, show last 4
		{"abcdefgh12345678", "****5678"}, // 16 chars, show last 4
	}
	for _, tc := range cases {
		if got := maskToken(tc.token); got != tc.want {
			t.Errorf("maskToken(%q) = %q, want %q", tc.token, got, tc.want)
		}
	}
}

// --- validateFlags ---

func TestValidateFlags_MissingConfigFile(t *testing.T) {
	// Save and restore the global
	orig := configFile
	defer func() { configFile = orig }()

	configFile = ""
	err := validateFlags(false)
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestValidateFlags_NoAuthRequired(t *testing.T) {
	orig := configFile
	defer func() { configFile = orig }()

	configFile = "config.yaml"
	err := validateFlags(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFlags_AuthRequired_MissingURL(t *testing.T) {
	origCfg := configFile
	origURL := jiraURL
	origToken := jiraToken
	t.Setenv("JIRA_URL", "")
	t.Setenv("JIRA_TOKEN", "")
	defer func() {
		configFile = origCfg
		jiraURL = origURL
		jiraToken = origToken
	}()

	configFile = "config.yaml"
	jiraURL = ""
	jiraToken = "token"

	err := validateFlags(true)
	if err == nil {
		t.Fatal("expected error for missing Jira URL")
	}
}

func TestValidateFlags_AuthRequired_MissingToken(t *testing.T) {
	origCfg := configFile
	origURL := jiraURL
	origToken := jiraToken
	t.Setenv("JIRA_URL", "")
	t.Setenv("JIRA_TOKEN", "")
	defer func() {
		configFile = origCfg
		jiraURL = origURL
		jiraToken = origToken
	}()

	configFile = "config.yaml"
	jiraURL = "https://jira.example.com"
	jiraToken = ""

	err := validateFlags(true)
	if err == nil {
		t.Fatal("expected error for missing Jira token")
	}
}

func TestValidateFlags_AuthRequired_AllPresent(t *testing.T) {
	origCfg := configFile
	origURL := jiraURL
	origToken := jiraToken
	t.Setenv("JIRA_URL", "")
	t.Setenv("JIRA_TOKEN", "")
	defer func() {
		configFile = origCfg
		jiraURL = origURL
		jiraToken = origToken
	}()

	configFile = "config.yaml"
	jiraURL = "https://jira.example.com"
	jiraToken = "my-token"

	err := validateFlags(true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- email precedence and mode resolution ---

func resetAuthGlobals(t *testing.T) {
	t.Helper()
	origURL, origToken, origEmail, origCloud := jiraURL, jiraToken, jiraEmail, isCloud
	origMode := resolvedMode
	origAutoDetected := modeAutoDetected
	t.Cleanup(func() {
		jiraURL, jiraToken, jiraEmail, isCloud = origURL, origToken, origEmail, origCloud
		resolvedMode = origMode
		modeAutoDetected = origAutoDetected
	})
}

func TestRequireAuth_EmailFlagWinsOverEnv(t *testing.T) {
	resetAuthGlobals(t)
	t.Setenv("JIRA_URL", "")
	t.Setenv("JIRA_TOKEN", "")
	t.Setenv("JIRA_EMAIL", "env@example.com")
	t.Setenv("JIRA_CLOUD", "")

	jiraURL = "https://jira.company.com"
	jiraToken = "tok"
	jiraEmail = "flag@example.com"
	isCloud = false

	if err := requireAuth(); err != nil {
		t.Fatalf("requireAuth: %v", err)
	}
	if jiraEmail != "flag@example.com" {
		t.Errorf("jiraEmail = %q, want flag@example.com (flag wins)", jiraEmail)
	}
}

func TestRequireAuth_EmailEnvUsedWhenFlagEmpty(t *testing.T) {
	resetAuthGlobals(t)
	t.Setenv("JIRA_URL", "")
	t.Setenv("JIRA_TOKEN", "")
	t.Setenv("JIRA_EMAIL", "env@example.com")
	t.Setenv("JIRA_CLOUD", "")

	jiraURL = "https://acme.atlassian.net"
	jiraToken = "tok"
	jiraEmail = ""
	isCloud = false

	if err := requireAuth(); err != nil {
		t.Fatalf("requireAuth: %v", err)
	}
	if jiraEmail != "env@example.com" {
		t.Errorf("jiraEmail = %q, want env@example.com (env fallback)", jiraEmail)
	}
}

func TestRequireAuth_CloudAutoDetectedMissingEmailFailsFast(t *testing.T) {
	resetAuthGlobals(t)
	t.Setenv("JIRA_URL", "")
	t.Setenv("JIRA_TOKEN", "")
	t.Setenv("JIRA_EMAIL", "")
	t.Setenv("JIRA_CLOUD", "")

	jiraURL = "https://acme.atlassian.net" // auto-detects Cloud
	jiraToken = "tok"
	jiraEmail = ""
	isCloud = false

	err := requireAuth()
	if err == nil {
		t.Fatal("expected fail-fast error for Cloud without email")
	}
	if !strings.Contains(err.Error(), "JIRA_EMAIL") {
		t.Errorf("error = %q, want it to name JIRA_EMAIL", err.Error())
	}
}

func TestRequireAuth_CloudViaEnvOverrideMissingEmailFailsFast(t *testing.T) {
	resetAuthGlobals(t)
	t.Setenv("JIRA_URL", "")
	t.Setenv("JIRA_TOKEN", "")
	t.Setenv("JIRA_EMAIL", "")
	t.Setenv("JIRA_CLOUD", "true") // forces Cloud on a non-atlassian host

	jiraURL = "https://jira.company.com"
	jiraToken = "tok"
	jiraEmail = ""
	isCloud = false

	err := requireAuth()
	if err == nil {
		t.Fatal("expected fail-fast error for Cloud (env override) without email")
	}
	if resolvedMode != jira.ModeCloud {
		t.Errorf("resolvedMode = %v, want ModeCloud (JIRA_CLOUD override)", resolvedMode)
	}
}

func TestRequireAuth_CloudWithEmailSucceeds(t *testing.T) {
	resetAuthGlobals(t)
	t.Setenv("JIRA_URL", "")
	t.Setenv("JIRA_TOKEN", "")
	t.Setenv("JIRA_EMAIL", "")
	t.Setenv("JIRA_CLOUD", "")

	jiraURL = "https://acme.atlassian.net"
	jiraToken = "tok"
	jiraEmail = "user@acme.com"
	isCloud = false

	if err := requireAuth(); err != nil {
		t.Fatalf("requireAuth: %v", err)
	}
	if resolvedMode != jira.ModeCloud {
		t.Errorf("resolvedMode = %v, want ModeCloud", resolvedMode)
	}
}

func TestRequireAuth_DataCenterNoEmailRequired(t *testing.T) {
	resetAuthGlobals(t)
	t.Setenv("JIRA_URL", "")
	t.Setenv("JIRA_TOKEN", "")
	t.Setenv("JIRA_EMAIL", "")
	t.Setenv("JIRA_CLOUD", "")

	jiraURL = "https://jira.company.com"
	jiraToken = "tok"
	jiraEmail = ""
	isCloud = false

	if err := requireAuth(); err != nil {
		t.Fatalf("requireAuth (Data Center, no email): %v", err)
	}
	if resolvedMode != jira.ModeDataCenter {
		t.Errorf("resolvedMode = %v, want ModeDataCenter", resolvedMode)
	}
}

func TestNewJiraClient_CloudUsesResolvedMode(t *testing.T) {
	resetAuthGlobals(t)

	var gotAuth, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
		if _, err := w.Write([]byte(`{"key":"CLOUD-1"}`)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer server.Close()

	jiraURL = server.URL
	jiraToken = "tok"
	jiraEmail = "user@acme.com"
	resolvedMode = jira.ModeCloud

	c, err := newJiraClient()
	if err != nil {
		t.Fatalf("newJiraClient: %v", err)
	}
	c.MaxRetries = 0

	if _, err := c.CreateIssue(context.Background(), &jira.CreateIssueRequest{Fields: map[string]any{"summary": "x"}}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user@acme.com:tok"))
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
	if gotPath != "/rest/api/3/issue" {
		t.Errorf("path = %q, want /rest/api/3/issue", gotPath)
	}
}
