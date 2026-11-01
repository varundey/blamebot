package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runGrep(t *testing.T, in grepLogsInput) (string, error) {
	t.Helper()
	raw, _ := json.Marshal(in)
	return GrepLogs{}.Run(context.Background(), raw)
}

func TestGrepLogsSingleFile(t *testing.T) {
	p := writeFile(t, "app.log", "INFO ok\nERROR db timeout\nINFO ok\nERROR disk full\n")

	got, err := runGrep(t, grepLogsInput{Pattern: "ERROR", Path: p})
	if err != nil {
		t.Fatal(err)
	}
	want := p + ":2:ERROR db timeout\n" + p + ":4:ERROR disk full"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGrepLogsDirectoryIsRecursive(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "a.log"), []byte("needle\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "b.log"), []byte("hay\nneedle\n"), 0o644)

	got, err := runGrep(t, grepLogsInput{Pattern: "needle", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, "needle") != 2 || !strings.Contains(got, "b.log:2:needle") {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestGrepLogsNoMatch(t *testing.T) {
	got, err := runGrep(t, grepLogsInput{Pattern: "zzz", Path: writeFile(t, "a.log", "abc\n")})
	if err != nil || got != "no matches" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestGrepLogsLimit(t *testing.T) {
	p := writeFile(t, "a.log", strings.Repeat("hit\n", 10))

	got, err := runGrep(t, grepLogsInput{Pattern: "hit", Path: p, MaxMatches: 3})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(got, "hit\n") + strings.Count(got, ":hit"); n < 3 {
		t.Fatalf("expected 3 matches, got %q", got)
	}
	if !strings.Contains(got, "stopped at 3 matches") {
		t.Fatalf("missing truncation note: %q", got)
	}
}

func TestGrepLogsExactLimitIsNotTruncated(t *testing.T) {
	p := writeFile(t, "a.log", strings.Repeat("hit\n", 3))

	got, err := runGrep(t, grepLogsInput{Pattern: "hit", Path: p, MaxMatches: 3})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "stopped at") {
		t.Fatalf("should not report truncation when matches == limit: %q", got)
	}
}

func TestGrepLogsSkipsBinaryAndTruncatesLongLines(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bin.dat"), []byte("x\x00needle\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "long.log"), []byte("needle"+strings.Repeat("x", 1000)+"\n"), 0o644)

	got, err := runGrep(t, grepLogsInput{Pattern: "needle", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "bin.dat") {
		t.Fatalf("binary file should be skipped: %q", got)
	}
	if !strings.HasSuffix(got, "...") || len(got) > len(dir)+maxLineLen+40 {
		t.Fatalf("long line not truncated: %d bytes", len(got))
	}
}

func TestGrepLogsErrors(t *testing.T) {
	p := writeFile(t, "a.log", "x\n")
	if _, err := runGrep(t, grepLogsInput{Pattern: "(", Path: p}); err == nil {
		t.Fatal("expected error for bad regex")
	}
	if _, err := runGrep(t, grepLogsInput{Pattern: "x", Path: p + ".missing"}); err == nil {
		t.Fatal("expected error for missing path")
	}
	if _, err := runGrep(t, grepLogsInput{Pattern: "x"}); err == nil {
		t.Fatal("expected error for missing path field")
	}
}
