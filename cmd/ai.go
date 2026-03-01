// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kengou/Jira-ticket-creator/internal/ai"
	"github.com/kengou/Jira-ticket-creator/internal/config"
)

var (
	aiUseClaude   bool
	aiUseOpencode bool
	aiUseCopilot  bool
	aiPrompt      string
	aiOutput      string
	aiModel       string
	aiClaudeKey   string
	aiCopilotPath string
	aiMaxTokens   int
)

var aiCmd = &cobra.Command{
	Use:   "ai",
	Short: "Generate a YAML plan with the help of AI",
	Long: `Generate a jira-ai-creator YAML configuration file from a plain-English description
using an AI provider (Claude, OpenCode, or GitHub Copilot).

Select exactly one provider via flags. The generated YAML is validated before writing.

Examples:
  jira-ai-creator ai --claude   -p "create 3 auth epics with stories" -o tickets/auth.yaml
  jira-ai-creator ai --opencode -p "monitoring plan for a k8s cluster" -o tickets/monitor.yaml
  jira-ai-creator ai --copilot  -p "data pipeline epics and stories"   -o tickets/pipeline.yaml`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAI()
	},
}

func init() {
	rootCmd.AddCommand(aiCmd)

	aiCmd.Flags().BoolVar(&aiUseClaude, "claude", false, "Use Anthropic Claude")
	aiCmd.Flags().BoolVar(&aiUseOpencode, "opencode", false, "Use local OpenCode daemon")
	aiCmd.Flags().BoolVar(&aiUseCopilot, "copilot", false, "Use GitHub Copilot CLI or VSCode extension")
	aiCmd.Flags().StringVarP(&aiPrompt, "prompt", "p", "", "Plain-English description of the Jira plan to generate (required)")
	aiCmd.Flags().StringVarP(&aiOutput, "output", "o", "", "Output YAML file path (default: stdout)")
	aiCmd.Flags().StringVar(&aiModel, "model", "", "Override AI model (provider-specific)")
	aiCmd.Flags().StringVar(&aiClaudeKey, "claude-key", "", "Anthropic API key (default: ANTHROPIC_API_KEY env)")
	aiCmd.Flags().StringVar(&aiCopilotPath, "copilot-path", "", "Path to Copilot agent binary (default: auto-detected)")
	aiCmd.Flags().IntVar(&aiMaxTokens, "max-tokens", 4096, "Maximum tokens in AI response")
}

var yamlFenceRe = regexp.MustCompile("(?m)^```(?:yaml)?\\s*\\n?|^```\\s*$")

func runAI() error {
	if aiPrompt == "" {
		return errors.New("--prompt / -p is required")
	}

	// Warn when the API key was provided via CLI flag — it is visible in process
	// listings (ps aux) and shell history. Prefer the ANTHROPIC_API_KEY env var.
	if aiClaudeKey != "" && aiClaudeKey != os.Getenv("ANTHROPIC_API_KEY") {
		fmt.Fprintln(os.Stderr, "⚠️  Warning: --claude-key passed on command line is visible in process listings; prefer setting ANTHROPIC_API_KEY env var")
	}

	provider, err := ai.NewProvider(ai.Config{
		UseClaude:   aiUseClaude,
		UseOpencode: aiUseOpencode,
		UseCopilot:  aiUseCopilot,
		Model:       aiModel,
		ClaudeKey:   aiClaudeKey,
		CopilotPath: aiCopilotPath,
		MaxTokens:   aiMaxTokens,
	})
	if err != nil {
		return err
	}

	fmt.Printf("🤖 Generating with %s…\n\n", provider.Name())

	raw, err := provider.Generate(context.Background(), aiPrompt)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	// Strip markdown code fences the AI may have added despite instructions.
	clean := strings.TrimSpace(yamlFenceRe.ReplaceAllString(raw, ""))

	// Validate the generated YAML before writing.
	cfg, err := config.LoadConfigFromBytes([]byte(clean))
	if err != nil {
		return fmt.Errorf("AI output is not valid YAML: %w\n\nRaw output:\n%s", err, raw)
	}

	validationErrors := validateConfig(cfg)

	var errorCount, warnCount int
	for _, e := range validationErrors {
		if e.Severity == "error" {
			errorCount++
			fmt.Printf("❌ %s\n", e.String())
		} else {
			warnCount++
			fmt.Printf("⚠️  %s\n", e.String())
		}
	}

	if errorCount > 0 {
		return fmt.Errorf("generated YAML failed validation with %d error(s); output not written", errorCount)
	}

	if warnCount > 0 {
		fmt.Printf("\n⚠️  %d warning(s); proceeding.\n\n", warnCount)
	}

	// Write output.
	if aiOutput == "" {
		fmt.Print(clean)
		if !strings.HasSuffix(clean, "\n") {
			fmt.Println()
		}
	} else {
		if err := os.WriteFile(aiOutput, []byte(clean+"\n"), 0600); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Printf("✅ Saved to %s (%d issues)\n", aiOutput, len(cfg.Issues))
	}

	return nil
}
