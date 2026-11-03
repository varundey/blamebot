package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

const (
	defaultRows = 50
	maxRows     = 200
	maxCellLen  = 200
)

// QuerySQLite runs a read-only query against a SQLite database file.
type QuerySQLite struct{}

type querySQLiteInput struct {
	DB      string `json:"db"`
	SQL     string `json:"sql"`
	MaxRows int    `json:"max_rows"`
}

func (QuerySQLite) Name() string { return "query_sqlite" }

func (QuerySQLite) Description() string {
	return "Run a read-only SQL query against a SQLite database file and return rows as tab-separated text with a header. " +
		"The database is opened read-only, so writes fail. To discover the schema, query sqlite_master. " +
		"Defaults to 50 rows, capped at 200."
}

func (QuerySQLite) Schema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"db":       map[string]any{"type": "string", "description": "Path to the SQLite database file."},
			"sql":      map[string]any{"type": "string", "description": "A single SELECT (or PRAGMA/EXPLAIN) statement."},
			"max_rows": map[string]any{"type": "integer", "description": "Maximum rows to return (default 50, max 200)."},
		},
		Required: []string{"db", "sql"},
	}
}

func (QuerySQLite) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	var in querySQLiteInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.DB == "" || strings.TrimSpace(in.SQL) == "" {
		return "", errors.New("db and sql are required")
	}
	limit := in.MaxRows
	switch {
	case limit <= 0:
		limit = defaultRows
	case limit > maxRows:
		limit = maxRows
	}

	// mode=ro refuses writes at the connection level; query_only is a second guard.
	dsn := (&url.URL{Scheme: "file", Opaque: in.DB, RawQuery: "mode=ro&_pragma=query_only(1)"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return "", err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, in.SQL)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(strings.Join(cols, "\t"))

	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	n := 0
	for rows.Next() {
		if n == limit {
			fmt.Fprintf(&sb, "\n[stopped at %d rows; add a LIMIT or WHERE clause for more]", limit)
			return sb.String(), nil
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		cells := make([]string, len(vals))
		for i, v := range vals {
			cells[i] = formatCell(v)
		}
		sb.WriteString("\n" + strings.Join(cells, "\t"))
		n++
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if n == 0 {
		sb.WriteString("\n(no rows)")
	}
	return sb.String(), nil
}

func formatCell(v any) string {
	var s string
	switch x := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		s = string(x)
	default:
		s = fmt.Sprint(x)
	}
	s = strings.NewReplacer("\t", " ", "\n", " ").Replace(s)
	if len(s) > maxCellLen {
		s = s[:maxCellLen] + "..."
	}
	return s
}
