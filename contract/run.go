package contract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout is the per-step timeout used when Env.Timeout is zero or negative.
const DefaultTimeout = 30 * time.Second

// Env configures how contract assertions are executed by Run.
type Env struct {
	// Dir is the working directory for commands. If empty, commands inherit the
	// current process working directory.
	Dir string
	// PathPrefix is prepended to PATH using os.PathListSeparator. If empty,
	// PATH is left unchanged.
	PathPrefix string
	// Extra holds additional environment variables in "KEY=VALUE" form. Entries
	// override inherited environment variables with the same key.
	Extra []string
	// Shell is the shell invocation prefix used to run each command string.
	// If empty, defaults to []string{"sh", "-c"}. If a single element is given
	// (e.g. []string{"/bin/bash"}), "-c" is appended automatically.
	Shell []string
	// Timeout is the per-step command timeout. If zero or negative, DefaultTimeout
	// is used.
	Timeout time.Duration
	// Bindings optionally supplies initial placeholder bindings. Run clones this
	// map before executing so the caller's Env is never mutated across runs.
	Bindings Bindings
}

// Run executes each assertion's commands sequentially using env and returns one
// Result per assertion in the same order. Placeholder bindings are threaded
// across passing assertions. Timeouts, context cancellations, and command start
// failures produce INCONCLUSIVE.
func Run(ctx context.Context, assertions []Assertion, env Env) []Result {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := env.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	shell := resolveShell(env.Shell)
	procEnv := buildProcessEnv(os.Environ(), env.Extra, env.PathPrefix)
	bindings := env.Bindings.Clone()

	results := make([]Result, 0, len(assertions))
	for _, a := range assertions {
		results = append(results, runAssertion(ctx, a, env.Dir, shell, procEnv, timeout, bindings))
	}
	return results
}

func runAssertion(
	ctx context.Context,
	a Assertion,
	dir string,
	shell []string,
	procEnv []string,
	timeout time.Duration,
	bindings Bindings,
) Result {
	if len(a.Commands) == 0 {
		return Result{
			Name:   a.Name,
			Status: INCONCLUSIVE,
			Detail: "assertion has no commands",
		}
	}

	var stdout, stderr string
	var exitCode int
	total := len(a.Commands)

	for i, rawCmd := range a.Commands {
		if err := ctx.Err(); err != nil {
			return Result{
				Name:   a.Name,
				Status: INCONCLUSIVE,
				Detail: fmt.Sprintf("canceled before running command: %v", err),
			}
		}

		cmdStr, subErr := Substitute(rawCmd, bindings)
		if subErr != nil {
			return Result{
				Name:   a.Name,
				Status: FAIL,
				Detail: subErr.Error(),
			}
		}

		out, errOut, code, status, detail := runStep(ctx, dir, shell, procEnv, timeout, cmdStr)
		if status != "" {
			return Result{
				Name:   a.Name,
				Status: status,
				Detail: detail,
			}
		}

		if i < total-1 && code != 0 {
			norm := Normalize(combineOutput(out, errOut))
			return Result{
				Name:   a.Name,
				Status: FAIL,
				Detail: fmt.Sprintf("step %d/%d exited %d (out: %s)", i+1, total, code, truncate(norm, 140)),
			}
		}
		stdout, stderr, exitCode = out, errOut, code
	}

	// Pass stdout only for --json assertions so stderr warnings do not break
	// JSON decoding; pass combined stdout+stderr for plain-text assertions
	// (e.g. error text written to stderr).
	checkOut := stdout
	if !a.IsJSON() {
		checkOut = combineOutput(stdout, stderr)
	} else if exitCode != a.Exit && strings.TrimSpace(stdout) == "" && stderr != "" {
		checkOut = stderr
	}
	return Check(a, checkOut, exitCode, bindings)
}

func runStep(
	ctx context.Context,
	dir string,
	shell []string,
	procEnv []string,
	timeout time.Duration,
	cmdStr string,
) (stdout, stderr string, exitCode int, status Status, detail string) {
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := make([]string, 0, len(shell))
	args = append(args, shell[1:]...)
	args = append(args, cmdStr)

	cmd := exec.CommandContext(stepCtx, shell[0], args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = procEnv
	configureCmd(cmd)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if err == nil {
		return stdout, stderr, 0, "", ""
	}

	if stepCtx.Err() != nil {
		if ctx.Err() != nil {
			return stdout, stderr, -1, INCONCLUSIVE, fmt.Sprintf("command canceled: %v", ctx.Err())
		}
		if errors.Is(stepCtx.Err(), context.DeadlineExceeded) {
			return stdout, stderr, -1, INCONCLUSIVE, fmt.Sprintf("command timed out after %s", timeout)
		}
		return stdout, stderr, -1, INCONCLUSIVE, fmt.Sprintf("command interrupted: %v", stepCtx.Err())
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout, stderr, exitErr.ExitCode(), "", ""
	}

	return stdout, stderr, -1, INCONCLUSIVE, fmt.Sprintf("failed to start command: %v", err)
}

func resolveShell(shell []string) []string {
	if len(shell) == 0 {
		return defaultShell()
	}
	if len(shell) == 1 {
		return []string{shell[0], "-c"}
	}
	cp := make([]string, len(shell))
	copy(cp, shell)
	return cp
}

func combineOutput(stdout, stderr string) string {
	if stderr == "" {
		return stdout
	}
	if stdout == "" {
		return stderr
	}
	if strings.HasSuffix(stdout, "\n") {
		return stdout + stderr
	}
	return stdout + "\n" + stderr
}

func buildProcessEnv(base, extra []string, pathPrefix string) []string {
	type entry struct {
		key string
		val string
	}
	order := make([]string, 0, len(base)+len(extra)+1)
	vals := make(map[string]entry, len(base)+len(extra)+1)
	set := func(kv string) {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return
		}
		if _, exists := vals[k]; !exists {
			order = append(order, k)
		}
		vals[k] = entry{key: k, val: v}
	}

	for _, kv := range base {
		set(kv)
	}
	for _, kv := range extra {
		set(kv)
	}

	if pathPrefix != "" {
		cur, exists := vals["PATH"]
		if !exists || cur.val == "" {
			set("PATH=" + pathPrefix)
		} else {
			set("PATH=" + pathPrefix + string(os.PathListSeparator) + cur.val)
		}
	}

	out := make([]string, 0, len(order))
	for _, k := range order {
		e := vals[k]
		out = append(out, e.key+"="+e.val)
	}
	return out
}
