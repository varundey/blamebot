package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/varundey/blamebot/internal/agent"
	"github.com/varundey/blamebot/internal/tools"
)

const defaultModel = "claude-sonnet-5-5"

func main() {
	model := flag.String("model", defaultModel, "Claude model to use")
	flag.Parse()

	// One scanner serves both the prompt and the approval questions, so the
	// two never fight over buffered stdin.
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 64*1024), 4*1024*1024) // allow pasted log excerpts

	reg := tools.NewRegistry(
		tools.TailLogs{},
		tools.GrepLogs{},
		tools.NewHTTPProbe(),
		tools.QuerySQLite{},
		tools.RunCommand{Confirm: func(command string) bool {
			fmt.Printf("\x1b[33mrun command?\x1b[0m %s [y/N] ", command)
			if !in.Scan() {
				return false
			}
			answer := strings.ToLower(strings.TrimSpace(in.Text()))
			return answer == "y" || answer == "yes"
		}},
	)
	a := agent.New(*model, reg)
	a.Trace = os.Stderr
	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "blamebot (model: %s). Describe the problem, or type 'exit' to quit.\n", *model)
	for {
		fmt.Print("\x1b[1mwhat broke?>\x1b[0m ")
		if !in.Scan() {
			break
		}
		line := strings.TrimSpace(in.Text())
		switch line {
		case "":
			continue
		case "exit", "quit":
			return
		}
		reply, err := a.Send(ctx, line)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			continue
		}
		fmt.Println(reply)
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading input:", err)
		os.Exit(1)
	}
}
