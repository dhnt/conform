package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhnt/conform/spec"
)

func TestSpecCheckCleanAndFindings(t *testing.T) {
	v1Dir := filepath.Join("..", "spec", "testdata", "form-spec-v1")
	v3Dir := filepath.Join("..", "spec", "testdata", "form-spec-v3")

	t.Run("v3_clean_text", func(t *testing.T) {
		var out, errb bytes.Buffer
		code := Run([]string{"spec", "check", v3Dir}, &out, &errb)
		if code != 0 {
			t.Fatalf("code=%d want 0; stdout=%q stderr=%q", code, out.String(), errb.String())
		}
		if out.Len() != 0 {
			t.Errorf("stdout=%q, want empty", out.String())
		}
	})

	t.Run("v3_clean_json", func(t *testing.T) {
		var out, errb bytes.Buffer
		code := Run([]string{"spec", "check", v3Dir, "--json"}, &out, &errb)
		if code != 0 {
			t.Fatalf("code=%d want 0; stdout=%q stderr=%q", code, out.String(), errb.String())
		}
		if strings.TrimSpace(out.String()) != "[]" {
			t.Fatalf("stdout=%q, want []", out.String())
		}
		var findings []spec.Finding
		if err := json.Unmarshal(out.Bytes(), &findings); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(findings) != 0 {
			t.Errorf("got %d findings, want 0", len(findings))
		}
	})

	t.Run("v1_findings_text", func(t *testing.T) {
		var out, errb bytes.Buffer
		code := Run([]string{"spec", "check", v1Dir}, &out, &errb)
		if code != 1 {
			t.Fatalf("code=%d want 1; stdout=%q stderr=%q", code, out.String(), errb.String())
		}
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		wantPrefixes := []string{
			"closed-undeclared CONTRACT.md: ",
			"example-unreferenced examples/contact.yaml: ",
			"flag-undefined --fields: ",
		}
		if len(lines) != len(wantPrefixes) {
			t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(wantPrefixes), out.String())
		}
		for i, prefix := range wantPrefixes {
			if !strings.HasPrefix(lines[i], prefix) {
				t.Errorf("line[%d] = %q, want prefix %q", i, lines[i], prefix)
			}
		}
	})

	t.Run("v1_findings_json", func(t *testing.T) {
		var out, errb bytes.Buffer
		code := Run([]string{"spec", "check", "--json", v1Dir}, &out, &errb)
		if code != 1 {
			t.Fatalf("code=%d want 1; stdout=%q stderr=%q", code, out.String(), errb.String())
		}
		var findings []spec.Finding
		if err := json.Unmarshal(out.Bytes(), &findings); err != nil {
			t.Fatalf("unmarshal: %v\nraw:\n%s", err, out.String())
		}
		if len(findings) != 3 {
			t.Fatalf("got %d findings, want 3: %+v", len(findings), findings)
		}
		if findings[0].Rule != "closed-undeclared" || findings[1].Rule != "example-unreferenced" || findings[2].Rule != "flag-undefined" {
			t.Errorf("unexpected order: %+v", findings)
		}
	})
}

func TestSpecCheckUsageAndErrors(t *testing.T) {
	v2Dir := filepath.Join("..", "spec", "testdata", "form-spec-v2")

	var out, errb bytes.Buffer
	if code := Run([]string{"help"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "spec") {
		t.Fatalf("help: code=%d out=%q", code, out.String())
	}
	out.Reset()
	errb.Reset()
	if code := Run([]string{"spec", "--help"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "check") {
		t.Fatalf("spec --help: code=%d out=%q", code, out.String())
	}
	out.Reset()
	errb.Reset()
	if code := Run([]string{"spec", "check", "--help"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "--json") {
		t.Fatalf("spec check --help: code=%d out=%q", code, out.String())
	}

	badCases := [][]string{
		{"spec"},
		{"spec", "unknown"},
		{"spec", "check"},
		{"spec", "check", v2Dir, "extra"},
		{"spec", "check", v2Dir, "--unknown"},
		{"spec", "check", v2Dir, "--json=nope"},
		{"spec", "check", filepath.Join(t.TempDir(), "missing")},
	}
	for _, args := range badCases {
		out.Reset()
		errb.Reset()
		if code := Run(args, &out, &errb); code != 2 {
			t.Errorf("Run(%v): code=%d, want 2 (stderr=%q)", args, code, errb.String())
		}
	}
}
