// SPDX-License-Identifier: Apache-2.0
package ai

import (
	"context"
	"errors"
	"fmt"
	"os"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type claudeProvider struct {
	client    *anthropic.Client
	model     anthropic.Model
	maxTokens int64
}

func newClaudeProvider(cfg Config) (Provider, error) {
	key := cfg.ClaudeKey
	if key == "" {
		key = os.Getenv("ANTHROPIC_API_KEY")
	}
	if key == "" {
		return nil, errors.New("anthropic API key required: set ANTHROPIC_API_KEY or use --claude-key")
	}

	model := anthropic.Model(cfg.Model)
	if model == "" {
		model = anthropic.ModelClaudeSonnet4_6
	}

	client := anthropic.NewClient(option.WithAPIKey(key))
	return &claudeProvider{
		client:    &client,
		model:     model,
		maxTokens: int64(cfg.MaxTokens),
	}, nil
}

func (p *claudeProvider) Name() string { return "Claude (Anthropic)" }

func (p *claudeProvider) Generate(ctx context.Context, userPrompt string) (string, error) {
	resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		System:    []anthropic.TextBlockParam{{Text: SystemPrompt()}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("claude API error: %w", err)
	}

	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			return tb.Text, nil
		}
	}

	return "", errors.New("claude returned no text content")
}
