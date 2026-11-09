// Command login500 is a deliberately broken service for trying out blamebot.
//
// It serves GET /login?user=NAME and GET /healthz. Version 2.4.1 of the code
// stores a "region" with each session, but the migration that adds that
// column failed during deploy and was never retried, so every login returns
// a 500. The response body says nothing useful; the cause is only visible in
// the logs and the database.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type server struct {
	db  *sql.DB
	log *os.File
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	dir := flag.String("dir", "examples/login500/data", "directory for app.db and app.log")
	flag.Parse()

	s, err := newServer(*dir)
	if err != nil {
		log.Fatal(err)
	}
	s.logf("info", "service starting version=2.4.1 addr=%s", *addr)
	fmt.Printf("login500 listening on http://%s (data in %s)\n", *addr, *dir)
	log.Fatal(http.ListenAndServe(*addr, s.routes()))
}

// newServer opens the database and log in dir, seeding both on first run.
func newServer(dir string) (*server, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dbPath, logPath := filepath.Join(dir, "app.db"), filepath.Join(dir, "app.log")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		if err := seed(dbPath, logPath); err != nil {
			return nil, fmt.Errorf("seed: %w", err)
		}
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	lf, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	return &server{db: db, log: lf}, nil
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "ok") })
	mux.HandleFunc("/login", s.login)
	return mux
}

func (s *server) logf(level, format string, args ...any) {
	fmt.Fprintf(s.log, "%s level=%s %s\n", time.Now().UTC().Format(time.RFC3339), level, fmt.Sprintf(format, args...))
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	user := r.URL.Query().Get("user")
	var id int
	if err := s.db.QueryRow(`SELECT id FROM users WHERE name = ?`, user).Scan(&id); err != nil {
		s.logf("warn", "login rejected user=%q reason=unknown_user", user)
		http.Error(w, `{"error":"unknown user"}`, http.StatusUnauthorized)
		return
	}
	token := fmt.Sprintf("%016x", rand.Uint64())
	_, err := s.db.Exec(`INSERT INTO sessions (user_id, token, region, created_at) VALUES (?, ?, ?, ?)`,
		id, token, "ap-south-1", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		s.logf("error", "login failed user=%q err=%q", user, "insert session: "+err.Error())
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	s.logf("info", "login ok user=%q", user)
	fmt.Fprintf(w, `{"token":%q}`+"\n", token)
}

// seed creates the database as it looked after the bad deploy, and a log
// with a day of healthy traffic from version 2.4.0 followed by the deploy.
func seed(dbPath, logPath string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT UNIQUE NOT NULL)`,
		`CREATE TABLE sessions (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, token TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE schema_migrations (version TEXT PRIMARY KEY, status TEXT NOT NULL, applied_at TEXT, error TEXT)`,
		`INSERT INTO users (name) VALUES ('ana'), ('bo'), ('cy'), ('dee')`,
		`INSERT INTO schema_migrations VALUES
			('0005_add_users_index', 'applied', '2026-10-01T09:00:00Z', NULL),
			('0006_add_sessions_created_at', 'applied', '2026-10-20T09:00:00Z', NULL),
			('0007_add_sessions_region', 'failed', NULL, 'database is locked')`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
	}

	f, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer f.Close()

	rng := rand.New(rand.NewSource(7))
	users := []string{"ana", "bo", "cy", "dee"}
	t := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 300; i++ {
		t = t.Add(time.Duration(30+rng.Intn(120)) * time.Second)
		ts := t.Format(time.RFC3339)
		switch rng.Intn(6) {
		case 0:
			fmt.Fprintf(f, "%s level=info msg=healthz\n", ts)
		case 1:
			fmt.Fprintf(f, "%s level=warn login rejected user=%q reason=unknown_user\n", ts, "guest"+fmt.Sprint(rng.Intn(50)))
		default:
			fmt.Fprintf(f, "%s level=info login ok user=%q\n", ts, users[rng.Intn(len(users))])
		}
	}
	deploy := t.Add(20 * time.Minute)
	fmt.Fprintf(f, "%s level=info deploy started version=2.4.1\n", deploy.Format(time.RFC3339))
	fmt.Fprintf(f, "%s level=error migration failed version=0007_add_sessions_region err=%q\n",
		deploy.Add(3*time.Second).Format(time.RFC3339), "database is locked")
	fmt.Fprintf(f, "%s level=warn continuing startup despite failed migration\n", deploy.Add(3*time.Second).Format(time.RFC3339))
	return nil
}
