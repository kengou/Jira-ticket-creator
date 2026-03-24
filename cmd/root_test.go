package cmd

import (
	"testing"
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
