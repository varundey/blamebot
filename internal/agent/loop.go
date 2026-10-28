// Package agent runs the conversation between the user and Claude.
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

const maxTokens = 16000

// Agent holds a conversation with Claude. The API is stateless, so the full
// history is sent on every request.
type Agent struct {
	client  anthropic.Client
	model   string
	history []anthropic.MessageParam
}

// New returns an Agent that reads ANTHROPIC_API_KEY from the environment.
func New(model string) *Agent {
	return &Agent{client: anthropic.NewClient(), model: model}
}

// Send adds a user message to the conversation and returns Claude's reply.
func (a *Agent) Send(ctx context.Context, text string) (string, error) {
	a.history = append(a.history, anthropic.NewUserMessage(anthropic.NewTextBlock(text)))

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: maxTokens,
		Messages:  a.history,
	})
	if err != nil {
		return "", fmt.Errorf("messages.new: %w", err)
	}
	a.history = append(a.history, resp.ToParam())

	var sb strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String(), nil
}
