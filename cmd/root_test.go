package cmd

import (
	"testing"
)

// --- maskToken ---

func TestMaskToken(t *testing.T) {
	cases := []string{"", "abc", "abcd", "abcde", "abcdefgh12345678"}
	for _, token := range cases {
		if got := maskToken(token); got != "****" {
			t.Errorf("maskToken(%q) = %q, want %q", token, got, "****")
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
