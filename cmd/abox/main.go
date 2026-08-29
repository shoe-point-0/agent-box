package main

import (
	"fmt"
	"io"
	"os"

	"github.com/shoe-point-0/agent-box/internal/version"
)

const name = "abox"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		if _, err := fmt.Fprintln(stdout, version.Current().String(name)); err != nil {
			return 1
		}
		return 0
	}

	_, _ = fmt.Fprintln(stderr, "abox: no product commands are implemented yet; use --version")
	return 2
}
