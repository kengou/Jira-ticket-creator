// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/kengou/Jira-ticket-creator/internal/config"
	"github.com/kengou/Jira-ticket-creator/internal/validation"
)

// ValidationError is a type alias for validation.Error so that existing code
// in the cmd package (including tests) does not need to change.
type ValidationError = validation.Error

// validateConfig delegates to the internal validation package.
func validateConfig(cfg *config.Config) []ValidationError {
	return validation.ValidateConfig(cfg)
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the YAML configuration file",
	Long:  `Validates the YAML configuration file and reports any errors or warnings.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateFlags(false); err != nil {
			return err
		}
		return runValidate()
	},
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate() error {
	fmt.Fprintf(os.Stderr, "%s Validating: %s\n\n", emoji("🔍", "[CHECK]"), configFile)

	rawBytes, err := os.ReadFile(filepath.Clean(configFile))
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := validation.ValidateRawYAML(rawBytes); err != nil {
		return fmt.Errorf("config file failed schema validation: %w", err)
	}

	cfg, err := config.LoadConfigFromBytes(rawBytes)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	validationErrors := validateConfig(cfg)

	if len(validationErrors) == 0 {
		fmt.Fprintf(os.Stderr, "%s Configuration is valid!\n", emoji("✅", "[OK]"))
		printSummary(cfg)
		return nil
	}

	var errorCount, warningCount int
	for _, e := range validationErrors {
		if e.Severity == validation.SeverityError {
			errorCount++
			fmt.Fprintf(os.Stderr, "%s %s\n", emoji("❌", "[ERR]"), e.String())
		} else {
			warningCount++
			fmt.Fprintf(os.Stderr, "%s  %s\n", emoji("⚠️", "[WARN]"), e.String())
		}
	}

	fmt.Fprintf(os.Stderr, "\n%s Validation Summary:\n", emoji("📊", "[SUMMARY]"))
	fmt.Fprintf(os.Stderr, "  - Errors: %d\n", errorCount)
	fmt.Fprintf(os.Stderr, "  - Warnings: %d\n", warningCount)

	if errorCount > 0 {
		return fmt.Errorf("validation failed with %d errors", errorCount)
	}

	if warningCount > 0 {
		fmt.Fprintf(os.Stderr, "\n%s  Configuration has warnings but is valid.\n", emoji("⚠️", "[WARN]"))
	}

	return nil
}

func printSummary(cfg *config.Config) {
	fmt.Fprintf(os.Stderr, "\n%s Summary:\n", emoji("📊", "[SUMMARY]"))
	fmt.Fprintf(os.Stderr, "  - Schema version: %s\n", cfg.SchemaVersion)
	fmt.Fprintf(os.Stderr, "  - Project: %s\n", cfg.Defaults.ProjectKey)
	fmt.Fprintf(os.Stderr, "  - Issues: %d\n", len(cfg.Issues))

	typeCounts := make(map[string]int)
	for _, issue := range cfg.Issues {
		typeCounts[cfg.EffectiveIssueType(&issue)]++
	}
	keys := make([]string, 0, len(typeCounts))
	for t := range typeCounts {
		keys = append(keys, t)
	}
	sort.Strings(keys)
	for _, t := range keys {
		fmt.Fprintf(os.Stderr, "    - %s: %d\n", t, typeCounts[t])
	}
}
