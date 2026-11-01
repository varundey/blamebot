package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	defaultMatches = 100
	maxMatches     = 500
	maxLineLen     = 400
)

// GrepLogs searches a log file, or every file under a directory, for a regex.
type GrepLogs struct{}

type grepLogsInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	MaxMatches int    `json:"max_matches"`
}

func (GrepLogs) Name() string { return "grep_logs" }

func (GrepLogs) Description() string {
	return "Search log files for lines matching a regular expression (Go RE2 syntax). " +
		"Path may be a file or a directory, which is searched recursively. " +
		"Matches come back as path:line:text. Defaults to 100 matches, capped at 500."
}

func (GrepLogs) Schema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"pattern":     map[string]any{"type": "string", "description": "Regular expression to search for. Prefix with (?i) for case-insensitive."},
			"path":        map[string]any{"type": "string", "description": "A log file or a directory of logs."},
			"max_matches": map[string]any{"type": "integer", "description": "Stop after this many matching lines (default 100, max 500)."},
		},
		Required: []string{"pattern", "path"},
	}
}

func (GrepLogs) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	var in grepLogsInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.Pattern == "" || in.Path == "" {
		return "", errors.New("pattern and path are required")
	}
	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return "", fmt.Errorf("bad pattern: %w", err)
	}
	limit := in.MaxMatches
	switch {
	case limit <= 0:
		limit = defaultMatches
	case limit > maxMatches:
		limit = maxMatches
	}

	var (
		hits      []string
		truncated bool
	)
	err = filepath.WalkDir(in.Path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		more, err := grepFile(p, re, limit-len(hits), &hits)
		if err != nil {
			return err
		}
		if more {
			truncated = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	if len(hits) == 0 {
		return "no matches", nil
	}
	out := strings.Join(hits, "\n")
	if truncated {
		out += fmt.Sprintf("\n[stopped at %d matches; narrow the pattern or path for more]", limit)
	}
	return out, nil
}

// grepFile appends up to room matches from the file to hits. It reports
// whether the file had further matches beyond room.
func grepFile(path string, re *regexp.Regexp, room int, hits *[]string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)

	for n := 1; sc.Scan(); n++ {
		line := sc.Bytes()
		if n == 1 && bytes.IndexByte(line, 0) >= 0 {
			return false, nil // binary file
		}
		if !re.Match(line) {
			continue
		}
		if room <= 0 {
			return true, nil
		}
		text := string(line)
		if len(text) > maxLineLen {
			text = text[:maxLineLen] + "..."
		}
		*hits = append(*hits, fmt.Sprintf("%s:%d:%s", path, n, text))
		room--
	}
	return false, sc.Err()
}
