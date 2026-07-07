package main

import (
	"os"

	"github.com/stackradar/stackradar-cli/internal/cli"
)

func main() {
	command := cli.NewRootCommand(cli.Streams{
		Out: os.Stdout,
		Err: os.Stderr,
	})

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
