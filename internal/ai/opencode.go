// SPDX-License-Identifier: Apache-2.0
package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"

	opencode "github.com/sst/opencode-sdk-go"
)

type opencodeProvider struct {
	client *opencode.Client
	model  string // optional "providerID/modelID", e.g. "anthropic/claude-haiku-4-5"
}

func newOpencodeProvider(cfg Config) (Provider, error) {
	return &opencodeProvider{client: opencode.NewClient(), model: cfg.Model}, nil
}

func (p *opencodeProvider) Name() string { return "OpenCode (local daemon)" }

func (p *opencodeProvider) Generate(ctx context.Context, userPrompt string) (string, error) {
	// Create a new session on the local OpenCode daemon.
	session, err := p.client.Session.New(ctx, opencode.SessionNewParams{
		Title: opencode.F("jira-ai-creator"),
	})
	if err != nil {
		return "", fmt.Errorf("opencode: failed to create session (is the daemon running?): %w", err)
	}

	// Combine system prompt + user prompt into a single message part,
	// since OpenCode does not have a separate system-prompt API.
	fullPrompt := SystemPrompt() + "\n\n" + userPrompt

	params := opencode.SessionPromptParams{
		Parts: opencode.F([]opencode.SessionPromptParamsPartUnion{
			opencode.TextPartInputParam{
				Text: opencode.F(fullPrompt),
				Type: opencode.F(opencode.TextPartInputTypeText),
			},
		}),
	}
	// If a model was specified as "providerID/modelID", wire it through.
	if p.model != "" {
		providerID, modelID, _ := strings.Cut(p.model, "/")
		if modelID == "" {
			// No slash: treat the whole string as the model ID; provider inferred by OpenCode.
			modelID = providerID
			providerID = ""
		}
		params.Model = opencode.F(opencode.SessionPromptParamsModel{
			ModelID:    opencode.F(modelID),
			ProviderID: opencode.F(providerID),
		})
	}

	resp, err := p.client.Session.Prompt(ctx, session.ID, params)
	if err != nil {
		return "", fmt.Errorf("opencode: prompt failed: %w", err)
	}

	var sb strings.Builder
	for _, part := range resp.Parts {
		if part.Type == opencode.PartTypeText {
			if tp, ok := part.AsUnion().(opencode.TextPart); ok {
				sb.WriteString(tp.Text)
			}
		}
	}

	result := sb.String()
	if result == "" {
		return "", errors.New("opencode: no text content in response")
	}
	return result, nil
}
