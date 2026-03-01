// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kengou/Jira-ticket-creator/internal/apply"
	"github.com/kengou/Jira-ticket-creator/internal/config"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show what would be created without making API calls",
	Long:  `Prints a plan of what issues would be created and in what order, without actually creating them.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateFlags(false); err != nil {
			return err
		}
		return runPlan()
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
}

func runPlan() error {
	fmt.Printf("📋 Planning: %s\n\n", configFile)

	// Load config
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate
	errs := validateConfig(cfg)
	hasErrors := false
	for _, e := range errs {
		if e.Severity == "error" {
			if !hasErrors {
				fmt.Println("❌ Configuration has validation errors:")
				hasErrors = true
			}
			fmt.Printf("  - %s\n", e.String())
		}
	}
	if hasErrors {
		return errors.New("cannot create plan due to validation errors")
	}

	// Print plan
	return apply.PrintPlan(cfg)
}
