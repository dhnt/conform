package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/dhnt/conform/cert"
	"github.com/dhnt/conform/contract"
	"github.com/dhnt/conform/walk"
)

const mutateUsage = `usage: conform mutate CONTRACT.md [--path P] [--dir D] [--fresh-home] [--reset-cmd CMD] [--timeout D] [--kill-strays] [--shards N --shard K] [--json]

flags:
  --path P       directory or path list prepended to PATH
  --dir D        working directory for commands
  --fresh-home   use an empty HOME and TMPDIR per run
  --reset-cmd C  run C before each run
  --timeout D    per-step timeout (default 30s)
  --kill-strays  kill the step process group after each step
  --shards N     split the mutant set into N shards (default 1)
  --shard K      run only shard K (0 <= K < N): mutant i belongs to shard i mod N,
                 so N runs with K = 0..N-1 cover exactly the full set
  --json         emit cert.MutationReport
`

func runMutate(args []string, stdout, stderr io.Writer) int {
	args, shards, shard, err := parseShardFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "conform mutate: %v\n%s", err, mutateUsage)
		return 2
	}
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
	all := len(mutants)
	mutants = shardMutants(mutants, shards, shard)
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
		if shards > 1 {
			fmt.Fprintf(stdout, "shard %d/%d of %d mutants\n", shard, shards, all)
		}
		fmt.Fprintf(stdout, "strength %d/%d\n", report.Killed, report.Total)
	}
	if report.Killed != report.Total {
		return 1
	}
	return 0
}

// parseShardFlags removes --shards N and --shard K (also --flag=value) from
// args, so the flags shared with `conform run` stay unchanged.
func parseShardFlags(args []string) (rest []string, shards, shard int, err error) {
	shards = 1
	for i := 0; i < len(args); i++ {
		name, val, hasVal := strings.Cut(args[i], "=")
		if name != "--shards" && name != "--shard" {
			rest = append(rest, args[i])
			continue
		}
		if !hasVal {
			if i+1 >= len(args) {
				return nil, 0, 0, fmt.Errorf("flag %s requires a value", name)
			}
			i++
			val = args[i]
		}
		n, convErr := strconv.Atoi(val)
		if convErr != nil {
			return nil, 0, 0, fmt.Errorf("invalid value %q for %s", val, name)
		}
		if name == "--shards" {
			shards = n
		} else {
			shard = n
		}
	}
	if shards < 1 {
		return nil, 0, 0, fmt.Errorf("--shards must be at least 1, got %d", shards)
	}
	if shard < 0 || shard >= shards {
		return nil, 0, 0, fmt.Errorf("--shard must be in [0, %d), got %d", shards, shard)
	}
	return rest, shards, shard, nil
}

// shardMutants keeps mutant i when i mod shards == shard.
func shardMutants(mutants []cert.Mutant, shards, shard int) []cert.Mutant {
	if shards <= 1 {
		return mutants
	}
	var mine []cert.Mutant
	for i, m := range mutants {
		if i%shards == shard {
			mine = append(mine, m)
		}
	}
	return mine
}
