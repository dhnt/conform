package main

import (
	"bytes"
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
	"github.com/dhnt/conform/cert"
	"github.com/dhnt/conform/contract"
)

const certUsage = `usage: conform cert --contract C --reference-cmd CMD --candidate-cmd CMD [--runs N] [--out evidence.json] [flags]

flags:
  --contract C            path to CONTRACT.md (required)
  --reference-cmd CMD     command for the reference implementation (required)
  --candidate-cmd CMD     command for the candidate implementation (required)
  --runs N                number of reference runs for noise baseline (default 2)
  --out PATH              write sealed evidence JSON to PATH (default stdout)
  --candidate-digest D    caller-supplied candidate identity digest
  --reference-digest D    caller-supplied reference identity digest
  --bin NAME              command name to shim on PATH (default inferred from contract)
  --dir D                 working directory (default isolated temp dir per run)
  --shell S               shell used to run contract commands (default "sh -c")
  --timeout D             per-step command timeout (default 30s)
`

type certOpts struct {
	contractPath    string
	referenceCmd    string
	candidateCmd    string
	runs            int
	outPath         string
	candidateDigest string
	referenceDigest string
	binName         string
	dir             string
	shell           string
	timeout         time.Duration
}

func runCert(args []string, stdout, stderr io.Writer) int {
	opts, err := parseCertArgs(args)
	if errors.Is(err, errHelp) {
		fmt.Fprint(stdout, certUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "conform cert: %v\n%s", err, certUsage)
		return 2
	}

	contractBytes, err := os.ReadFile(opts.contractPath)
	if err != nil {
		fmt.Fprintf(stderr, "conform cert: %v\n", err)
		return 2
	}

	assertions, err := contract.Parse(bytes.NewReader(contractBytes))
	if err != nil {
		fmt.Fprintf(stderr, "conform cert: parse %s: %v\n", opts.contractPath, err)
		return 2
	}

	binName := opts.binName
	if binName == "" {
		binName = inferBinName(assertions)
	}

	refShimDir, refIdent, err := prepareCommand(binName, opts.referenceCmd, opts.referenceDigest)
	if err != nil {
		fmt.Fprintf(stderr, "conform cert: reference-cmd: %v\n", err)
		return 2
	}
	if refShimDir != "" {
		defer os.RemoveAll(refShimDir)
	}

	candShimDir, candIdent, err := prepareCommand(binName, opts.candidateCmd, opts.candidateDigest)
	if err != nil {
		fmt.Fprintf(stderr, "conform cert: candidate-cmd: %v\n", err)
		return 2
	}
	if candShimDir != "" {
		defer os.RemoveAll(candShimDir)
	}

	shell := parseShell(opts.shell)
	ctx := context.Background()

	refRuns := make([][]contract.Result, 0, opts.runs)
	for i := 0; i < opts.runs; i++ {
		runDir, cleanup, err := resolveRunDir(opts.dir)
		if err != nil {
			fmt.Fprintf(stderr, "conform cert: create workdir: %v\n", err)
			return 2
		}
		env := contract.Env{
			Dir:        runDir,
			PathPrefix: refShimDir,
			Shell:      shell,
			Timeout:    opts.timeout,
		}
		res := contract.Run(ctx, assertions, env)
		cleanup()
		refRuns = append(refRuns, res)
	}

	baseline := cert.Baseline(refRuns)

	candDir, cleanupCand, err := resolveRunDir(opts.dir)
	if err != nil {
		fmt.Fprintf(stderr, "conform cert: create workdir: %v\n", err)
		return 2
	}
	candResults := contract.Run(ctx, assertions, contract.Env{
		Dir:        candDir,
		PathPrefix: candShimDir,
		Shell:      shell,
		Timeout:    opts.timeout,
	})
	cleanupCand()

	gradedCLI, cliVerdict := cert.Grade(candResults, baseline)
	layers := map[cert.Layer]cert.Verdict{
		cert.LayerCLI: cliVerdict,
	}

	shellName := opts.shell
	if shellName == "" {
		shellName = "sh"
	}

	rec := cert.Evidence{
		ConformVersion: conform.Version,
		ContractSHA256: cert.SHA256Bytes(contractBytes),
		Candidate:      candIdent,
		Reference:      refIdent,
		Environment:    cert.CurrentEnv(shellName),
		Baseline:       &baseline,
		Layers:         layers,
		Verdict:        cert.CombineLayers(layers),
		Results:        cert.ResultsForLayer(cert.LayerCLI, gradedCLI, cert.Fresh),
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}
	rec.ApplySeal()

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rec); err != nil {
		fmt.Fprintf(stderr, "conform cert: encode evidence: %v\n", err)
		return 2
	}

	if opts.outPath != "" {
		if dir := filepath.Dir(opts.outPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fmt.Fprintf(stderr, "conform cert: create output directory: %v\n", err)
				return 2
			}
		}
		if err := os.WriteFile(opts.outPath, buf.Bytes(), 0o644); err != nil {
			fmt.Fprintf(stderr, "conform cert: write %s: %v\n", opts.outPath, err)
			return 2
		}
		fmt.Fprintf(stdout, "%s %s\n", rec.Verdict, opts.outPath)
	} else {
		if _, err := stdout.Write(buf.Bytes()); err != nil {
			fmt.Fprintf(stderr, "conform cert: write stdout: %v\n", err)
			return 2
		}
	}

	if rec.Verdict != cert.PASS {
		return 1
	}
	return 0
}

func parseCertArgs(args []string) (certOpts, error) {
	opts := certOpts{
		runs:    2,
		timeout: contract.DefaultTimeout,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				return opts, fmt.Errorf("unexpected positional argument %q", args[i+1])
			}
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			return opts, fmt.Errorf("unexpected positional argument %q", arg)
		}

		trimmed := strings.TrimPrefix(arg, "--")
		if trimmed == arg {
			trimmed = strings.TrimPrefix(arg, "-")
		}
		name, val, hasVal := strings.Cut(trimmed, "=")

		switch name {
		case "h", "help":
			return opts, errHelp
		case "contract", "reference-cmd", "candidate-cmd", "runs", "out",
			"candidate-digest", "reference-digest", "bin", "dir", "shell", "timeout":
			if !hasVal {
				if i+1 >= len(args) {
					return opts, fmt.Errorf("flag --%s requires a value", name)
				}
				i++
				val = args[i]
			}
			switch name {
			case "contract":
				opts.contractPath = val
			case "reference-cmd":
				opts.referenceCmd = val
			case "candidate-cmd":
				opts.candidateCmd = val
			case "runs":
				n, err := strconv.Atoi(val)
				if err != nil || n < 1 {
					return opts, fmt.Errorf("invalid --runs %q (must be integer >= 1)", val)
				}
				opts.runs = n
			case "out":
				opts.outPath = val
			case "candidate-digest":
				opts.candidateDigest = val
			case "reference-digest":
				opts.referenceDigest = val
			case "bin":
				opts.binName = val
			case "dir":
				opts.dir = val
			case "shell":
				opts.shell = val
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

	if opts.contractPath == "" {
		return opts, errors.New("missing required --contract")
	}
	if opts.referenceCmd == "" {
		return opts, errors.New("missing required --reference-cmd")
	}
	if opts.candidateCmd == "" {
		return opts, errors.New("missing required --candidate-cmd")
	}
	return opts, nil
}

func inferBinName(assertions []contract.Assertion) string {
	for _, a := range assertions {
		for _, cmd := range a.Commands {
			for _, tok := range splitWords(cmd) {
				if isEnvAssignment(tok) {
					continue
				}
				base := filepath.Base(tok)
				if base != "" && base != "." && base != "/" {
					return base
				}
			}
		}
	}
	return ""
}

func prepareCommand(binName, cmdStr, suppliedDigest string) (shimDir string, ident cert.Identity, err error) {
	tokens := splitWords(cmdStr)
	if len(tokens) == 0 {
		return "", cert.Identity{}, errors.New("empty command")
	}

	var envAssigns []string
	idx := 0
	for idx < len(tokens) && isEnvAssignment(tokens[idx]) {
		envAssigns = append(envAssigns, tokens[idx])
		idx++
	}
	if idx >= len(tokens) {
		return "", cert.Identity{}, errors.New("command has only environment assignments")
	}

	cmdTokens := make([]string, len(tokens)-idx)
	copy(cmdTokens, tokens[idx:])

	// Resolve relative paths on disk to absolute paths so shims work from isolated temp workdirs.
	var resolvedFile string
	for i, tok := range cmdTokens {
		if i > 1 {
			break
		}
		if info, statErr := os.Stat(tok); statErr == nil && !info.IsDir() {
			if abs, absErr := filepath.Abs(tok); absErr == nil {
				cmdTokens[i] = abs
				if resolvedFile == "" || i == 0 {
					resolvedFile = abs
				}
			}
		}
	}

	ident.Name = filepath.Base(cmdTokens[0])
	if suppliedDigest != "" {
		ident.Digest = suppliedDigest
	} else if resolvedFile != "" {
		h, hErr := cert.SHA256File(resolvedFile)
		if hErr != nil {
			return "", cert.Identity{}, fmt.Errorf("hash executable: %w", hErr)
		}
		ident.Digest = "sha256:" + h
	} else {
		sanitized := make([]string, len(cmdTokens))
		copy(sanitized, cmdTokens)
		sanitized[0] = filepath.Base(sanitized[0])
		ident.Digest = "sha256:" + cert.SHA256Bytes([]byte(strings.Join(sanitized, " ")))
	}

	if binName == "" {
		return "", ident, nil
	}

	tmpDir, err := os.MkdirTemp("", "conform-shim-*")
	if err != nil {
		return "", cert.Identity{}, err
	}

	var script strings.Builder
	script.WriteString("#!/bin/sh\nexec ")
	if len(envAssigns) > 0 {
		script.WriteString("env ")
		for _, kv := range envAssigns {
			script.WriteString(shellQuote(kv))
			script.WriteByte(' ')
		}
	}
	for _, tok := range cmdTokens {
		script.WriteString(shellQuote(tok))
		script.WriteByte(' ')
	}
	script.WriteString("\"$@\"\n")

	shimPath := filepath.Join(tmpDir, binName)
	if err := os.WriteFile(shimPath, []byte(script.String()), 0o755); err != nil {
		os.RemoveAll(tmpDir)
		return "", cert.Identity{}, err
	}
	return tmpDir, ident, nil
}

func resolveRunDir(dir string) (string, func(), error) {
	if dir != "" {
		return dir, func() {}, nil
	}
	tmp, err := os.MkdirTemp("", "conform-work-*")
	if err != nil {
		return "", nil, err
	}
	return tmp, func() { os.RemoveAll(tmp) }, nil
}

func isEnvAssignment(tok string) bool {
	k, _, ok := strings.Cut(tok, "=")
	if !ok || k == "" {
		return false
	}
	for i, r := range k {
		if i == 0 && !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func splitWords(s string) []string {
	var words []string
	var cur strings.Builder
	inToken := false
	var quote rune

	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inToken = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inToken {
				words = append(words, cur.String())
				cur.Reset()
				inToken = false
			}
		default:
			cur.WriteRune(r)
			inToken = true
		}
	}
	if inToken {
		words = append(words, cur.String())
	}
	return words
}
