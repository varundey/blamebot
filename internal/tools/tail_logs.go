package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	defaultTailLines = 50
	maxTailLines     = 500
)

// TailLogs returns the last lines of a log file.
type TailLogs struct{}

type tailLogsInput struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
}

func (TailLogs) Name() string { return "tail_logs" }

func (TailLogs) Description() string {
	return "Return the last N lines of a log file. Use this first to see what a service was doing most recently. " +
		"Defaults to 50 lines, capped at 500."
}

func (TailLogs) Schema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"path":  map[string]any{"type": "string", "description": "Path to the log file."},
			"lines": map[string]any{"type": "integer", "description": "Number of trailing lines to return (default 50, max 500)."},
		},
		Required: []string{"path"},
	}
}

func (TailLogs) Run(_ context.Context, raw json.RawMessage) (string, error) {
	var in tailLogsInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.Path == "" {
		return "", errors.New("path is required")
	}
	n := in.Lines
	switch {
	case n <= 0:
		n = defaultTailLines
	case n > maxTailLines:
		n = maxTailLines
	}

	f, err := os.Open(in.Path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	out, err := lastLines(f, n)
	if err != nil {
		return "", err
	}
	if out == "" {
		return "(file is empty)", nil
	}
	return out, nil
}

// lastLines reads backwards from the end of f in blocks, so large logs are
// not read in full.
func lastLines(f *os.File, n int) (string, error) {
	const block = 8192

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return "", err
	}

	var buf []byte
	pos := size
	// Need n newlines plus one more to know where the first wanted line starts.
	for pos > 0 && bytes.Count(buf, []byte{'\n'}) <= n {
		step := int64(block)
		if pos < step {
			step = pos
		}
		pos -= step
		chunk := make([]byte, step)
		if _, err := f.ReadAt(chunk, pos); err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		buf = append(chunk, buf...)
	}

	buf = bytes.TrimRight(buf, "\n")
	lines := bytes.Split(buf, []byte{'\n'})
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return string(bytes.Join(lines, []byte{'\n'})), nil
}
