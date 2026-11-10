# blamebot

A small terminal agent that finds what broke your service and blames it, loudly.
You describe the symptom; it reads logs, probes endpoints and queries the
database, then names the culprit with the evidence and what it would change.
It investigates only: the tools it has cannot modify the system.

```
what broke?> the /login endpoint is returning 500s
```

## Tools

| Tool           | What it does                                                       |
| -------------- | ------------------------------------------------------------------ |
| `tail_logs`    | Last N lines of a file, read backwards from the end                |
| `grep_logs`    | Regex search over a file or directory, with a match limit          |
| `http_probe`   | One GET/HEAD: status, latency, key headers, first 2 KB of the body |
| `query_sqlite` | Read-only SQL (`mode=ro` plus `query_only`), row-limited           |
| `run_command`  | Shell command, run only after you answer `y` at the prompt         |

Every tool caps its output so a noisy log can't flood the conversation, and says
so when it truncates.

## Run it

Requires Go 1.24+ and an `ANTHROPIC_API_KEY`.

```sh
go run ./cmd/blamebot                    # default model: claude-sonnet-5-5
go run ./cmd/blamebot --model claude-opus-5-5
```

Each tool call is echoed as a dimmed line on stderr, so you can see what the
agent is looking at as it goes. While it investigates it stays quiet and
precise; only the final verdict is loud. Pass `--no-color` (or set `NO_COLOR`)
to turn the highlighting off.

## The verdict

Every answer ends in one of two shapes. When the evidence names a culprit:

```
BLAME: <the thing that broke it>
<a blunt sentence or two on what it did>
Evidence: <log lines, responses, rows, with paths and line numbers>
Fix: <what a human should change>
```

When it can't prove a cause it says `NO ONE TO BLAME (YET)` and lists what it
ruled out, rather than guessing. It blames things (a migration, a config key, a
dependency), never people.

## Try it on a broken service

`examples/login500` is a tiny HTTP service with a planted bug: a deploy added a
`region` column to sessions in code, but the migration failed and was never
retried, so every login returns a bare `500 {"error":"internal error"}`. The
cause is only visible in the logs and the database.

```sh
go run ./examples/login500             # serves on 127.0.0.1:8080, data in examples/login500/data
go run ./cmd/blamebot
what broke?> /login on http://127.0.0.1:8080 is returning 500s. Logs and db are in examples/login500/data
```

A good investigation probes the endpoint, greps the log for errors, finds the
failed `0007_add_sessions_region` migration, and confirms it in
`schema_migrations`. The verdict should open along the lines of:

```
BLAME: migration 0007_add_sessions_region
It failed with "database is locked" during the 2.4.1 deploy, and startup carried on anyway.
```

(That is the intended shape, not a recorded run.) Delete
`examples/login500/data` to reset the service.

## Layout

```
cmd/blamebot/         flags, REPL, approval prompt, verdict highlighting
internal/agent/       conversation loop and system prompt
internal/tools/       Tool interface, registry, one file per tool
examples/login500/    the broken service
```

## Tests

```sh
go vet ./... && go test ./...
```

The tool tests run against temp files, an `httptest` server and a temp SQLite
database. The agent loop is tested against a scripted fake Messages API, so no
API key is needed.
