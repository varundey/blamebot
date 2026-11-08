package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/varundey/oncall/internal/tools"
)

// echoTool returns its "v" input, or fails when v is "boom".
type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "echo back v" }
func (echoTool) Schema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{Properties: map[string]any{"v": map[string]any{"type": "string"}}}
}
func (echoTool) Run(_ context.Context, raw json.RawMessage) (string, error) {
	var in struct{ V string }
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", err
	}
	if in.V == "boom" {
		return "", errors.New("exploded")
	}
	return "echo:" + in.V, nil
}

// fakeAPI replies to each POST /v1/messages with the next canned response and
// records the request bodies it saw.
type fakeAPI struct {
	replies  []string
	requests []map[string]any
}

func (f *fakeAPI) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		f.requests = append(f.requests, req)

		i := len(f.requests) - 1
		if i >= len(f.replies) {
			i = len(f.replies) - 1 // keep repeating the last reply
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, f.replies[i])
	}
}

func message(stopReason string, content string) string {
	return fmt.Sprintf(`{"id":"msg_x","type":"message","role":"assistant","model":"fake","content":%s,`+
		`"stop_reason":%q,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`, content, stopReason)
}

func newTestAgent(t *testing.T, api *fakeAPI) *Agent {
	t.Helper()
	srv := httptest.NewServer(api.handler(t))
	t.Cleanup(srv.Close)
	return New("fake-model", tools.NewRegistry(echoTool{}),
		option.WithBaseURL(srv.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
}

func TestSendRunsParallelToolCallsInOneUserMessage(t *testing.T) {
	api := &fakeAPI{replies: []string{
		message("tool_use", `[{"type":"text","text":"checking"},
			{"type":"tool_use","id":"toolu_1","name":"echo","input":{"v":"a"}},
			{"type":"tool_use","id":"toolu_2","name":"echo","input":{"v":"boom"}},
			{"type":"tool_use","id":"toolu_3","name":"nope","input":{}}]`),
		message("end_turn", `[{"type":"text","text":"all done"}]`),
	}}
	a := newTestAgent(t, api)

	got, err := a.Send(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if got != "all done" {
		t.Fatalf("reply = %q", got)
	}
	if len(api.requests) != 2 {
		t.Fatalf("made %d requests, want 2", len(api.requests))
	}

	msgs := api.requests[1]["messages"].([]any)
	if len(msgs) != 3 { // user, assistant(tool_use), user(tool_results)
		t.Fatalf("second request had %d messages, want 3", len(msgs))
	}
	last := msgs[2].(map[string]any)
	if last["role"] != "user" {
		t.Fatalf("tool results sent as role %v", last["role"])
	}
	blocks := last["content"].([]any)
	if len(blocks) != 3 {
		t.Fatalf("got %d tool_result blocks in one message, want 3", len(blocks))
	}

	want := []struct {
		id      string
		content string
		isError bool
	}{
		{"toolu_1", "echo:a", false},
		{"toolu_2", "exploded", true},
		{"toolu_3", `unknown tool "nope"`, true},
	}
	for i, w := range want {
		b := blocks[i].(map[string]any)
		isErr, _ := b["is_error"].(bool)
		text := b["content"].([]any)[0].(map[string]any)["text"]
		if b["tool_use_id"] != w.id || text != w.content || isErr != w.isError {
			t.Errorf("block %d = %v, want %+v", i, b, w)
		}
	}
}

func TestSendKeepsHistoryAcrossMessages(t *testing.T) {
	api := &fakeAPI{replies: []string{message("end_turn", `[{"type":"text","text":"hi"}]`)}}
	a := newTestAgent(t, api)

	for _, q := range []string{"one", "two"} {
		if _, err := a.Send(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(api.requests[1]["messages"].([]any)); n != 3 {
		t.Fatalf("second request carried %d messages, want 3 (user, assistant, user)", n)
	}
}

func TestSendSendsSystemPromptAndSortedTools(t *testing.T) {
	api := &fakeAPI{replies: []string{message("end_turn", `[{"type":"text","text":"ok"}]`)}}
	a := newTestAgent(t, api)
	if _, err := a.Send(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	req := api.requests[0]
	sys := req["system"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(sys, "oncall") {
		t.Errorf("system prompt not sent: %q", sys)
	}
	if name := req["tools"].([]any)[0].(map[string]any)["name"]; name != "echo" {
		t.Errorf("tools = %v", req["tools"])
	}
}

func TestSendStopsAfterMaxTurns(t *testing.T) {
	api := &fakeAPI{replies: []string{
		message("tool_use", `[{"type":"tool_use","id":"toolu_1","name":"echo","input":{"v":"a"}}]`),
	}}
	a := newTestAgent(t, api)

	_, err := a.Send(context.Background(), "loop forever")
	if err == nil || !strings.Contains(err.Error(), "gave up") {
		t.Fatalf("err = %v", err)
	}
	if len(api.requests) != maxTurns {
		t.Fatalf("made %d requests, want %d", len(api.requests), maxTurns)
	}
}

func TestSendReportsRefusalAndMaxTokens(t *testing.T) {
	api := &fakeAPI{replies: []string{
		`{"id":"m","type":"message","role":"assistant","model":"fake","content":[],"stop_reason":"refusal",` +
			`"stop_details":{"type":"refusal","category":"cyber","explanation":"no"},"stop_sequence":null,` +
			`"usage":{"input_tokens":1,"output_tokens":1}}`,
		message("max_tokens", `[{"type":"text","text":"partial"}]`),
	}}
	a := newTestAgent(t, api)

	if _, err := a.Send(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("refusal err = %v", err)
	}
	got, err := a.Send(context.Background(), "y")
	if err != nil || !strings.Contains(got, "partial") || !strings.Contains(got, "max_tokens") {
		t.Fatalf("max_tokens reply = %q, %v", got, err)
	}
}

func TestSendForgetsRefusedExchange(t *testing.T) {
	api := &fakeAPI{replies: []string{
		`{"id":"m","type":"message","role":"assistant","model":"fake","content":[],"stop_reason":"refusal",` +
			`"stop_details":{"type":"refusal","category":"cyber","explanation":"no"},"stop_sequence":null,` +
			`"usage":{"input_tokens":1,"output_tokens":1}}`,
		message("end_turn", `[{"type":"text","text":"fine"}]`),
	}}
	a := newTestAgent(t, api)

	if _, err := a.Send(context.Background(), "first"); err == nil {
		t.Fatal("expected refusal error")
	}
	if _, err := a.Send(context.Background(), "second"); err != nil {
		t.Fatal(err)
	}
	msgs := api.requests[1]["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("request after refusal carried %d messages, want only the new one", len(msgs))
	}
}
