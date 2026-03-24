// SPDX-License-Identifier: Apache-2.0
package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"

	copilot "github.com/github/copilot-sdk/go"
)

// denyAllPermissions is a restrictive permission handler that denies every
// permission request from the Copilot agent. YAML generation does not require
// filesystem writes, shell execution, or network access beyond the Copilot API.
func denyAllPermissions(_ copilot.PermissionRequest, _ copilot.PermissionInvocation) (copilot.PermissionRequestResult, error) {
	return copilot.PermissionRequestResult{}, errors.New("permission denied: jira-ai-creator does not grant Copilot agent permissions")
}

type copilotProvider struct {
	cliPath   string
	model     string
	maxTokens int
}

func newCopilotProvider(cfg Config) (Provider, error) {
	cliPath, err := discoverCopilotCLI(cfg.CopilotPath)
	if err != nil {
		return nil, fmt.Errorf("GitHub Copilot CLI not found: %w\n\n"+
			"  The VSCode Copilot extension alone is not sufficient — a standalone CLI binary is required.\n"+
			"  Install it with Node.js:\n"+
			"    npm install -g @github/copilot\n\n"+
			"  Alternatively, point to an existing binary:\n"+
			"    --copilot-path /path/to/copilot\n"+
			"    export COPILOT_CLI_PATH=/path/to/copilot", err)
	}
	if cfg.Verbose {
		fmt.Printf("  🔍 Copilot CLI resolved: %s\n", cliPath)
	}
	return &copilotProvider{
		cliPath:   cliPath,
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
	}, nil
}

func (p *copilotProvider) Name() string { return "GitHub Copilot" }

func (p *copilotProvider) Generate(ctx context.Context, userPrompt string) (string, error) {
	client := copilot.NewClient(&copilot.ClientOptions{
		CLIPath: p.cliPath,
	})
	if err := client.Start(ctx); err != nil {
		return "", fmt.Errorf("copilot: failed to start CLI: %w", err)
	}
	defer func() {
		if stopErr := client.Stop(); stopErr != nil {
			fmt.Fprintf(os.Stderr, "copilot: cleanup warning: %v\n", stopErr)
		}
	}()

	sessionCfg := &copilot.SessionConfig{
		OnPermissionRequest: denyAllPermissions,
		SystemMessage: &copilot.SystemMessageConfig{
			Mode:    "replace",
			Content: SystemPrompt(),
		},
	}
	model := p.model
	if model == "" {
		model = DefaultModel
	}
	sessionCfg.Model = model

	session, err := client.CreateSession(ctx, sessionCfg)
	if err != nil {
		return "", fmt.Errorf("copilot: failed to create session: %w", err)
	}

	event, err := session.SendAndWait(ctx, copilot.MessageOptions{
		Prompt: userPrompt,
	})
	if err != nil {
		return "", fmt.Errorf("copilot: request failed: %w", err)
	}
	if event == nil || event.Data.Content == nil {
		return "", errors.New("copilot: empty response")
	}
	return *event.Data.Content, nil
}

// discoverCopilotCLI locates the Copilot CLI binary using a 4-step search:
//  1. --copilot-path flag value (explicit)
//  2. COPILOT_CLI_PATH environment variable
//  3. "copilot" in PATH
//  4. VSCode extension directory scan
//
// Every candidate path is validated to exist and be executable before use.
func discoverCopilotCLI(flagPath string) (string, error) {
	// 1. Explicit flag
	if flagPath != "" {
		if err := validateExecutable(flagPath); err != nil {
			return "", fmt.Errorf("--copilot-path %q: %w", flagPath, err)
		}
		return flagPath, nil
	}

	// 2. COPILOT_CLI_PATH env var
	if envPath := os.Getenv("COPILOT_CLI_PATH"); envPath != "" {
		if err := validateExecutable(envPath); err != nil {
			return "", fmt.Errorf("COPILOT_CLI_PATH %q: %w", envPath, err)
		}
		return envPath, nil
	}

	// 3. PATH lookup — exec.LookPath returns the absolute path; Go 1.19+ no
	// longer resolves binaries in the current working directory on Unix.
	if p, err := exec.LookPath("copilot"); err == nil {
		if validateExecutable(p) == nil {
			return p, nil
		}
	}

	// 4. VSCode extension directory — least trusted source.
	// The extensions directory is writable by the user and any process running
	// as that user, so the binary could have been substituted.
	if p := findCopilotInVSCode(); p != "" {
		if validateExecutable(p) == nil {
			fmt.Fprintf(os.Stderr,
				"Warning: Copilot binary resolved from VSCode extensions directory:\n"+
					"  %s\n"+
					"This is the least trusted discovery method. The binary is not integrity-verified.\n"+
					"For higher assurance, install via 'npm install -g @github/copilot' or use --copilot-path.\n\n",
				p)
			return p, nil
		}
	}

	return "", errors.New("copilot CLI binary not found in PATH or VSCode extension directories")
}

// validateExecutable checks that a path points to an existing, executable file.
func validateExecutable(path string) error {
	absPath, absErr := filepath.Abs(path)
	if absErr != nil {
		return fmt.Errorf("cannot resolve executable path: %w", absErr)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("not found: %w", err)
	}
	if info.IsDir() {
		return errors.New("path is a directory, not an executable file")
	}
	// Check execute bit (owner, group, or other) on Unix.
	if info.Mode()&0o111 == 0 {
		return errors.New("file is not executable")
	}
	return nil
}

// findCopilotInVSCode scans the VSCode extensions directory for the Copilot agent binary.
func findCopilotInVSCode() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	agentBinary := "agent"
	if runtime.GOOS == "windows" {
		agentBinary = "agent.exe"
	}

	// VSCode stores extensions under ~/.vscode/extensions on all platforms.
	// On Windows it's %USERPROFILE%\.vscode\extensions (same as home).
	pattern := filepath.Join(home, ".vscode", "extensions", "github.copilot-*", "dist", agentBinary)
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return ""
	}

	// Sort to ensure deterministic selection of the highest version.
	sort.Strings(matches)
	return matches[len(matches)-1]
}
