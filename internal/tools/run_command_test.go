package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func runCmd(t *testing.T, r RunCommand, command string) (string, error) {
	t.Helper()
	raw, _ := json.Marshal(runCommandInput{Command: command})
	return r.Run(context.Background(), raw)
}

func approve(string) bool { return true }

func TestRunCommandOutputAndExitCode(t *testing.T) {
	r := RunCommand{Confirm: approve}

	got, err := runCmd(t, r, "echo hello")
	if err != nil || got != "hello\n[exit code 0]" {
		t.Fatalf("got %q, %v", got, err)
	}

	got, err = runCmd(t, r, "echo oops >&2; exit 3")
	if err != nil || got != "oops\n[exit code 3]" {
		t.Fatalf("got %q, %v", got, err)
	}

	got, _ = runCmd(t, r, "true")
	if got != "(no output, exit code 0)" {
		t.Fatalf("got %q", got)
	}
}

func TestRunCommandNeedsApproval(t *testing.T) {
	var asked []string
	r := RunCommand{Confirm: func(c string) bool {
		asked = append(asked, c)
		return false
	}}

	marker := t.TempDir() + "/ran"
	got, err := runCmd(t, r, "touch "+marker)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "declined") {
		t.Fatalf("got %q", got)
	}
	if len(asked) != 1 || asked[0] != "touch "+marker {
		t.Fatalf("confirm saw %v", asked)
	}
	if out, _ := runCmd(t, RunCommand{Confirm: approve}, "test -e "+marker+" && echo exists"); strings.Contains(out, "exists") {
		t.Fatal("declined command still ran")
	}
}

func TestRunCommandWithoutConfirmNeverRuns(t *testing.T) {
	got, err := runCmd(t, RunCommand{}, "echo hi")
	if err != nil || !strings.Contains(got, "declined") {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestRunCommandTruncatesOutput(t *testing.T) {
	got, err := runCmd(t, RunCommand{Confirm: approve}, "yes x | head -c 50000")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "[output truncated, ") || len(got) > maxCommandOut+200 {
		t.Fatalf("output not capped: %d bytes", len(got))
	}
}

func TestRunCommandRejectsEmpty(t *testing.T) {
	if _, err := runCmd(t, RunCommand{Confirm: approve}, "  "); err == nil {
		t.Fatal("expected error")
	}
}
