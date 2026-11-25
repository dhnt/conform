package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhnt/conform"
	"github.com/dhnt/conform/cert"
	"github.com/dhnt/conform/contract"
)

func TestCertPassAndSealedEvidence(t *testing.T) {
	contractFile, fakeBinDir := fixturePaths(t)
	fakeForm := filepath.Join(fakeBinDir, "form")
	outPath := filepath.Join(t.TempDir(), "nested", "evidence.json")

	var out, errb bytes.Buffer
	code := run([]string{
		"cert",
		"--contract", contractFile,
		"--reference-cmd", fakeForm,
		"--candidate-cmd", fakeForm,
		"--runs", "2",
		"--candidate-digest", "sha256:cand-digest-1",
		"--reference-digest", "sha256:ref-digest-1",
		"--out", outPath,
	}, &out, &errb)
	if code != 0 {
		t.Fatalf("code=%d want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errb.String())
	}
	if !strings.HasPrefix(out.String(), "PASS ") {
		t.Errorf("stdout = %q, want PASS prefix", out.String())
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read evidence.json: %v", err)
	}
	ok, err := cert.VerifyJSON(raw)
	if err != nil || !ok {
		t.Fatalf("VerifyJSON = (%v, %v), want (true, nil)", ok, err)
	}

	var rec cert.Evidence
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if !cert.Verify(rec) {
		t.Fatalf("cert.Verify(rec) = false, want true")
	}

	wantSHA, err := cert.SHA256File(contractFile)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ConformVersion != conform.Version {
		t.Errorf("ConformVersion = %q, want %q", rec.ConformVersion, conform.Version)
	}
	if rec.ContractSHA256 != wantSHA {
		t.Errorf("ContractSHA256 = %q, want %q", rec.ContractSHA256, wantSHA)
	}
	if rec.Candidate.Digest != "sha256:cand-digest-1" {
		t.Errorf("Candidate.Digest = %q, want sha256:cand-digest-1", rec.Candidate.Digest)
	}
	if rec.Reference.Digest != "sha256:ref-digest-1" {
		t.Errorf("Reference.Digest = %q, want sha256:ref-digest-1", rec.Reference.Digest)
	}
	if rec.Baseline == nil || rec.Baseline.Runs != 2 || len(rec.Baseline.Noisy) != 0 {
		t.Errorf("Baseline = %+v, want Runs=2 Noisy=[]", rec.Baseline)
	}
	if rec.Layers[cert.LayerCLI] != cert.PASS || rec.Verdict != cert.PASS {
		t.Errorf("Layers=%v Verdict=%s, want PASS", rec.Layers, rec.Verdict)
	}
	if len(rec.Results) != 9 {
		t.Fatalf("len(Results) = %d, want 9", len(rec.Results))
	}
	for _, r := range rec.Results {
		if r.Layer != cert.LayerCLI || r.Status != contract.PASS || r.Freshness != cert.Fresh {
			t.Errorf("unexpected result: %+v", r)
		}
	}
	if rec.Timestamp == "" || rec.Environment.OS == "" || rec.Environment.Arch == "" {
		t.Errorf("missing timestamp or env summary: ts=%q env=%+v", rec.Timestamp, rec.Environment)
	}
}

func TestCertFailingCandidate(t *testing.T) {
	tmp := t.TempDir()
	contractPath := filepath.Join(tmp, "CONTRACT.md")
	md := "### C-01\nEcho ok.\n```console\n$ demo check\nok\n```\nexit: 0\n"
	if err := os.WriteFile(contractPath, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	refBin := filepath.Join(tmp, "ref-demo")
	if err := os.WriteFile(refBin, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	candBin := filepath.Join(tmp, "cand-demo")
	if err := os.WriteFile(candBin, []byte("#!/bin/sh\necho wrong\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(tmp, "evidence.json")
	var out, errb bytes.Buffer
	code := run([]string{
		"cert",
		"--contract", contractPath,
		"--reference-cmd", refBin,
		"--candidate-cmd", candBin,
		"--runs", "2",
		"--out", outPath,
	}, &out, &errb)
	if code != 1 {
		t.Fatalf("code=%d want 1\nstdout:\n%s\nstderr:\n%s", code, out.String(), errb.String())
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var rec cert.Evidence
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	if !cert.Verify(rec) {
		t.Fatal("failed evidence record did not verify")
	}
	if rec.Verdict != cert.FAIL || rec.Layers[cert.LayerCLI] != cert.FAIL {
		t.Errorf("Verdict=%s Layers=%v, want FAIL", rec.Verdict, rec.Layers)
	}
}

func TestCertNoisyReferenceDowngradesToInconclusive(t *testing.T) {
	tmp := t.TempDir()
	contractPath := filepath.Join(tmp, "CONTRACT.md")
	md := "### N-01\nStable.\n```console\n$ demo stable\nok\n```\nexit: 0\n\n" +
		"### N-02\nFlaky.\n```console\n$ demo flaky\nok\n```\nexit: 0\n"
	if err := os.WriteFile(contractPath, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	// Reference script succeeds on first flaky call, fails on second flaky call.
	counterFile := filepath.Join(tmp, "count")
	refScript := "#!/bin/sh\n" +
		"if [ \"$1\" = \"stable\" ]; then echo ok; exit 0; fi\n" +
		"if [ -f " + shellQuote(counterFile) + " ]; then echo fail; exit 1; fi\n" +
		"touch " + shellQuote(counterFile) + "\n" +
		"echo ok\n"
	refBin := filepath.Join(tmp, "ref-flaky")
	if err := os.WriteFile(refBin, []byte(refScript), 0o755); err != nil {
		t.Fatal(err)
	}

	// Candidate passes stable, fails flaky -> should become INCONCLUSIVE, not FAIL.
	candScript := "#!/bin/sh\n" +
		"if [ \"$1\" = \"stable\" ]; then echo ok; exit 0; fi\n" +
		"echo fail; exit 1\n"
	candBin := filepath.Join(tmp, "cand-flaky")
	if err := os.WriteFile(candBin, []byte(candScript), 0o755); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	code := run([]string{
		"cert",
		"--contract", contractPath,
		"--reference-cmd", refBin,
		"--candidate-cmd", candBin,
		"--runs", "2",
	}, &out, &errb)
	if code != 1 {
		t.Fatalf("code=%d want 1\nstdout:\n%s\nstderr:\n%s", code, out.String(), errb.String())
	}

	ok, err := cert.VerifyJSON(out.Bytes())
	if err != nil || !ok {
		t.Fatalf("VerifyJSON(stdout) = (%v, %v), want (true, nil)", ok, err)
	}

	var rec cert.Evidence
	if err := json.Unmarshal(out.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Baseline == nil || len(rec.Baseline.Noisy) != 1 || rec.Baseline.Noisy[0] != "N-02" {
		t.Fatalf("Baseline = %+v, want Noisy=[N-02]", rec.Baseline)
	}
	if rec.Verdict != cert.INCONCLUSIVE {
		t.Errorf("Verdict = %s, want INCONCLUSIVE", rec.Verdict)
	}
	if len(rec.Results) != 2 || rec.Results[0].Status != contract.PASS || rec.Results[1].Status != contract.INCONCLUSIVE {
		t.Errorf("Results = %+v, want [PASS, INCONCLUSIVE]", rec.Results)
	}
}

func TestCertUsageAndErrors(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"cert", "--help"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "--reference-cmd") {
		t.Fatalf("cert --help: code=%d out=%q", code, out.String())
	}

	cases := [][]string{
		{"cert"},
		{"cert", "--contract", "a.md"},
		{"cert", "--contract", "a.md", "--reference-cmd", "true"},
		{"cert", "--contract", "a.md", "--reference-cmd", "true", "--candidate-cmd", "true", "--runs", "0"},
		{"cert", "--contract", "a.md", "--reference-cmd", "true", "--candidate-cmd", "true", "--unknown"},
		{"cert", "--contract", filepath.Join(t.TempDir(), "missing.md"), "--reference-cmd", "true", "--candidate-cmd", "true"},
	}
	for _, args := range cases {
		out.Reset()
		errb.Reset()
		if code := run(args, &out, &errb); code != 2 {
			t.Errorf("run(%v): code=%d, want 2 (stderr=%q)", args, code, errb.String())
		}
	}
}
