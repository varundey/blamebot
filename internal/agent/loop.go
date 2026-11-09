// Package agent runs the conversation between the user and Claude.
package agent

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/varundey/blamebot/internal/tools"
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

	// Trace, if set, receives a dimmed one-line note for each tool call.
	Trace io.Writer
}

// New returns an Agent. Unless opts say otherwise, the client reads
// ANTHROPIC_API_KEY from the environment.
func New(model string, reg *tools.Registry, opts ...option.RequestOption) *Agent {
	return &Agent{client: anthropic.NewClient(opts...), model: model, tools: reg}
}

// Send adds a user message to the conversation, runs any tools Claude asks
// for, and returns Claude's final reply.
func (a *Agent) Send(ctx context.Context, text string) (string, error) {
	start := len(a.history)
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
			// A refusal carries no content, and replaying an empty assistant
			// turn would make every later request fail, so forget this exchange.
			a.history = a.history[:start]
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
			a.trace("-> %s %s", use.Name, oneLine(string(use.Input), 160))
			out, err := a.tools.Run(ctx, use.Name, use.Input)
			if err != nil {
				a.trace("   failed: %s", oneLine(err.Error(), 160))
				results = append(results, anthropic.NewToolResultBlock(use.ID, err.Error(), true))
				continue
			}
			a.trace("   %d bytes", len(out))
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

func (a *Agent) trace(format string, args ...any) {
	if a.Trace != nil {
		fmt.Fprintf(a.Trace, "\x1b[2m"+format+"\x1b[0m\n", args...)
	}
}

// oneLine collapses s onto a single line of at most max bytes.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
