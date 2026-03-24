// SPDX-License-Identifier: Apache-2.0
//
// Package cmd implements the CLI commands for jira-ai-creator.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kengou/Jira-ticket-creator/internal/jira"
)

var (
	// Global flags
	configFile string
	jiraURL    string
	jiraToken  string
	verbose    bool
	isCloud    bool

	// Command-specific flags
	dryRun          bool
	continueOnError bool
)

var rootCmd = &cobra.Command{
	Use:   "jira-ai-creator",
	Short: "Create Jira issues from YAML configuration",
	Long: `jira-ai-creator is a CLI tool that creates Jira issues from YAML configuration files.

It supports:
  - Epic/Story/Bug hierarchies with parent references
  - Issue dependencies and automatic ordering
  - Custom fields and description templates
  - Idempotency (won't create duplicates on re-runs)
  - Issue links (blocks, relates to, duplicates, etc.)

Quick start:
  1. Export credentials:
     export JIRA_URL="https://jira.yourcompany.com"
     export JIRA_TOKEN="your-personal-access-token"

  2. Validate your config:
     jira-ai-creator validate -f issues.yaml

  3. Preview what will be created:
     jira-ai-creator plan -f issues.yaml

  4. Create issues:
     jira-ai-creator apply -f issues.yaml`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Persistent flags (available to all commands).
	// Sensitive values (jira-url, token) default to "" so that the actual values
	// never appear in --help output. The env vars are applied in requireAuth().
	rootCmd.PersistentFlags().StringVarP(&configFile, "file", "f", "", "YAML configuration file")
	rootCmd.PersistentFlags().StringVar(&jiraURL, "jira-url", "", "Jira base URL (env: JIRA_URL)")
	rootCmd.PersistentFlags().StringVar(&jiraToken, "token", "", "Jira PAT token (env: JIRA_TOKEN)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&isCloud, "cloud", false, "Use Jira Cloud API (v3) instead of Data Center (v2)")
}

// newJiraClient creates a configured Jira client using the global flag values.
func newJiraClient() (*jira.Client, error) {
	return jira.NewClient(jiraURL, jiraToken, isCloud)
}

// maskToken redacts a token for safe logging, exposing only the last 4 characters.
func maskToken(token string) string {
	if len(token) < 12 {
		return "****"
	}
	return "****" + token[len(token)-4:]
}

// useEmoji reports whether emoji output is appropriate (NO_COLOR is not set and TERM is not dumb).
func useEmoji() bool {
	return os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

// emoji returns e when emoji output is appropriate, otherwise fallback.
func emoji(e, fallback string) string {
	if useEmoji() {
		return e
	}
	return fallback
}

// requireAuth applies env-var defaults and checks that Jira authentication is set.
func requireAuth() error {
	// Apply env var defaults when the flag was not explicitly provided.
	if jiraURL == "" {
		jiraURL = os.Getenv("JIRA_URL")
	}
	if jiraToken == "" {
		jiraToken = os.Getenv("JIRA_TOKEN")
	}

	if jiraURL == "" {
		return errors.New("jira URL is required (use --jira-url or set JIRA_URL)")
	}
	if jiraToken == "" {
		return errors.New("jira token is required (use --token or set JIRA_TOKEN)")
	}

	// Warn when the token was provided via the CLI flag rather than the env var.
	// CLI arguments are visible in process listings (ps aux) and shell history.
	if strings.TrimSpace(os.Getenv("JIRA_TOKEN")) != strings.TrimSpace(jiraToken) {
		fmt.Fprintln(os.Stderr, emoji("\u26a0\ufe0f", "[WARN]")+"  Warning: --token passed on command line is visible in process listings; prefer setting JIRA_TOKEN env var")
	}
	return nil
}

// validateFlags checks that required flags are set.
func validateFlags(needsAuth bool) error {
	if configFile == "" {
		return errors.New("config file is required (use -f or --file)")
	}
	if needsAuth {
		return requireAuth()
	}
	return nil
}
