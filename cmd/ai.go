// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kengou/Jira-ticket-creator/internal/ai"
	"github.com/kengou/Jira-ticket-creator/internal/config"
	"github.com/kengou/Jira-ticket-creator/internal/source"
	"github.com/kengou/Jira-ticket-creator/internal/validation"
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
	aiSourceDir   string
	aiTimeout     int
	aiYes         bool
	aiIncludeDocs bool
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
	aiCmd.Flags().StringVar(&aiModel, "model", os.Getenv("JIRA_AI_MODEL"), "Override AI model; env: JIRA_AI_MODEL (Claude/Copilot: model name; OpenCode: provider/modelID)")
	aiCmd.Flags().StringVar(&aiClaudeKey, "claude-key", "", "Anthropic API key (default: ANTHROPIC_API_KEY env)")
	aiCmd.Flags().StringVar(&aiCopilotPath, "copilot-path", "", "Path to Copilot agent binary (default: auto-detected)")
	aiCmd.Flags().IntVar(&aiMaxTokens, "max-tokens", 4096, "Maximum tokens in AI response")
	aiCmd.Flags().StringVarP(&aiSourceDir, "source-dir", "d", "", "Source directory to analyse and include as context for the AI")
	aiCmd.Flags().IntVar(&aiTimeout, "timeout", 120, "Timeout in seconds for AI generation")
	aiCmd.Flags().BoolVar(&aiYes, "yes", false, "Skip confirmation prompts (e.g. before sending source files to AI)")
	aiCmd.Flags().BoolVar(&aiIncludeDocs, "include-docs", false, "Include .md and .txt files when scanning --source-dir (excluded by default to reduce prompt injection risk)")

	// Hide --claude-key from help output to discourage passing secrets via CLI flags.
	// Users should prefer the ANTHROPIC_API_KEY environment variable.
	if err := aiCmd.Flags().MarkHidden("claude-key"); err != nil {
		panic("jira-ai-creator: " + err.Error())
	}
}

// yamlFencedBlockRe matches a fenced code block that optionally starts with ```yaml.
// Capture group 1 contains the block content.
var yamlFencedBlockRe = regexp.MustCompile("(?ms)^```(?:yaml)?[ \t]*\n(.*?)\n^```[ \t]*$")

// extractYAML returns the content of the first fenced YAML block in s,
// or the full string trimmed if no fences are found.
func extractYAML(s string) string {
	if m := yamlFencedBlockRe.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(s)
}

func runAI() error {
	if aiPrompt == "" {
		return errors.New("--prompt / -p is required")
	}

	// Warn when the API key was provided via CLI flag — it is visible in process
	// listings (ps aux) and shell history. Prefer the ANTHROPIC_API_KEY env var.
	if aiClaudeKey != "" && aiClaudeKey != os.Getenv("ANTHROPIC_API_KEY") {
		fmt.Fprintln(os.Stderr, emoji("⚠️ ", "[WARN]")+" Warning: --claude-key passed on command line is visible in process listings; prefer setting ANTHROPIC_API_KEY env var")
	}

	provider, err := ai.NewProvider(ai.Config{
		UseClaude:   aiUseClaude,
		UseOpencode: aiUseOpencode,
		UseCopilot:  aiUseCopilot,
		Model:       aiModel,
		ClaudeKey:   aiClaudeKey,
		CopilotPath: aiCopilotPath,
		MaxTokens:   aiMaxTokens,
		Verbose:     verbose,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "%s Generating with %s…\n\n", emoji("🤖", "[AI]"), provider.Name())

	userPrompt := aiPrompt
	if aiSourceDir != "" {
		opts := source.BuildOptions{IncludeDocs: aiIncludeDocs}
		files, err := source.ListFiles(aiSourceDir, opts)
		if err != nil {
			return fmt.Errorf("scan source directory: %w", err)
		}
		if len(files) == 0 {
			fmt.Fprintln(os.Stderr, "Warning: --source-dir matched zero files; proceeding without source context")
		} else {
			if !aiYes {
				fmt.Fprintf(os.Stderr, "The following %d file(s) from %q will be sent to the AI provider:\n", len(files), aiSourceDir)
				for _, f := range files {
					fmt.Fprintf(os.Stderr, "  %s\n", f)
				}
				if !aiIncludeDocs {
					fmt.Fprintln(os.Stderr, "  (Note: .md and .txt excluded; use --include-docs to include them)")
				}
				if !confirmPrompt(os.Stderr, os.Stdin, "\nProceed? [y/N] ") {
					return errors.New("aborted by user")
				}
			}
			sourceCtx, buildErr := source.BuildContextFromFiles(aiSourceDir, files)
			if buildErr != nil {
				return fmt.Errorf("build source context: %w", buildErr)
			}
			fmt.Fprintf(os.Stderr, "Included source context from: %s (%d files)\n\n", aiSourceDir, len(files))
			userPrompt = sourceCtx + "\n\nUSER REQUEST:\n" + aiPrompt
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(aiTimeout)*time.Second)
	defer cancel()

	spin := newSpinner(fmt.Sprintf("Generating… (timeout %ds)", aiTimeout))
	spin.Start()
	raw, err := provider.Generate(ctx, userPrompt)
	spin.Stop()
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	// Strip markdown code fences the AI may have added despite instructions.
	clean := extractYAML(raw)

	// Schema-validate the raw YAML BEFORE deserialization (OWASP ASVS V5.5).
	// This rejects unexpected keys, wrong types, and structure violations from a
	// misbehaving or compromised AI provider before any Go struct is populated
	// with untrusted data.
	if schemaErr := validation.ValidateRawYAML([]byte(clean)); schemaErr != nil {
		if tmpFile, tmpErr := os.CreateTemp("", "jira-ai-creator-*.yaml"); tmpErr == nil {
			if chmodErr := os.Chmod(tmpFile.Name(), 0600); chmodErr != nil {
				fmt.Fprintf(os.Stderr, "warning: could not set permissions on temp file: %v\n", chmodErr)
			}
			tmpName := tmpFile.Name()
			_, writeErr := fmt.Fprint(tmpFile, raw)
			closeErr := tmpFile.Close()
			if writeErr == nil && closeErr == nil {
				fmt.Fprintf(os.Stderr, "Raw AI output saved to: %s\n", tmpName)
				return fmt.Errorf("AI output failed schema validation: %w", schemaErr)
			}
		}
		return fmt.Errorf("AI output failed schema validation: %w", schemaErr)
	}

	// Parse the schema-valid YAML into the config struct.
	cfg, err := config.LoadConfigFromBytes([]byte(clean))
	if err != nil {
		// Save raw output to a temp file so the user can inspect it without terminal noise.
		if tmpFile, tmpErr := os.CreateTemp("", "jira-ai-creator-*.yaml"); tmpErr == nil {
			if chmodErr := os.Chmod(tmpFile.Name(), 0600); chmodErr != nil {
				fmt.Fprintf(os.Stderr, "warning: could not set permissions on temp file: %v\n", chmodErr)
			}
			tmpName := tmpFile.Name()
			_, writeErr := fmt.Fprint(tmpFile, raw)
			closeErr := tmpFile.Close()
			if writeErr == nil && closeErr == nil {
				fmt.Fprintf(os.Stderr, "Raw AI output saved to: %s\n", tmpName)
				return fmt.Errorf("AI output is not valid YAML: %w", err)
			}
		}
		return fmt.Errorf("AI output is not valid YAML: %w", err)
	}

	validationErrors := validateConfig(cfg)

	var errorCount, warnCount int
	for _, e := range validationErrors {
		if e.Severity == validation.SeverityError {
			errorCount++
			fmt.Fprintf(os.Stderr, "%s %s\n", emoji("❌", "[ERR]"), e.String())
		} else {
			warnCount++
			fmt.Fprintf(os.Stderr, "%s  %s\n", emoji("⚠️", "[WARN]"), e.String())
		}
	}

	if errorCount > 0 {
		return fmt.Errorf("generated YAML failed validation with %d error(s); output not written", errorCount)
	}

	if warnCount > 0 {
		fmt.Fprintf(os.Stderr, "\n%s  %d warning(s); proceeding.\n\n", emoji("⚠️", "[WARN]"), warnCount)
	}

	// Prepend AI-generated marker so that `apply` can warn about un-reviewed output.
	const aiMarker = "# ai-generated: true\n# reviewed: false\n"
	markedOutput := aiMarker + clean

	// Write output.
	if aiOutput == "" {
		fmt.Print(markedOutput)
		if !strings.HasSuffix(clean, "\n") {
			fmt.Println()
		}
	} else {
		if err := os.WriteFile(aiOutput, []byte(markedOutput+"\n"), 0600); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Saved to %s (%d issues)\n", aiOutput, len(cfg.Issues))
		fmt.Fprintln(os.Stderr, "IMPORTANT: Review the generated YAML before running 'apply'.")
		fmt.Fprintln(os.Stderr, "           Change '# reviewed: false' to '# reviewed: true' after review.")
	}

	return nil
}
