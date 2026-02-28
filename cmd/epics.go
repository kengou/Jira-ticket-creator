// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"fmt"
	"os"

	"github.com/kengou/Jira-ticket-creator/internal/jira"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var projectKey string
var epicStatus string
var epicsOutputFile string

var epicsCmd = &cobra.Command{
	Use:   "epics",
	Short: "List existing Epics in a Jira project",
	Long: `Fetches and displays all existing Epics from the specified Jira project.
Optionally filter by status (e.g. "In Progress", "Done").
Prefix with "NOT:" to negate (e.g. "NOT:Done" returns all epics that are NOT Done).

Use --output/-o to save the epics as a YAML file that follows the jira-ai-creator
schema format (schemaVersion, defaults, issues). The YAML includes the description
of each epic and can be used as input for the apply command.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateEpicsFlags(); err != nil {
			return err
		}
		return runEpics()
	},
}

func init() {
	rootCmd.AddCommand(epicsCmd)

	epicsCmd.Flags().StringVarP(&projectKey, "project", "p", "", "Jira project key (required)")
	epicsCmd.Flags().StringVarP(&epicStatus, "status", "s", "", "Filter by status (e.g. \"In Progress\", \"NOT:Done\" to negate)")
	epicsCmd.Flags().StringVarP(&epicsOutputFile, "output", "o", "", "Save epics to a YAML file (jira-ai-creator schema format)")
	epicsCmd.MarkFlagRequired("project")
}

// validateEpicsFlags checks that required flags for the epics command are set.
func validateEpicsFlags() error {
	if jiraURL == "" {
		return fmt.Errorf("Jira URL is required (use --jira-url or set JIRA_URL)")
	}
	if jiraToken == "" {
		return fmt.Errorf("Jira token is required (use --token or set JIRA_TOKEN)")
	}
	if projectKey == "" {
		return fmt.Errorf("project key is required (use -p or --project)")
	}
	return nil
}

// epicsYAMLIssue represents an epic in the YAML output format.
type epicsYAMLIssue struct {
	ID          string `yaml:"id"`
	IssueType   string `yaml:"issueType"`
	EpicName    string `yaml:"epicName"`
	Summary     string `yaml:"summary"`
	Description string `yaml:"description,omitempty"`
}

// epicsYAMLDefaults represents the defaults section in the YAML output.
type epicsYAMLDefaults struct {
	ProjectKey string `yaml:"projectKey"`
}

// epicsYAMLConfig represents the full YAML output structure.
type epicsYAMLConfig struct {
	SchemaVersion string            `yaml:"schemaVersion"`
	Defaults      epicsYAMLDefaults `yaml:"defaults"`
	Issues        []epicsYAMLIssue  `yaml:"issues"`
}

// epicsToYAML converts fetched epics to a YAML byte slice following the
// jira-ai-creator schema format.
func epicsToYAML(project string, epics []jira.Epic) ([]byte, error) {
	cfg := epicsYAMLConfig{
		SchemaVersion: "1.0",
		Defaults: epicsYAMLDefaults{
			ProjectKey: project,
		},
		Issues: make([]epicsYAMLIssue, 0, len(epics)),
	}

	for _, e := range epics {
		issue := epicsYAMLIssue{
			ID:          e.Key,
			IssueType:   "Epic",
			EpicName:    e.Summary,
			Summary:     e.Summary,
			Description: e.Description,
		}
		cfg.Issues = append(cfg.Issues, issue)
	}

	return yaml.Marshal(&cfg)
}

func runEpics() error {
	if epicStatus != "" {
		fmt.Printf("Fetching Epics for project %s (status: %s) from %s...\n\n", projectKey, epicStatus, jiraURL)
	} else {
		fmt.Printf("Fetching Epics for project %s from %s...\n\n", projectKey, jiraURL)
	}

	client := jira.NewClient(jiraURL, jiraToken, isCloud)

	epics, err := client.FetchEpics(projectKey, epicStatus)
	if err != nil {
		return fmt.Errorf("failed to fetch epics: %w", err)
	}

	if len(epics) == 0 {
		fmt.Printf("No Epics found in project %s.\n", projectKey)
		return nil
	}

	fmt.Printf("Found %d Epic(s):\n\n", len(epics))
	fmt.Printf("  %-12s  %-14s  %s\n", "KEY", "STATUS", "SUMMARY")
	fmt.Printf("  %-12s  %-14s  %s\n", "---", "------", "-------")

	for _, epic := range epics {
		fmt.Printf("  %-12s  %-14s  %s\n", epic.Key, epic.Status, epic.Summary)
	}

	// Save to YAML file if --output is specified
	if epicsOutputFile != "" {
		yamlData, err := epicsToYAML(projectKey, epics)
		if err != nil {
			return fmt.Errorf("failed to marshal epics to YAML: %w", err)
		}

		if err := os.WriteFile(epicsOutputFile, yamlData, 0644); err != nil {
			return fmt.Errorf("failed to write YAML file %q: %w", epicsOutputFile, err)
		}

		fmt.Printf("\nEpics saved to %s\n", epicsOutputFile)
	}

	return nil
}
