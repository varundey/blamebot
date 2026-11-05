// Package agent runs the conversation between the user and Claude.
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/varundey/oncall/internal/tools"
)

const (
	maxTokens = 16000
	// maxTurns bounds the model/tool round trips for a single user message so
	// a confused model cannot loop forever.
	maxTurns = 25
)

// Agent holds a conversation with Claude. The API is stateless, so the full
// history is sent on every request.
type Agent struct {
	client  anthropic.Client
	model   string
	tools   *tools.Registry
	history []anthropic.MessageParam
}

// New returns an Agent that reads ANTHROPIC_API_KEY from the environment.
func New(model string, reg *tools.Registry) *Agent {
	return &Agent{client: anthropic.NewClient(), model: model, tools: reg}
}

// Send adds a user message to the conversation, runs any tools Claude asks
// for, and returns Claude's final reply.
func (a *Agent) Send(ctx context.Context, text string) (string, error) {
	a.history = append(a.history, anthropic.NewUserMessage(anthropic.NewTextBlock(text)))

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(a.model),
			MaxTokens: maxTokens,
			System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
			Messages:  a.history,
			Tools:     a.tools.Params(),
		})
		if err != nil {
			return "", fmt.Errorf("messages.new: %w", err)
		}
		a.history = append(a.history, resp.ToParam())

		switch resp.StopReason {
		case anthropic.StopReasonToolUse:
			// Fall through to run the requested tools below.
		case anthropic.StopReasonRefusal:
			return "", fmt.Errorf("request declined (%s)", resp.StopDetails.Category)
		case anthropic.StopReasonMaxTokens:
			return replyText(resp) + "\n[response cut off at max_tokens]", nil
		default:
			return replyText(resp), nil
		}

		// All results for one assistant turn go back in a single user message.
		results := make([]anthropic.ContentBlockParamUnion, 0, len(resp.Content))
		for _, block := range resp.Content {
			use, ok := block.AsAny().(anthropic.ToolUseBlock)
			if !ok {
				continue
			}
			out, err := a.tools.Run(ctx, use.Name, use.Input)
			if err != nil {
				results = append(results, anthropic.NewToolResultBlock(use.ID, err.Error(), true))
				continue
			}
			results = append(results, anthropic.NewToolResultBlock(use.ID, out, false))
		}
		a.history = append(a.history, anthropic.NewUserMessage(results...))
	}
	return "", fmt.Errorf("gave up after %d model turns without a final answer", maxTurns)
}

func replyText(m *anthropic.Message) string {
	var sb strings.Builder
	for _, block := range m.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String()
}
