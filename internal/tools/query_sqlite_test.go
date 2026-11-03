package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func newTestDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE sessions (id INTEGER PRIMARY KEY, user TEXT, expires_at TEXT)`,
		`INSERT INTO sessions (user, expires_at) VALUES ('ana', '2026-01-01'), ('bo', NULL), ('cy', '2026-03-03')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func runQuery(t *testing.T, in querySQLiteInput) (string, error) {
	t.Helper()
	raw, _ := json.Marshal(in)
	return QuerySQLite{}.Run(context.Background(), raw)
}

func TestQuerySQLiteSelect(t *testing.T) {
	db := newTestDB(t)

	got, err := runQuery(t, querySQLiteInput{DB: db, SQL: "SELECT user, expires_at FROM sessions ORDER BY id"})
	if err != nil {
		t.Fatal(err)
	}
	want := "user\texpires_at\nana\t2026-01-01\nbo\tNULL\ncy\t2026-03-03"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestQuerySQLiteNoRows(t *testing.T) {
	got, err := runQuery(t, querySQLiteInput{DB: newTestDB(t), SQL: "SELECT user FROM sessions WHERE id > 99"})
	if err != nil || got != "user\n(no rows)" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestQuerySQLiteRowLimit(t *testing.T) {
	db := newTestDB(t)

	got, err := runQuery(t, querySQLiteInput{DB: db, SQL: "SELECT user FROM sessions", MaxRows: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "stopped at 2 rows") || strings.Contains(got, "cy") {
		t.Fatalf("got %q", got)
	}

	// Exactly at the limit is not truncated.
	got, _ = runQuery(t, querySQLiteInput{DB: db, SQL: "SELECT user FROM sessions", MaxRows: 3})
	if strings.Contains(got, "stopped at") {
		t.Fatalf("should not truncate at exact limit: %q", got)
	}
}

func TestQuerySQLiteRefusesWrites(t *testing.T) {
	db := newTestDB(t)
	for _, q := range []string{
		"DELETE FROM sessions",
		"INSERT INTO sessions (user) VALUES ('x')",
		"DROP TABLE sessions",
		"CREATE TABLE t (a)",
	} {
		if _, err := runQuery(t, querySQLiteInput{DB: db, SQL: q}); err == nil {
			t.Errorf("expected %q to fail", q)
		}
	}

	got, err := runQuery(t, querySQLiteInput{DB: db, SQL: "SELECT count(*) AS n FROM sessions"})
	if err != nil || got != "n\n3" {
		t.Fatalf("data changed or query failed: %q, %v", got, err)
	}
}

func TestQuerySQLiteDoesNotCreateMissingDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, err := runQuery(t, querySQLiteInput{DB: path, SQL: "SELECT 1"}); err == nil {
		t.Fatal("expected error for missing database")
	}
}

func TestQuerySQLiteErrors(t *testing.T) {
	db := newTestDB(t)
	if _, err := runQuery(t, querySQLiteInput{DB: db, SQL: "SELEC nope"}); err == nil {
		t.Fatal("expected syntax error")
	}
	if _, err := runQuery(t, querySQLiteInput{DB: db}); err == nil {
		t.Fatal("expected error for empty sql")
	}
}
