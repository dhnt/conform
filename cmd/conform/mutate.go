package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dhnt/conform/cert"
	"github.com/dhnt/conform/contract"
	"github.com/dhnt/conform/walk"
)

const mutateUsage = `usage: conform mutate CONTRACT.md [--path P] [--dir D] [--fresh-home] [--reset-cmd CMD] [--timeout D] [--kill-strays] [--json]

flags:
  --path P       directory or path list prepended to PATH
  --dir D        working directory for commands
  --fresh-home   use an empty HOME and TMPDIR per run
  --reset-cmd C  run C before each run
  --timeout D    per-step timeout (default 30s)
  --kill-strays  kill the step process group after each step
  --json         emit cert.MutationReport
`

func runMutate(args []string, stdout, stderr io.Writer) int {
	opts, err := parseRunArgs(args)
	if errors.Is(err, errHelp) {
		fmt.Fprint(stdout, mutateUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "conform mutate: %v\n%s", err, mutateUsage)
		return 2
	}
	f, err := os.Open(opts.contractPath)
	if err != nil {
		fmt.Fprintf(stderr, "conform mutate: %v\n", err)
		return 2
	}
	defer f.Close()
	assertions, err := contract.Parse(f)
	if err != nil {
		fmt.Fprintf(stderr, "conform mutate: parse: %v\n", err)
		return 2
	}
	observed := map[string]walk.InventoryResult{}
	env := contract.Env{
		Dir: opts.dir, PathPrefix: resolvePathList(opts.pathPrefix), Shell: parseShell(opts.shell),
		Timeout: opts.timeout, FreshHome: opts.freshHome, ResetCmd: opts.resetCmd, KillStrays: opts.killStrays,
		UIObserved: func(name string, _ []byte, inventory walk.InventoryResult) { observed[name] = inventory },
	}
	ctx := context.Background()
	baseline := contract.Run(ctx, assertions, env)
	var failures []string
	for _, r := range baseline {
		if r.Status != contract.PASS {
			failures = append(failures, r.Name)
		}
	}
	if len(failures) > 0 {
		fmt.Fprintf(stderr, "conform mutate: baseline not PASS: %s\n", strings.Join(failures, ", "))
		return 2
	}
	env.UIObserved = nil
	mutants := cert.DefaultMutants(assertions, walk.InventoryResult{})
	for _, a := range assertions {
		if a.Kind == "ui" {
			mutants = append(mutants, cert.DefaultUIMutants(a.Name, observed[a.Name])...)
		}
	}
	report, err := cert.Mutate(ctx, assertions, cert.Environment{CLI: env}, mutants)
	if err != nil {
		fmt.Fprintf(stderr, "conform mutate: %v\n", err)
		return 2
	}
	if opts.jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(stderr, "conform mutate: encode: %v\n", err)
			return 2
		}
	} else {
		for _, m := range report.Mutants {
			state := "SURVIVED"
			if m.Killed {
				state = "KILLED"
			}
			fmt.Fprintf(stdout, "%s %s %s\n", state, m.Layer, m.Name)
		}
		fmt.Fprintf(stdout, "strength %d/%d\n", report.Killed, report.Total)
	}
	if report.Killed != report.Total {
		return 1
	}
	return 0
}
