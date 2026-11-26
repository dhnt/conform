package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/dhnt/conform/spec"
)

const specUsage = `usage: conform spec <command> [args]

commands:
  check     check static soundness of a spec folder
`

const specCheckUsage = `usage: conform spec check DIR [--json]

flags:
  --json    emit findings as a JSON array
`

func runSpec(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "conform spec: missing subcommand\n%s", specUsage)
		return 2
	}
	switch args[0] {
	case "check":
		return runSpecCheck(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, specUsage)
		return 0
	default:
		fmt.Fprintf(stderr, "conform spec: unknown command %q\n%s", args[0], specUsage)
		return 2
	}
}

func runSpecCheck(args []string, stdout, stderr io.Writer) int {
	dir, jsonOut, err := parseSpecCheckArgs(args)
	if errors.Is(err, errHelp) {
		fmt.Fprint(stdout, specCheckUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "conform spec check: %v\n%s", err, specCheckUsage)
		return 2
	}

	findings, err := spec.Check(dir)
	if err != nil {
		fmt.Fprintf(stderr, "conform spec check: %v\n", err)
		return 2
	}
	if findings == nil {
		findings = []spec.Finding{}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Rule != findings[j].Rule {
			return findings[i].Rule < findings[j].Rule
		}
		if findings[i].Subject != findings[j].Subject {
			return findings[i].Subject < findings[j].Subject
		}
		return findings[i].Message < findings[j].Message
	})

	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(findings); err != nil {
			fmt.Fprintf(stderr, "conform spec check: encode json: %v\n", err)
			return 2
		}
	} else {
		for _, f := range findings {
			fmt.Fprintf(stdout, "%s %s: %s\n", f.Rule, f.Subject, singleLine(f.Message))
		}
	}

	if len(findings) > 0 {
		return 1
	}
	return 0
}

func parseSpecCheckArgs(args []string) (dir string, jsonOut bool, err error) {
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
			return "", false, errHelp
		case "json":
			if !hasVal {
				jsonOut = true
			} else {
				b, parseErr := strconv.ParseBool(val)
				if parseErr != nil {
					return "", false, fmt.Errorf("invalid value %q for --json: %w", val, parseErr)
				}
				jsonOut = b
			}
		default:
			return "", false, fmt.Errorf("unknown flag %q", arg)
		}
	}

	if len(pos) == 0 {
		return "", false, errors.New("missing spec directory")
	}
	if len(pos) > 1 {
		return "", false, fmt.Errorf("unexpected extra argument %q", pos[1])
	}
	return pos[0], jsonOut, nil
}
