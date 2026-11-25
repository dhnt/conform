// Command conform is the CLI front door of the conform suite. bashy exposes
// it as `bashy conform`.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/dhnt/conform"
)

const usage = `usage: conform <command> [args]

commands:
  version   print the suite version
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, conform.Version)
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "conform: unknown command %q\n%s", args[0], usage)
	return 2
}
