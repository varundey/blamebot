package main

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlantedBug(t *testing.T) {
	dir := t.TempDir()
	s, err := newServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.routes())
	defer srv.Close()

	get := func(path string) (int, string) {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, string(body)
	}

	if code, _ := get("/healthz"); code != 200 {
		t.Errorf("healthz = %d, want 200", code)
	}
	if code, _ := get("/login?user=nobody"); code != 401 {
		t.Errorf("unknown user = %d, want 401", code)
	}
	code, body := get("/login?user=ana")
	if code != 500 || strings.Contains(body, "region") {
		t.Errorf("login = %d %q, want a 500 that hides the cause", code, body)
	}

	logs, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logs), "migration failed version=0007_add_sessions_region") {
		t.Error("log is missing the failed migration line")
	}
	if !strings.Contains(string(logs), "table sessions has no column named region") {
		t.Errorf("log is missing the insert error:\n%s", tail(string(logs)))
	}

	db, err := sql.Open("sqlite", filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var status string
	if err := db.QueryRow(`SELECT status FROM schema_migrations WHERE version = '0007_add_sessions_region'`).Scan(&status); err != nil || status != "failed" {
		t.Errorf("migration 0007 status = %q, %v; want failed", status, err)
	}
}

func tail(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return strings.Join(lines, "\n")
}
