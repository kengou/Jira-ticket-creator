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

	copilot "github.com/github/copilot-sdk/go"
)

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
	defer client.Stop() //nolint:errcheck

	sessionCfg := &copilot.SessionConfig{
		OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
		SystemMessage: &copilot.SystemMessageConfig{
			Mode:    "replace",
			Content: SystemPrompt(),
		},
	}
	if p.model != "" {
		sessionCfg.Model = p.model
	}

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
func discoverCopilotCLI(flagPath string) (string, error) {
	// 1. Explicit flag
	if flagPath != "" {
		return flagPath, nil
	}

	// 2. COPILOT_CLI_PATH env var
	if envPath := os.Getenv("COPILOT_CLI_PATH"); envPath != "" {
		return envPath, nil
	}

	// 3. PATH lookup
	if p, err := exec.LookPath("copilot"); err == nil {
		return p, nil
	}

	// 4. VSCode extension directory
	if p := findCopilotInVSCode(); p != "" {
		return p, nil
	}

	return "", errors.New("copilot CLI binary not found in PATH or VSCode extension directories")
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

	// Use the last match (highest version when sorted lexicographically).
	return matches[len(matches)-1]
}
