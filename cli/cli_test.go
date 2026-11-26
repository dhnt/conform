package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhnt/conform"
	"github.com/dhnt/conform/contract"
)

func fixturePaths(t *testing.T) (contractFile, fakeBinDir string) {
	t.Helper()
	contractFile = filepath.Join("..", "contract", "testdata", "form-v1", "CONTRACT.md")
	fakeBinDir = filepath.Join("..", "contract", "testdata", "fakecli", "bin")
	if _, err := os.Stat(contractFile); err != nil {
		t.Fatalf("stat contract fixture: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fakeBinDir, "form")); err != nil {
		t.Fatalf("stat fakecli fixture: %v", err)
	}
	return contractFile, fakeBinDir
}

func TestRun(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run([]string{"version"}, &out, &errb); code != 0 || strings.TrimSpace(out.String()) != conform.Version {
		t.Fatalf("version: code=%d out=%q", code, out.String())
	}
	if code := Run(nil, &out, &errb); code != 2 {
		t.Fatalf("no args: code=%d", code)
	}
	if code := Run([]string{"nope"}, &out, &errb); code != 2 {
		t.Fatalf("unknown: code=%d", code)
	}
	out.Reset()
	errb.Reset()
	if code := Run([]string{"help"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "run") {
		t.Fatalf("help: code=%d out=%q", code, out.String())
	}
}

func TestRunContractTextAndFlagsAfterFile(t *testing.T) {
	contractFile, fakeBinDir := fixturePaths(t)
	workDir := t.TempDir()

	var out, errb bytes.Buffer
	code := Run([]string{
		"run", contractFile,
		"--dir", workDir,
		"--path", fakeBinDir,
		"--shell", "/bin/sh -c",
		"--timeout", "10s",
		"--fresh-home",
		"--reset-cmd", "true",
		"--kill-strays",
	}, &out, &errb)
	if code != 0 {
		t.Fatalf("code=%d want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errb.String())
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 9 {
		t.Fatalf("got %d lines, want 9:\n%s", len(lines), out.String())
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "PASS F-0") {
			t.Errorf("line[%d] = %q, want PASS F-0*", i, line)
		}
	}
}

func TestRunContractJSONAndFlagsBeforeFile(t *testing.T) {
	contractFile, fakeBinDir := fixturePaths(t)
	workDir := t.TempDir()

	var out, errb bytes.Buffer
	code := Run([]string{
		"run",
		"--dir=" + workDir,
		"--path=" + fakeBinDir,
		"--timeout=10s",
		"--json",
		contractFile,
	}, &out, &errb)
	if code != 0 {
		t.Fatalf("code=%d want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errb.String())
	}

	var results []contract.Result
	if err := json.Unmarshal(out.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal json output: %v\nraw:\n%s", err, out.String())
	}
	if len(results) != 9 {
		t.Fatalf("got %d results, want 9", len(results))
	}
	for _, r := range results {
		if r.Status != contract.PASS {
			t.Errorf("%s: got %s (%s), want PASS", r.Name, r.Status, r.Detail)
		}
	}
}

func TestRunContractExit1OnFailAndInconclusive(t *testing.T) {
	_, fakeBinDir := fixturePaths(t)
	tmp := t.TempDir()

	t.Run("fail_exits_1", func(t *testing.T) {
		md := "### X-01\nMismatch.\n```console\n$ form list\nexpected-something-else\n```\nexit: 0\n"
		cpath := filepath.Join(tmp, "fail.md")
		if err := os.WriteFile(cpath, []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		code := Run([]string{"run", cpath, "--dir", t.TempDir(), "--path", fakeBinDir}, &out, &errb)
		if code != 1 {
			t.Fatalf("code=%d want 1; stdout=%q stderr=%q", code, out.String(), errb.String())
		}
		if !strings.HasPrefix(out.String(), "FAIL X-01 ") {
			t.Errorf("stdout = %q, want prefix 'FAIL X-01 '", out.String())
		}
	})

	t.Run("timeout_inconclusive_exits_1", func(t *testing.T) {
		md := "### T-01\nSleeps.\n```console\n$ form sleep 10\nawake\n```\nexit: 0\n"
		cpath := filepath.Join(tmp, "timeout.md")
		if err := os.WriteFile(cpath, []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		code := Run([]string{"run", cpath, "--dir", t.TempDir(), "--path", fakeBinDir, "--timeout", "150ms", "--json"}, &out, &errb)
		if code != 1 {
			t.Fatalf("code=%d want 1; stdout=%q stderr=%q", code, out.String(), errb.String())
		}
		var results []contract.Result
		if err := json.Unmarshal(out.Bytes(), &results); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(results) != 1 || results[0].Status != contract.INCONCLUSIVE {
			t.Errorf("results = %+v, want single INCONCLUSIVE", results)
		}
	})

	t.Run("bad_shell_inconclusive_exits_1", func(t *testing.T) {
		md := "### S-01\nEcho.\n```console\n$ echo ok\nok\n```\nexit: 0\n"
		cpath := filepath.Join(tmp, "shell.md")
		if err := os.WriteFile(cpath, []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		code := Run([]string{"run", cpath, "--shell", "/nonexistent/conform-shell"}, &out, &errb)
		if code != 1 {
			t.Fatalf("code=%d want 1; stdout=%q stderr=%q", code, out.String(), errb.String())
		}
		if !strings.HasPrefix(out.String(), "INCONCLUSIVE S-01 ") {
			t.Errorf("stdout = %q, want prefix 'INCONCLUSIVE S-01 '", out.String())
		}
	})
}

func TestRunContractUsageAndErrors(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run([]string{"run", "--help"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "--timeout") {
		t.Fatalf("run --help: code=%d out=%q", code, out.String())
	}

	cases := [][]string{
		{"run"},
		{"run", "a.md", "b.md"},
		{"run", "a.md", "--unknown"},
		{"run", "a.md", "--timeout", "notaduration"},
		{"run", "a.md", "--timeout", "0s"},
		{"run", "a.md", "--dir"},
		{"run", filepath.Join(t.TempDir(), "missing.md")},
	}
	for _, args := range cases {
		out.Reset()
		errb.Reset()
		if code := Run(args, &out, &errb); code != 2 {
			t.Errorf("Run(%v): code=%d, want 2 (stderr=%q)", args, code, errb.String())
		}
	}
}

func TestRunSpecAdvisory(t *testing.T) {
	for _, outcome := range []struct {
		expected string
		code     int
	}{{"ok", 0}, {"different", 1}} {
		for _, format := range []string{"--json=false", "--json"} {
			dir := t.TempDir()
			md := "Names, etc.\n### A-01\nPrint.\n```console\n$ echo ok\n" + outcome.expected + "\n```\nexit: 0\n"
			write := func(name, contents string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var baseline string
			run := func(name, note string) {
				t.Helper()
				var out, errb bytes.Buffer
				code := Run([]string{"run", filepath.Join(dir, name), format}, &out, &errb)
				if code != outcome.code || errb.String() != note {
					t.Fatalf("%s %s: code=%d stderr=%q, want %d and %q", name, format, code, errb.String(), outcome.code, note)
				}
				if baseline == "" {
					baseline = out.String()
					if baseline == "" || (format == "--json" && !json.Valid(out.Bytes())) {
						t.Fatalf("unexpected baseline output %q", baseline)
					}
				} else if out.String() != baseline {
					t.Fatalf("stdout=%q, want %q", out.String(), baseline)
				}
			}
			write("CONTRACT.md", md)
			run("CONTRACT.md", "") // Missing features.yaml.
			write("features.yaml", "- id: names\n  status: tbd\n")
			write("other.md", md)
			run("other.md", "")
			run("CONTRACT.md", "conform: note: spec check reports 2 finding(s) for this spec (run \"conform spec check "+dir+"\"); where a rule is underspecified, do not guess silently\n")
			write("CONTRACT.md", strings.Replace(md, "Names, etc.", "Names are ASCII letters.", 1))
			run("CONTRACT.md", "conform: note: spec check reports 1 finding(s) for this spec (run \"conform spec check "+dir+"\"); where a rule is underspecified, do not guess silently\n")
			dag, err := os.ReadFile(filepath.Join("..", "spec", "testdata", "form-spec-v4", "dag.md"))
			if err != nil {
				t.Fatal(err)
			}
			write("dag.md", string(dag))
			run("CONTRACT.md", "")
		}
	}
}
