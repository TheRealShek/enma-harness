package main

import (
	"context"
	"fmt"
	"os"

	"github.com/therealshek/enma-harness/cli"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "eval" {
		os.Exit(cli.RunEvalCommand(os.Args[2:]))
	}

	if err := cli.RunInteractive(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "Input error:", err)
		os.Exit(1)
	}
}
