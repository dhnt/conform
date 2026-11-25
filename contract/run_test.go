package contract_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dhnt/conform/contract"
)

func fakeCLIBinDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "fakecli", "bin"))
	if err != nil {
		t.Fatalf("abs fakecli bin: %v", err)
	}
	return abs
}

func TestRunFormV1FakeCLI(t *testing.T) {
	assertions := loadFormV1Contract(t)
	workDir := t.TempDir()

	results := contract.Run(context.Background(), assertions, contract.Env{
		Dir:        workDir,
		PathPrefix: fakeCLIBinDir(t),
		Timeout:    10 * time.Second,
	})

	if len(results) != len(assertions) {
		t.Fatalf("got %d results, want %d", len(results), len(assertions))
	}
	for i, res := range results {
		if res.Name != assertions[i].Name {
			t.Errorf("result[%d].Name = %q, want %q", i, res.Name, assertions[i].Name)
		}
		if res.Status != contract.PASS {
			t.Errorf("%s: got %s (%s), want PASS", res.Name, res.Status, res.Detail)
		}
	}
}

func TestRunBindingsAcrossSteps(t *testing.T) {
	md := `### B-01
Bind dynamic ID and PORT from the first command.
` + "```console" + `
$ form add --id ignored --open --json
{"id":"{ID}","inviteURL":"...","opened":true,"openedURL":"...","ownerKey":"{KEY}","schema_version":"bashy-form-v1","status":"pending","url":"http://localhost:{PORT}/?id={ID}"}
` + "```" + `
exit: 0

### B-02
Substitute bound {ID} into the next command and verify {ID} and {PORT} consistency.
` + "```console" + `
$ form open {ID} --json --fields id,status,url
{"id":"{ID}","schema_version":"bashy-form-v1","status":"pending","url":"http://localhost:{PORT}/?id={ID}"}
` + "```" + `
exit: 0

### B-03
Answer the dynamically bound form and verify status transitions.
` + "```console" + `
$ form answer {ID} --complete --json
{"id":"{ID}","schema_version":"bashy-form-v1","status":"answered"}
` + "```" + `
exit: 0
`
	assertions, err := contract.Parse(strings.NewReader(md))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	env := contract.Env{
		Dir:        t.TempDir(),
		PathPrefix: fakeCLIBinDir(t),
		Extra: []string{
			"FAKE_FORCE_ID=dyn-form-42",
			"FAKE_PORT=55123",
			"FAKE_KEY=secretKey9988",
		},
		Timeout: 10 * time.Second,
	}

	results := contract.Run(context.Background(), assertions, env)
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for _, r := range results {
		if r.Status != contract.PASS {
			t.Errorf("%s: got %s (%s), want PASS", r.Name, r.Status, r.Detail)
		}
	}

	// Re-running with different dynamic values using the same Env must not
	// leak bindings from the first run.
	env.Dir = t.TempDir()
	env.Extra = []string{
		"FAKE_FORCE_ID=dyn-form-99",
		"FAKE_PORT=55999",
		"FAKE_KEY=otherKey112233",
	}
	results2 := contract.Run(context.Background(), assertions, env)
	for _, r := range results2 {
		if r.Status != contract.PASS {
			t.Errorf("second run %s: got %s (%s), want PASS", r.Name, r.Status, r.Detail)
		}
	}
}

func TestRunUnboundPlaceholderAfterFailedBind(t *testing.T) {
	md := `### U-01
Fails so {ID} is not bound.
` + "```console" + `
$ form add --id bad-secret --json
{"id":"{ID}"}
` + "```" + `
exit: 0

### U-02
Cannot substitute {ID}, so fails cleanly.
` + "```console" + `
$ form open {ID} --json
{"id":"{ID}"}
` + "```" + `
exit: 0

### U-03
Independent step still runs after earlier failure.
` + "```console" + `
$ form list
` + "```" + `
exit: 0
`
	assertions, err := contract.Parse(strings.NewReader(md))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	results := contract.Run(context.Background(), assertions, contract.Env{
		Dir:        t.TempDir(),
		PathPrefix: fakeCLIBinDir(t),
		Timeout:    10 * time.Second,
	})
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if results[0].Status != contract.FAIL {
		t.Errorf("U-01: got %s (%s), want FAIL", results[0].Status, results[0].Detail)
	}
	if results[1].Status != contract.FAIL || !strings.Contains(results[1].Detail, "unbound placeholder") {
		t.Errorf("U-02: got %s (%s), want FAIL with unbound placeholder", results[1].Status, results[1].Detail)
	}
	if results[2].Status != contract.PASS {
		t.Errorf("U-03: got %s (%s), want PASS", results[2].Status, results[2].Detail)
	}
}

func TestRunOutputStreamsJSONvsPlainText(t *testing.T) {
	md := `### S-01
JSON assertion ignores stderr warnings and parses stdout only.
` + "```console" + `
$ form add --id stream-test --open --json
{"id":"{ID}","inviteURL":"...","opened":true,"openedURL":"...","ownerKey":"{KEY}","schema_version":"bashy-form-v1","status":"pending","url":"http://localhost:{PORT}/?id={ID}"}
` + "```" + `
exit: 0

### S-02
Plain-text assertion receives stderr error output.
` + "```console" + `
$ form add --id bad-secret --json
error: Secret fields are refused here...
` + "```" + `
exit: 1

### S-03
Plain-text assertion receives combined stdout and stderr.
` + "```console" + `
$ form both
stdout-line
stderr-line
` + "```" + `
exit: 0
`
	assertions, err := contract.Parse(strings.NewReader(md))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	results := contract.Run(context.Background(), assertions, contract.Env{
		Dir:        t.TempDir(),
		PathPrefix: fakeCLIBinDir(t),
		Extra:      []string{"FAKE_JSON_STDERR=1"},
		Timeout:    10 * time.Second,
	})
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for _, r := range results {
		if r.Status != contract.PASS {
			t.Errorf("%s: got %s (%s), want PASS", r.Name, r.Status, r.Detail)
		}
	}
}

func TestRunTimeoutInconclusive(t *testing.T) {
	assertions := []contract.Assertion{
		{
			Name:     "T-01",
			Commands: []string{"form sleep 10"},
			Expected: "awake",
			Exit:     0,
		},
		{
			Name:     "T-02",
			Commands: []string{"form list"},
			Expected: "",
			Exit:     0,
		},
	}

	start := time.Now()
	results := contract.Run(context.Background(), assertions, contract.Env{
		Dir:        t.TempDir(),
		PathPrefix: fakeCLIBinDir(t),
		Timeout:    150 * time.Millisecond,
	})
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Fatalf("timeout took %s; child process group was not killed promptly", elapsed)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Status != contract.INCONCLUSIVE || !strings.Contains(results[0].Detail, "timed out") {
		t.Errorf("T-01: got %s (%s), want INCONCLUSIVE with timed out", results[0].Status, results[0].Detail)
	}
	if results[1].Status != contract.PASS {
		t.Errorf("T-02: got %s (%s), want PASS after prior timeout", results[1].Status, results[1].Detail)
	}
}

func TestRunContextCanceledInconclusive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	assertions := []contract.Assertion{
		{Name: "C-01", Commands: []string{"form sleep 10"}, Exit: 0},
		{Name: "C-02", Commands: []string{"form list"}, Exit: 0},
	}

	results := contract.Run(ctx, assertions, contract.Env{
		Dir:        t.TempDir(),
		PathPrefix: fakeCLIBinDir(t),
		Timeout:    5 * time.Second,
	})
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Status != contract.INCONCLUSIVE {
			t.Errorf("%s: got %s (%s), want INCONCLUSIVE", r.Name, r.Status, r.Detail)
		}
	}
}

func TestRunStartFailureInconclusive(t *testing.T) {
	t.Run("missing_shell", func(t *testing.T) {
		assertions := []contract.Assertion{
			{Name: "E-01", Commands: []string{"echo hi"}, Expected: "hi", Exit: 0},
		}
		results := contract.Run(context.Background(), assertions, contract.Env{
			Shell:   []string{"/nonexistent/conform-shell-binary"},
			Timeout: 2 * time.Second,
		})
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1", len(results))
		}
		if results[0].Status != contract.INCONCLUSIVE || !strings.Contains(results[0].Detail, "failed to start") {
			t.Errorf("got %+v, want INCONCLUSIVE with failed to start", results[0])
		}
	})

	t.Run("missing_dir", func(t *testing.T) {
		assertions := []contract.Assertion{
			{Name: "E-02", Commands: []string{"echo hi"}, Expected: "hi", Exit: 0},
		}
		results := contract.Run(context.Background(), assertions, contract.Env{
			Dir:     filepath.Join(t.TempDir(), "does-not-exist"),
			Timeout: 2 * time.Second,
		})
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1", len(results))
		}
		if results[0].Status != contract.INCONCLUSIVE {
			t.Errorf("got %+v, want INCONCLUSIVE", results[0])
		}
	})
}

func TestRunMultiCommandAssertion(t *testing.T) {
	t.Run("setup_then_check_pass", func(t *testing.T) {
		assertions := []contract.Assertion{
			{
				Name: "M-01",
				Commands: []string{
					"form add --id multi1 --open --json",
					"form open multi1 --json",
				},
				Expected: `{"id":"{ID}","schema_version":"bashy-form-v1","status":"pending","url":"http://localhost:{PORT}/?id={ID}"}`,
				Exit:     0,
			},
			{
				Name: "M-02",
				Commands: []string{
					"form answer {ID} --complete --json",
					"form wait {ID} --json",
				},
				Expected: `{"id":"{ID}","schema_version":"bashy-form-v1","status":"answered"}`,
				Exit:     0,
			},
		}
		results := contract.Run(context.Background(), assertions, contract.Env{
			Dir:        t.TempDir(),
			PathPrefix: fakeCLIBinDir(t),
			Timeout:    10 * time.Second,
		})
		for _, r := range results {
			if r.Status != contract.PASS {
				t.Errorf("%s: got %s (%s), want PASS", r.Name, r.Status, r.Detail)
			}
		}
	})

	t.Run("setup_step_failure_fails_assertion", func(t *testing.T) {
		assertions := []contract.Assertion{
			{
				Name: "M-03",
				Commands: []string{
					"form add --id bad-secret --json",
					"form list",
				},
				Expected: "",
				Exit:     0,
			},
		}
		results := contract.Run(context.Background(), assertions, contract.Env{
			Dir:        t.TempDir(),
			PathPrefix: fakeCLIBinDir(t),
			Timeout:    10 * time.Second,
		})
		if results[0].Status != contract.FAIL || !strings.Contains(results[0].Detail, "step 1/2") {
			t.Errorf("M-03: got %+v, want FAIL citing step 1/2", results[0])
		}
	})
}

func TestRunEnvDirExtraAndCustomShell(t *testing.T) {
	workDir := t.TempDir()
	realDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	// Custom shell wrapper script that injects an extra marker variable.
	shellDir := t.TempDir()
	customShell := filepath.Join(shellDir, "mysh")
	script := "#!/bin/sh\nexport CONFORM_SHELL_USED=yes\nexec /bin/sh \"$@\"\n"
	if err := os.WriteFile(customShell, []byte(script), 0o755); err != nil {
		t.Fatalf("write custom shell: %v", err)
	}

	assertions := []contract.Assertion{
		{
			Name:     "ENV-01",
			Commands: []string{"form pwd"},
			Expected: realDir,
			Exit:     0,
		},
		{
			Name:     "ENV-02",
			Commands: []string{"form env CUSTOM_VAR"},
			Expected: "hello-conform",
			Exit:     0,
		},
		{
			Name:     "ENV-03",
			Commands: []string{"form env CONFORM_SHELL_USED"},
			Expected: "yes",
			Exit:     0,
		},
	}

	results := contract.Run(context.Background(), assertions, contract.Env{
		Dir:        workDir,
		PathPrefix: fakeCLIBinDir(t),
		Extra:      []string{"CUSTOM_VAR=hello-conform"},
		Shell:      []string{customShell}, // single-element form appends -c
		Timeout:    10 * time.Second,
	})
	for _, r := range results {
		if r.Status != contract.PASS {
			t.Errorf("%s: got %s (%s), want PASS", r.Name, r.Status, r.Detail)
		}
	}
}
