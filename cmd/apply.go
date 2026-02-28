package cmd

import (
	"fmt"

	"github.com/kengou/Jira-ticket-creator/internal/apply"
	"github.com/kengou/Jira-ticket-creator/internal/config"
	"github.com/kengou/Jira-ticket-creator/internal/jira"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Create issues in Jira",
	Long:  `Creates issues in Jira according to the configuration file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// For dry-run, we don't need authentication
		needsAuth := !dryRun
		if err := validateFlags(needsAuth); err != nil {
			return err
		}
		return runApply()
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)

	applyCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Simulate creation without making API calls")
	applyCmd.Flags().BoolVar(&continueOnError, "continue-on-error", false, "Continue creating issues even if one fails")
}

func runApply() error {
	fmt.Printf("🚀 Applying: %s\n", configFile)
	if dryRun {
		fmt.Println("   [DRY RUN MODE - No changes will be made]")
		fmt.Println()
	} else {
		fmt.Printf("   Target: %s (token: %s)\n\n", jiraURL, maskToken(jiraToken))
	}

	// Load config
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override continue-on-error if flag is set
	if continueOnError {
		if cfg.Options == nil {
			cfg.Options = &config.Options{}
		}
		cfg.Options.ContinueOnError = true
	}

	// Validate
	errors := validateConfig(cfg)
	hasErrors := false
	for _, e := range errors {
		if e.Severity == "error" {
			if !hasErrors {
				fmt.Println("❌ Configuration has validation errors:")
				hasErrors = true
			}
			fmt.Printf("  - %s\n", e.String())
		}
	}
	if hasErrors {
		return fmt.Errorf("cannot apply due to validation errors")
	}

	// Print warnings
	hasWarnings := false
	for _, e := range errors {
		if e.Severity == "warning" {
			if !hasWarnings {
				fmt.Println("⚠️  Warnings:")
				hasWarnings = true
			}
			fmt.Printf("  - %s\n", e.String())
		}
	}
	if hasWarnings {
		fmt.Println()
	}

	// Create Jira client
	var client *jira.Client
	if !dryRun {
		client = jira.NewClient(jiraURL, jiraToken, isCloud)
	}

	// Create applier
	applier := apply.NewApplier(cfg, client, verbose, dryRun, configFile)

	// Apply
	if err := applier.Apply(); err != nil {
		return err
	}

	if !dryRun {
		fmt.Println("\n🎉 All done! Issues have been created in Jira.")
	} else {
		fmt.Println("\n✅ Dry run complete. No issues were created.")
	}

	return nil
}
