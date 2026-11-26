package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckFormSpecV1(t *testing.T) {
	findings, err := Check(filepath.Join("testdata", "form-spec-v1"))
	if err != nil {
		t.Fatalf("Check(form-spec-v1): %v", err)
	}
	want := [][2]string{
		{"closed-undeclared", "CONTRACT.md"},
		{"example-unreferenced", "examples/contact.yaml"},
		{"flag-undefined", "--fields"},
	}
	if len(findings) != len(want) {
		t.Fatalf("got %d findings, want %d: %+v", len(findings), len(want), findings)
	}
	for i, w := range want {
		if findings[i].Rule != w[0] || findings[i].Subject != w[1] {
			t.Errorf("finding[%d] = (%q, %q), want (%q, %q)", i, findings[i].Rule, findings[i].Subject, w[0], w[1])
		}
		if findings[i].Message == "" {
			t.Errorf("finding[%d] has empty message", i)
		}
	}

	// Verify closed-undeclared lists the exact closed assertions in form-spec-v1.
	closedMsg := findings[0].Message
	for _, name := range []string{"F-01", "F-02", "F-03", "F-04", "F-07"} {
		if !strings.Contains(closedMsg, name) {
			t.Errorf("closed-undeclared message %q missing assertion %s", closedMsg, name)
		}
	}
	for _, openOrOther := range []string{"F-05", "F-06", "F-08", "F-09", "U-01"} {
		if strings.Contains(closedMsg, openOrOther) {
			t.Errorf("closed-undeclared message %q unexpectedly lists %s", closedMsg, openOrOther)
		}
	}
}

func TestCheckFormSpecV2(t *testing.T) {
	findings, err := Check(filepath.Join("testdata", "form-spec-v2"))
	if err != nil {
		t.Fatalf("Check(form-spec-v2): %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("got findings %+v, want none", findings)
	}
}

func writeSpecDir(t *testing.T, contractMD, featuresYAML string, examples map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CONTRACT.md"), []byte(contractMD), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "features.yaml"), []byte(featuresYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, content := range examples {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRuleFeatureNoAcceptance(t *testing.T) {
	contractMD := "# Contract\n\nObjects without `\"...\":\"...\"` are closed.\n\n### A-01\nRun.\n```console\n$ app ping\nok\n```\nexit: 0\n"
	featuresYAML := strings.Join([]string{
		"- id: missing-block",
		"  status: verified",
		"  intent: No acceptance key at all.",
		"- id: empty-list",
		"  status: draft",
		"  acceptance: []",
		"- id: empty-section",
		"  status: verified",
		"  acceptance:",
		"- id: tbd-ok",
		"  status: tbd",
		"  intent: TBD features may omit acceptance.",
		"- id: verified-ok",
		"  status: verified",
		"  acceptance:",
		"    - app ping => ok",
		"",
	}, "\n")

	dir := writeSpecDir(t, contractMD, featuresYAML, nil)
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantSubjects := []string{"empty-list", "empty-section", "missing-block"}
	if len(findings) != len(wantSubjects) {
		t.Fatalf("got %+v, want subjects %v", findings, wantSubjects)
	}
	for i, sub := range wantSubjects {
		if findings[i].Rule != "feature-no-acceptance" || findings[i].Subject != sub {
			t.Errorf("finding[%d] = %+v, want feature-no-acceptance %s", i, findings[i], sub)
		}
	}
}

func TestRuleExampleUnreferenced(t *testing.T) {
	contractMD := strings.Join([]string{
		"# Contract",
		"",
		"Rules mention examples/orphan.yaml only in prose, not in a command.",
		"",
		"### A-01",
		"Uses examples/used.json (and prose mentions examples/orphan.yaml).",
		"```console",
		"$ app load examples/used.json",
		"ok",
		"```",
		"exit: 0",
		"",
	}, "\n")
	featuresYAML := "- id: load\n  status: verified\n  acceptance:\n    - app load examples/used.json => ok\n"
	examples := map[string]string{
		"examples/used.json":   "{}",
		"examples/orphan.yaml": "title: orphan\n",
	}

	dir := writeSpecDir(t, contractMD, featuresYAML, examples)
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Rule != "example-unreferenced" || findings[0].Subject != "examples/orphan.yaml" {
		t.Fatalf("got %+v, want single example-unreferenced examples/orphan.yaml", findings)
	}
}

func TestRuleFlagUndefined(t *testing.T) {
	contractMD := strings.Join([]string{
		"# Contract",
		"",
		"- `--json` emits JSON; `--format-long` is a different flag name; objects without `\"...\":\"...\"` are closed.",
		"",
		"### A-01",
		"Prose inside an assertion mentions `--format`.",
		"```console",
		"$ app add --format compact --only-two \"--quoted\" --json",
		"{\"ok\":true}",
		"```",
		"exit: 0",
		"",
		"### A-02",
		"List items.",
		"```console",
		"$ app list --format=compact --only-two '--quoted' --json",
		"{\"ok\":true}",
		"```",
		"exit: 0",
		"",
		"### A-03",
		"Show item.",
		"```console",
		"$ app show --format compact \"--quoted\" --json",
		"{\"ok\":true}",
		"```",
		"exit: 0",
		"",
	}, "\n")
	featuresYAML := "- id: core\n  status: verified\n  acceptance:\n    - app list => ok\n"

	dir := writeSpecDir(t, contractMD, featuresYAML, nil)
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Rule != "flag-undefined" || findings[0].Subject != "--format" {
		t.Fatalf("got %+v, want single flag-undefined --format", findings)
	}
}

func TestRuleClosedUndeclared(t *testing.T) {
	featuresYAML := "- id: core\n  status: verified\n  acceptance:\n    - app get => ok\n"

	// Word boundary check: "close" and "enclosed" are not the word "closed".
	undeclaredMD := strings.Join([]string{
		"# Contract",
		"",
		"- `--json` compares JSON; close_at and enclosed do not declare object openness.",
		"",
		"### A-01",
		"Closed output.",
		"```console",
		"$ app get --json",
		"{\"ok\":true}",
		"```",
		"exit: 0",
		"",
		"### A-02",
		"Open output (prose says closed here inside the assertion section, not rules text).",
		"```console",
		"$ app info --json",
		"{\"ok\":true,\"...\":\"...\"}",
		"```",
		"exit: 0",
		"",
	}, "\n")

	dir := writeSpecDir(t, undeclaredMD, featuresYAML, nil)
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Rule != "closed-undeclared" || findings[0].Subject != "CONTRACT.md" {
		t.Fatalf("got %+v, want single closed-undeclared CONTRACT.md", findings)
	}
	if !strings.Contains(findings[0].Message, "A-01") || strings.Contains(findings[0].Message, "A-02") {
		t.Errorf("unexpected closed-undeclared message: %q", findings[0].Message)
	}

	// Declaring "Closed" (case-insensitive) in a trailing non-assertion section clears the finding.
	declaredMD := undeclaredMD + "\n## Notes\nObjects without the open marker are Closed.\n"
	dir2 := writeSpecDir(t, declaredMD, featuresYAML, nil)
	findings2, err := Check(dir2)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings2) != 0 {
		t.Fatalf("got %+v, want no findings when Closed is in rules text", findings2)
	}
}

func TestRuleEnvelopeKeyMissing(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Contract\n\n- `--json` outputs are closed unless marked with `\"...\":\"...\"`.\n\n")
	for i := 1; i <= 9; i++ {
		status := "pending"
		if i == 9 {
			status = "done"
		}
		fmt.Fprintf(&b, "### A-%02d\nStep %d.\n```console\n$ app step%d --json\n{\"id\":\"{ID}\",\"schema_version\":\"app-v1\",\"status\":%q}\n```\nexit: 0\n\n", i, i, i, status)
	}
	// A-10 omits schema_version (9/10 = 90% carry "app-v1") and also omits id/status.
	b.WriteString("### A-10\nStep 10.\n```console\n$ app step10 --json\n{\"ok\":true}\n```\nexit: 0\n")

	featuresYAML := "- id: core\n  status: verified\n  acceptance:\n    - app step1 --json => ok\n"
	dir := writeSpecDir(t, b.String(), featuresYAML, nil)
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %+v, want 1 finding", findings)
	}
	if findings[0].Rule != "envelope-key-missing" || findings[0].Subject != "schema_version" {
		t.Fatalf("got %+v, want envelope-key-missing schema_version", findings[0])
	}
	if !strings.Contains(findings[0].Message, "A-10") {
		t.Errorf("message %q does not list missing assertion A-10", findings[0].Message)
	}
}

func TestCheckUnreadableOrInvalidInputs(t *testing.T) {
	if _, err := Check(""); err == nil {
		t.Error("Check(\"\") = nil, want error")
	}
	if _, err := Check(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("Check(missing) = nil, want error")
	}

	// Missing features.yaml.
	dirNoFeatures := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirNoFeatures, "CONTRACT.md"), []byte("### A-01\nOk.\n```console\n$ app ok\n```\nexit: 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(dirNoFeatures); err == nil {
		t.Error("Check(missing features.yaml) = nil, want error")
	}

	// Malformed CONTRACT.md.
	dirBadContract := writeSpecDir(t, "### A-01\nmissing fence\n", "- id: x\n  status: tbd\n", nil)
	if _, err := Check(dirBadContract); err == nil {
		t.Error("Check(bad CONTRACT.md) = nil, want error")
	}
}
