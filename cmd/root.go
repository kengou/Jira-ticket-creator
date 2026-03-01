// SPDX-License-Identifier: Apache-2.0
//
// Package cmd implements the CLI commands for jira-ai-creator.
package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
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
	// Persistent flags (available to all commands)
	rootCmd.PersistentFlags().StringVarP(&configFile, "file", "f", "", "YAML configuration file")
	rootCmd.PersistentFlags().StringVar(&jiraURL, "jira-url", os.Getenv("JIRA_URL"), "Jira base URL")
	rootCmd.PersistentFlags().StringVar(&jiraToken, "token", os.Getenv("JIRA_TOKEN"), "Jira PAT token")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&isCloud, "cloud", false, "Use Jira Cloud API (v3) instead of Data Center (v2)")
}

// maskToken redacts a token for safe logging.
func maskToken(_ string) string {
	return "****"
}

// requireAuth checks that Jira authentication flags are set.
func requireAuth() error {
	if jiraURL == "" {
		return errors.New("jira URL is required (use --jira-url or set JIRA_URL)")
	}
	if jiraToken == "" {
		return errors.New("jira token is required (use --token or set JIRA_TOKEN)")
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
