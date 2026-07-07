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

	"github.com/kengou/jira-ticket-creator/internal/jira"
	"github.com/kengou/jira-ticket-creator/internal/ui"
)

var (
	// Global flags
	configFile string
	jiraURL    string
	jiraToken  string
	jiraEmail  string
	verbose    bool
	isCloud    bool

	// resolvedMode is the platform mode resolved once by requireAuth and read by
	// newJiraClient. It is set during the startup auth validation.
	resolvedMode jira.Mode

	// modeAutoDetected records whether resolvedMode came from URL host
	// inspection (true) or from an explicit override flag/env (false). Set by
	// requireAuth alongside resolvedMode.
	modeAutoDetected bool

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

     Jira Data Center (personal access token):
       export JIRA_URL="https://jira.yourcompany.com"
       export JIRA_TOKEN="your-personal-access-token"

     Jira Cloud (Atlassian email + API token from id.atlassian.com;
     Cloud mode is auto-detected for *.atlassian.net URLs):
       export JIRA_URL="https://yourcompany.atlassian.net"
       export JIRA_EMAIL="you@yourcompany.com"
       export JIRA_TOKEN="your-api-token"

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
	rootCmd.PersistentFlags().BoolVar(&isCloud, "cloud", false, "Force Jira Cloud mode (auto-detected for *.atlassian.net; env: JIRA_CLOUD)")
	rootCmd.PersistentFlags().StringVar(&jiraEmail, "email", "", "Atlassian account email for Jira Cloud (env: JIRA_EMAIL)")
}

// newJiraClient creates a configured Jira client using the resolved platform
// mode (set by requireAuth) and the current credential globals.
func newJiraClient() (*jira.Client, error) {
	return jira.NewClientWithMode(jiraURL, resolvedMode, jira.Credentials{
		Email: jiraEmail,
		Token: jiraToken,
	})
}

// maskToken redacts a token for safe logging, exposing only the last 4 characters.
func maskToken(token string) string {
	if len(token) < 12 {
		return "****"
	}
	return "****" + token[len(token)-4:]
}

// useEmoji reports whether emoji output is appropriate (NO_COLOR is not set
// and TERM is not dumb). It delegates to internal/ui for a single source of
// truth; the local wrapper keeps all existing call sites (including spinner.go)
// unchanged.
func useEmoji() bool {
	return ui.UseEmoji()
}

// emoji returns e when emoji output is appropriate, otherwise fallback. It
// delegates to internal/ui for a single source of truth; the local wrapper
// keeps all existing call sites unchanged.
func emoji(e, fallback string) string {
	return ui.Emoji(e, fallback)
}

// resolvePlatformMode builds the mode-override pointer from the --cloud flag
// and JIRA_CLOUD env var, then delegates to jira.ResolveMode. It returns the
// resolved Mode and the autoDetected flag from jira.ResolveMode (false when an
// explicit override was applied, true when the mode was inferred from the URL
// host). This is the single place that encapsulates all override construction
// so that runCheck does not need to duplicate it.
func resolvePlatformMode() (jira.Mode, bool) {
	var override *bool
	if isCloud || cloudEnvSet() {
		forced := true
		override = &forced
	}
	return jira.ResolveMode(jiraURL, override)
}

// requireAuth applies env-var defaults, resolves the platform mode, and checks
// that Jira authentication is set. It MUST run before newJiraClient or
// NewApplier because it populates the package-level resolvedMode and
// modeAutoDetected globals that those functions read. In Cloud mode a missing
// email is rejected before any HTTP request is made.
func requireAuth() error {
	// Apply env var defaults when the flag was not explicitly provided.
	if jiraURL == "" {
		jiraURL = os.Getenv("JIRA_URL")
	}
	if jiraToken == "" {
		jiraToken = os.Getenv("JIRA_TOKEN")
	}
	if jiraEmail == "" {
		jiraEmail = os.Getenv("JIRA_EMAIL")
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

	// Resolve the platform mode exactly once and store both the mode and
	// whether it was auto-detected for downstream consumers (e.g. runCheck).
	mode, autoDetected := resolvePlatformMode()
	resolvedMode = mode
	modeAutoDetected = autoDetected

	if verbose {
		how := "auto-detected"
		if !autoDetected {
			how = "explicitly set"
		}
		fmt.Fprintf(os.Stderr, "Jira platform mode: %s (%s)\n", mode.String(), how)
	}

	// Fail fast: Cloud requires an Atlassian email together with the API token.
	if mode == jira.ModeCloud && strings.TrimSpace(jiraEmail) == "" {
		return errors.New("jira Cloud requires an Atlassian email together with an API token (use --email or set JIRA_EMAIL)")
	}

	return nil
}

// cloudEnvSet reports whether the JIRA_CLOUD environment variable is set to a
// truthy value ("1", "true", "yes", case-insensitive).
func cloudEnvSet() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("JIRA_CLOUD"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
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
