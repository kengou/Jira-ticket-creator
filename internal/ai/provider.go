// SPDX-License-Identifier: Apache-2.0
//
// Package ai provides AI provider implementations for generating Jira YAML plans.
package ai

import (
	"context"
	"errors"
)

// DefaultModel is the AI model used when the caller does not specify one.
const DefaultModel = "claude-haiku-4-5"

// Provider generates YAML from a plain-English prompt.
type Provider interface {
	Name() string
	Generate(ctx context.Context, userPrompt string) (string, error)
}

// ModelLister is an optional interface that providers can implement to list
// available models. Use a type assertion to check: if ml, ok := p.(ModelLister); ok { ... }
type ModelLister interface {
	ListModels(ctx context.Context) ([]string, error)
}

// Config holds the configuration for constructing a Provider.
type Config struct {
	UseClaude   bool
	UseOpencode bool
	UseCopilot  bool

	// Claude options
	ClaudeKey string // defaults to ANTHROPIC_API_KEY env
	Model     string // overrides provider default

	// Copilot options
	CopilotPath string // path to CLI binary; auto-detected if empty

	MaxTokens int  // defaults to 4096
	Verbose   bool // log extra diagnostic info (e.g. resolved CLI binary path)
}

// NewProvider constructs the appropriate Provider from cfg.
// Exactly one of UseClaude, UseOpencode, UseCopilot must be true.
func NewProvider(cfg Config) (Provider, error) {
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4096
	}

	count := 0
	if cfg.UseClaude {
		count++
	}
	if cfg.UseOpencode {
		count++
	}
	if cfg.UseCopilot {
		count++
	}
	if count != 1 {
		return nil, errors.New("specify exactly one of --claude, --opencode, --copilot")
	}

	switch {
	case cfg.UseClaude:
		return newClaudeProvider(cfg)
	case cfg.UseOpencode:
		return newOpencodeProvider(cfg)
	default:
		return newCopilotProvider(cfg)
	}
}
