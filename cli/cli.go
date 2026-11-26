// Package cli is the conform command line: cmd/conform and `bashy conform`
// both call Run, so the suite behaves the same wherever it is invoked.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dhnt/conform"
	"github.com/dhnt/conform/contract"
)

const usage = `usage: conform <command> [args]

commands:
  run       execute a CONTRACT.md suite
  cert      certify a candidate against a reference and write sealed evidence
  mutate    measure contract mutation strength
  spec      check static soundness of a spec folder
  version   print the suite version
`

const runUsage = `usage: conform run CONTRACT.md [--dir D] [--path P] [--shell S] [--timeout 30s] [--fresh-home] [--reset-cmd CMD] [--kill-strays] [--json]

flags:
  --dir D        working directory for commands
  --path P       directory or path list prepended to PATH
  --shell S      shell used to run commands (default "sh -c")
  --timeout D    per-step command timeout (default 30s)
  --fresh-home   use an empty HOME and TMPDIR per run
  --reset-cmd C  run C before each run
  --kill-strays  kill the step process group after each step
  --json         emit results as a JSON array
`

var errHelp = errors.New("help requested")

type runOpts struct {
	contractPath string
	dir          string
	pathPrefix   string
	shell        string
	timeout      time.Duration
	jsonOut      bool
	freshHome    bool
	resetCmd     string
	killStrays   bool
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "run":
		return runContract(args[1:], stdout, stderr)
	case "cert":
		return runCert(args[1:], stdout, stderr)
	case "mutate":
		return runMutate(args[1:], stdout, stderr)
	case "spec":
		return runSpec(args[1:], stdout, stderr)
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

func runContract(args []string, stdout, stderr io.Writer) int {
	opts, err := parseRunArgs(args)
	if errors.Is(err, errHelp) {
		fmt.Fprint(stdout, runUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "conform run: %v\n%s", err, runUsage)
		return 2
	}

	f, err := os.Open(opts.contractPath)
	if err != nil {
		fmt.Fprintf(stderr, "conform run: %v\n", err)
		return 2
	}
	defer f.Close()

	assertions, err := contract.Parse(f)
	if err != nil {
		fmt.Fprintf(stderr, "conform run: parse %s: %v\n", opts.contractPath, err)
		return 2
	}

	env := contract.Env{
		Dir:        opts.dir,
		PathPrefix: resolvePathList(opts.pathPrefix),
		Shell:      parseShell(opts.shell),
		Timeout:    opts.timeout,
		FreshHome:  opts.freshHome,
		ResetCmd:   opts.resetCmd,
		KillStrays: opts.killStrays,
	}

	results := contract.Run(context.Background(), assertions, env)
	if results == nil {
		results = []contract.Result{}
	}

	if opts.jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			fmt.Fprintf(stderr, "conform run: encode json: %v\n", err)
			return 2
		}
	} else {
		for _, r := range results {
			detail := singleLine(r.Detail)
			if detail == "" {
				fmt.Fprintf(stdout, "%s %s\n", r.Status, r.Name)
			} else {
				fmt.Fprintf(stdout, "%s %s %s\n", r.Status, r.Name, detail)
			}
		}
	}

	for _, r := range results {
		if r.Status != contract.PASS {
			return 1
		}
	}
	return 0
}

func parseRunArgs(args []string) (runOpts, error) {
	opts := runOpts{
		timeout: contract.DefaultTimeout,
	}
	var pos []string
	flagsDone := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if flagsDone || arg == "-" || !strings.HasPrefix(arg, "-") {
			pos = append(pos, arg)
			continue
		}
		if arg == "--" {
			flagsDone = true
			continue
		}

		trimmed := strings.TrimPrefix(arg, "--")
		if trimmed == arg {
			trimmed = strings.TrimPrefix(arg, "-")
		}
		name, val, hasVal := strings.Cut(trimmed, "=")

		switch name {
		case "h", "help":
			return opts, errHelp
		case "json":
			if !hasVal {
				opts.jsonOut = true
			} else {
				b, err := strconv.ParseBool(val)
				if err != nil {
					return opts, fmt.Errorf("invalid value %q for --json: %w", val, err)
				}
				opts.jsonOut = b
			}
		case "fresh-home", "kill-strays":
			b := true
			if hasVal {
				var err error
				b, err = strconv.ParseBool(val)
				if err != nil {
					return opts, fmt.Errorf("invalid value %q for --%s: %w", val, name, err)
				}
			}
			if name == "fresh-home" {
				opts.freshHome = b
			} else {
				opts.killStrays = b
			}
		case "dir", "path", "shell", "timeout", "reset-cmd":
			if !hasVal {
				if i+1 >= len(args) {
					return opts, fmt.Errorf("flag --%s requires a value", name)
				}
				i++
				val = args[i]
			}
			switch name {
			case "dir":
				opts.dir = val
			case "path":
				opts.pathPrefix = val
			case "shell":
				opts.shell = val
			case "reset-cmd":
				opts.resetCmd = val
			case "timeout":
				d, err := time.ParseDuration(val)
				if err != nil {
					return opts, fmt.Errorf("invalid --timeout %q: %w", val, err)
				}
				if d <= 0 {
					return opts, fmt.Errorf("--timeout must be positive, got %s", d)
				}
				opts.timeout = d
			}
		default:
			return opts, fmt.Errorf("unknown flag %q", arg)
		}
	}

	if len(pos) == 0 {
		return opts, errors.New("missing CONTRACT.md path")
	}
	if len(pos) > 1 {
		return opts, fmt.Errorf("unexpected extra argument %q", pos[1])
	}
	opts.contractPath = pos[0]
	return opts, nil
}

func parseShell(s string) []string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	if len(fields) == 1 {
		return []string{fields[0], "-c"}
	}
	return fields
}

func resolvePathList(p string) string {
	if p == "" {
		return ""
	}
	parts := filepath.SplitList(p)
	for i, part := range parts {
		if part == "" || filepath.IsAbs(part) {
			continue
		}
		if abs, err := filepath.Abs(part); err == nil {
			parts[i] = abs
		}
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
