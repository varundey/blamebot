package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func numberedLines(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	return sb.String()
}

func runTail(t *testing.T, path string, lines int) (string, error) {
	t.Helper()
	in, _ := json.Marshal(tailLogsInput{Path: path, Lines: lines})
	return TailLogs{}.Run(context.Background(), in)
}

func TestTailLogs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		lines   int
		want    string
	}{
		{"fewer lines than asked", "a\nb\n", 10, "a\nb"},
		{"exact count", "a\nb\nc\n", 3, "a\nb\nc"},
		{"last two", "a\nb\nc\nd\n", 2, "c\nd"},
		{"no trailing newline", "a\nb\nc", 2, "b\nc"},
		{"empty file", "", 5, "(file is empty)"},
		{"default when zero", numberedLines(60), 0, strings.TrimRight(numberedLines(60)[strings.Index(numberedLines(60), "line 11\n"):], "\n")},
		{"spans several read blocks", numberedLines(5000), 3, "line 4998\nline 4999\nline 5000"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runTail(t, writeFile(t, "app.log", tc.content), tc.lines)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTailLogsCapsLines(t *testing.T) {
	got, err := runTail(t, writeFile(t, "big.log", numberedLines(800)), 10000)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Split(got, "\n")); n != maxTailLines {
		t.Fatalf("got %d lines, want %d", n, maxTailLines)
	}
}

func TestTailLogsErrors(t *testing.T) {
	if _, err := runTail(t, filepath.Join(t.TempDir(), "nope.log"), 5); err == nil {
		t.Fatal("expected error for missing file")
	}
	if _, err := (TailLogs{}).Run(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error for missing path")
	}
	if _, err := (TailLogs{}).Run(context.Background(), json.RawMessage(`not json`)); err == nil {
		t.Fatal("expected error for bad json")
	}
}
