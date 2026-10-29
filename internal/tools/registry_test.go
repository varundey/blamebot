package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

type fakeTool struct{ name string }

func (f fakeTool) Name() string        { return f.name }
func (f fakeTool) Description() string { return "fake " + f.name }
func (f fakeTool) Schema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{}
}
func (f fakeTool) Run(context.Context, json.RawMessage) (string, error) {
	return "ran " + f.name, nil
}

func TestRegistryRun(t *testing.T) {
	r := NewRegistry(fakeTool{"a"}, fakeTool{"b"})

	got, err := r.Run(context.Background(), "b", nil)
	if err != nil || got != "ran b" {
		t.Fatalf("Run(b) = %q, %v", got, err)
	}

	_, err = r.Run(context.Background(), "missing", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("Run(missing) error = %v, want unknown tool", err)
	}
}

func TestRegistryParamsAreSorted(t *testing.T) {
	r := NewRegistry(fakeTool{"zeta"}, fakeTool{"alpha"}, fakeTool{"mid"})

	var names []string
	for _, p := range r.Params() {
		names = append(names, p.OfTool.Name)
	}
	if strings.Join(names, ",") != "alpha,mid,zeta" {
		t.Fatalf("param order = %v", names)
	}
}
