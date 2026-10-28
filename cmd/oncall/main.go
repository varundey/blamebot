package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/varundey/oncall/internal/agent"
)

const defaultModel = "claude-sonnet-5-5"

func main() {
	model := flag.String("model", defaultModel, "Claude model to use")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "oncall (model: %s). Describe the problem, or type 'exit' to quit.\n", *model)

	a := agent.New(*model)
	ctx := context.Background()

	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\x1b[1mpage>\x1b[0m ")
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
}
