// Package tools defines the actions the agent can take against a running
// system, and the registry that maps tool names to implementations.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/anthropics/anthropic-sdk-go"
)

// Tool is one capability exposed to the model.
type Tool interface {
	Name() string
	Description() string
	// Schema describes the tool's input as a JSON schema.
	Schema() anthropic.ToolInputSchemaParam
	// Run executes the tool. A returned error is reported back to the model
	// as a failed tool result rather than aborting the conversation.
	Run(ctx context.Context, input json.RawMessage) (string, error)
}

// Registry holds the tools available to an agent.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry returns a registry containing the given tools.
func NewRegistry(ts ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(ts))}
	for _, t := range ts {
		r.Register(t)
	}
	return r
}

// Register adds a tool, replacing any existing tool with the same name.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Run executes the named tool.
func (r *Registry) Run(ctx context.Context, name string, input json.RawMessage) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	return t.Run(ctx, input)
}

// Params returns the tool definitions in the shape the Messages API expects,
// sorted by name so the request prefix stays stable between turns.
func (r *Registry) Params() []anthropic.ToolUnionParam {
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)

	params := make([]anthropic.ToolUnionParam, 0, len(names))
	for _, n := range names {
		t := r.tools[n]
		params = append(params, anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        t.Name(),
			Description: anthropic.String(t.Description()),
			InputSchema: t.Schema(),
		}})
	}
	return params
}
