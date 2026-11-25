// Command conform is the CLI front door of the conform suite. bashy runs the
// same code in-process as `bashy conform` through package cli.
package main

import (
	"os"

	"github.com/dhnt/conform/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr)) }
