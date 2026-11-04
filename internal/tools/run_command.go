package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	commandTimeout = 30 * time.Second
	maxCommandOut  = 8000
)

// RunCommand runs a shell command, but only after Confirm approves it.
// It is the one tool that can change the system, so nothing runs without a
// human saying yes.
type RunCommand struct {
	// Confirm is shown the exact command and returns whether to run it.
	Confirm func(command string) bool
}

type runCommandInput struct {
	Command string `json:"command"`
}

func (RunCommand) Name() string { return "run_command" }

func (RunCommand) Description() string {
	return "Run a shell command (sh -c) and return its combined stdout/stderr and exit code. " +
		"The user is asked to approve every command before it runs and may decline. " +
		"Prefer the read-only tools (tail_logs, grep_logs, http_probe, query_sqlite) when they can answer the question; " +
		"use this for things like checking processes, disk space, or config. Commands time out after 30 seconds."
}

func (RunCommand) Schema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"command": map[string]any{"type": "string", "description": "The shell command to run."},
		},
		Required: []string{"command"},
	}
}

func (r RunCommand) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	var in runCommandInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if strings.TrimSpace(in.Command) == "" {
		return "", errors.New("command is required")
	}
	if r.Confirm == nil || !r.Confirm(in.Command) {
		return "The user declined to run this command. Do not retry it; try another approach or ask the user.", nil
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", in.Command)
	out := &cappedBuffer{limit: maxCommandOut}
	cmd.Stdout, cmd.Stderr = out, out
	cmd.WaitDelay = time.Second // don't hang on children that keep the pipe open

	err := cmd.Run()
	result := strings.TrimRight(out.String(), "\n")

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return result + fmt.Sprintf("\n[killed after %s timeout]", commandTimeout), nil
	case err == nil:
		if result == "" {
			return "(no output, exit code 0)", nil
		}
		return result + "\n[exit code 0]", nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return result + fmt.Sprintf("\n[exit code %d]", exitErr.ExitCode()), nil
	}
	return "", err
}

// cappedBuffer keeps the first limit bytes written and counts the rest.
type cappedBuffer struct {
	buf     bytes.Buffer
	limit   int
	dropped int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	room := c.limit - c.buf.Len()
	if room > len(p) {
		room = len(p)
	}
	if room > 0 {
		c.buf.Write(p[:room])
	}
	c.dropped += len(p) - room
	return len(p), nil
}

func (c *cappedBuffer) String() string {
	if c.dropped == 0 {
		return c.buf.String()
	}
	return c.buf.String() + fmt.Sprintf("\n[output truncated, %d more bytes]", c.dropped)
}
