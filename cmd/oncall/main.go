package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

const defaultModel = "claude-sonnet-5-5"

func main() {
	model := flag.String("model", defaultModel, "Claude model to use")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "oncall (model: %s). Describe the problem, or type 'exit' to quit.\n", *model)

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
		fmt.Printf("(not wired up yet) you said: %s\n", line)
	}
}
