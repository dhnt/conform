package cert

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dhnt/conform"
	"github.com/dhnt/conform/contract"
)

type baselineFixture struct {
	ReferenceRuns [][]contract.Result `json:"reference_runs"`
	ExpectedNoisy []string            `json:"expected_noisy"`
	Candidates    []struct {
		Label            string                     `json:"label"`
		Results          []contract.Result          `json:"results"`
		ExpectedVerdict  Verdict                    `json:"expected_verdict"`
		ExpectedStatuses map[string]contract.Status `json:"expected_statuses"`
	} `json:"candidates"`
}

func TestBaselineAndGradeFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "baseline_runs.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx baselineFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	nb := Baseline(fx.ReferenceRuns)
	if nb.Runs != len(fx.ReferenceRuns) {
		t.Fatalf("Runs = %d, want %d", nb.Runs, len(fx.ReferenceRuns))
	}
	if !reflect.DeepEqual(nb.Noisy, fx.ExpectedNoisy) {
		t.Fatalf("Noisy = %v, want %v", nb.Noisy, fx.ExpectedNoisy)
	}
	if nb.IsNoisy("F-01") || nb.IsNoisy("F-04") {
		t.Errorf("stable assertions F-01/F-04 unexpectedly marked noisy")
	}
	for _, name := range fx.ExpectedNoisy {
		if !nb.IsNoisy(name) {
			t.Errorf("expected %s to be marked noisy", name)
		}
	}

	for _, tc := range fx.Candidates {
		t.Run(tc.Label, func(t *testing.T) {
			graded, verdict := Grade(tc.Results, nb)
			if verdict != tc.ExpectedVerdict {
				t.Fatalf("verdict = %s, want %s (graded=%+v)", verdict, tc.ExpectedVerdict, graded)
			}
			got := make(map[string]contract.Status, len(graded))
			for _, r := range graded {
				got[r.Name] = r.Status
			}
			if !reflect.DeepEqual(got, tc.ExpectedStatuses) {
				t.Fatalf("statuses = %v, want %v", got, tc.ExpectedStatuses)
			}
		})
	}
}

func TestBaselineEdgeCases(t *testing.T) {
	t.Run("empty_runs", func(t *testing.T) {
		nb := Baseline(nil)
		if nb.Runs != 0 || len(nb.Noisy) != 0 {
			t.Fatalf("got %+v, want 0 runs and empty noisy", nb)
		}
	})

	t.Run("missing_in_one_reference_run_marks_noisy", func(t *testing.T) {
		runs := [][]contract.Result{
			{
				{Name: "A-01", Status: contract.PASS, Detail: "ok"},
				{Name: "A-02", Status: contract.PASS, Detail: "ok"},
			},
			{
				{Name: "A-01", Status: contract.PASS, Detail: "ok"},
			},
		}
		nb := Baseline(runs)
		if !reflect.DeepEqual(nb.Noisy, []string{"A-02"}) {
			t.Fatalf("Noisy = %v, want [A-02]", nb.Noisy)
		}
	})

	t.Run("three_runs_third_disagrees", func(t *testing.T) {
		runs := [][]contract.Result{
			{{Name: "A-01", Status: contract.PASS}, {Name: "A-02", Status: contract.PASS}},
			{{Name: "A-01", Status: contract.PASS}, {Name: "A-02", Status: contract.PASS}},
			{{Name: "A-01", Status: contract.PASS}, {Name: "A-02", Status: contract.FAIL}},
		}
		nb := Baseline(runs)
		if nb.Runs != 3 || !reflect.DeepEqual(nb.Noisy, []string{"A-02"}) {
			t.Fatalf("got %+v, want Runs=3 Noisy=[A-02]", nb)
		}
	})

	t.Run("candidate_inconclusive_stays_inconclusive_even_when_not_noisy", func(t *testing.T) {
		runs := [][]contract.Result{
			{{Name: "A-01", Status: contract.PASS}},
			{{Name: "A-01", Status: contract.PASS}},
		}
		nb := Baseline(runs)
		graded, verdict := Grade([]contract.Result{
			{Name: "A-01", Status: contract.INCONCLUSIVE, Detail: "command timed out"},
		}, nb)
		if verdict != INCONCLUSIVE {
			t.Fatalf("verdict = %s, want INCONCLUSIVE", verdict)
		}
		if len(graded) != 1 || graded[0].Status != contract.INCONCLUSIVE {
			t.Fatalf("graded = %+v, want INCONCLUSIVE", graded)
		}
	})

	t.Run("noisy_detail_mentions_baseline", func(t *testing.T) {
		nb := NoiseBaseline{Runs: 2, Noisy: []string{"A-01"}}
		graded, verdict := Grade([]contract.Result{
			{Name: "A-01", Status: contract.FAIL, Detail: "exit 1 != expected 0"},
		}, nb)
		if verdict != INCONCLUSIVE {
			t.Fatalf("verdict = %s, want INCONCLUSIVE", verdict)
		}
		if !strings.Contains(graded[0].Detail, "reference-vs-reference") || !strings.Contains(graded[0].Detail, "exit 1 != expected 0") {
			t.Errorf("detail = %q, want mention of baseline and original reason", graded[0].Detail)
		}
	})
}

func TestCombineVerdictsAND(t *testing.T) {
	cases := []struct {
		name     string
		verdicts []Verdict
		want     Verdict
	}{
		{"empty", nil, INCONCLUSIVE},
		{"single_pass", []Verdict{PASS}, PASS},
		{"single_fail", []Verdict{FAIL}, FAIL},
		{"single_inconclusive", []Verdict{INCONCLUSIVE}, INCONCLUSIVE},
		{"all_pass", []Verdict{PASS, PASS, PASS}, PASS},
		{"pass_and_fail", []Verdict{PASS, FAIL}, FAIL},
		{"fail_and_pass", []Verdict{FAIL, PASS}, FAIL},
		{"pass_and_inconclusive", []Verdict{PASS, INCONCLUSIVE}, INCONCLUSIVE},
		{"inconclusive_and_pass", []Verdict{INCONCLUSIVE, PASS}, INCONCLUSIVE},
		{"fail_dominates_inconclusive", []Verdict{INCONCLUSIVE, FAIL, PASS}, FAIL},
		{"inconclusive_after_fail", []Verdict{FAIL, INCONCLUSIVE}, FAIL},
		{"unknown_status_fails_closed", []Verdict{PASS, Verdict("")}, INCONCLUSIVE},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CombineVerdicts(tc.verdicts...); got != tc.want {
				t.Errorf("CombineVerdicts(%v) = %s, want %s", tc.verdicts, got, tc.want)
			}
		})
	}
}

func TestCombineLayersCLIAndUI(t *testing.T) {
	cases := []struct {
		name   string
		layers map[Layer]Verdict
		want   Verdict
	}{
		{"empty", nil, INCONCLUSIVE},
		{"cli_pass_ui_pass", map[Layer]Verdict{LayerCLI: PASS, LayerUI: PASS}, PASS},
		{"cli_pass_ui_fail", map[Layer]Verdict{LayerCLI: PASS, LayerUI: FAIL}, FAIL},
		{"cli_fail_ui_pass", map[Layer]Verdict{LayerCLI: FAIL, LayerUI: PASS}, FAIL},
		{"cli_pass_ui_inconclusive", map[Layer]Verdict{LayerCLI: PASS, LayerUI: INCONCLUSIVE}, INCONCLUSIVE},
		{"cli_inconclusive_ui_pass", map[Layer]Verdict{LayerCLI: INCONCLUSIVE, LayerUI: PASS}, INCONCLUSIVE},
		{"cli_inconclusive_ui_fail", map[Layer]Verdict{LayerCLI: INCONCLUSIVE, LayerUI: FAIL}, FAIL},
		{"cli_fail_ui_inconclusive", map[Layer]Verdict{LayerCLI: FAIL, LayerUI: INCONCLUSIVE}, FAIL},
		{"cli_only_pass", map[Layer]Verdict{LayerCLI: PASS}, PASS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CombineLayers(tc.layers); got != tc.want {
				t.Errorf("CombineLayers(%v) = %s, want %s", tc.layers, got, tc.want)
			}
		})
	}
}

func sampleEvidence() Evidence {
	nb := NoiseBaseline{
		Runs:  2,
		Noisy: []string{},
		ReferenceRuns: []map[string]contract.Status{
			{"F-01": contract.PASS, "F-02": contract.PASS},
			{"F-01": contract.PASS, "F-02": contract.PASS},
		},
	}
	layers := map[Layer]Verdict{
		LayerCLI: PASS,
		LayerUI:  PASS,
	}
	rec := Evidence{
		ConformVersion: conform.Version,
		ContractSHA256: SHA256Bytes([]byte("# sample contract\n")),
		Candidate:      Identity{Name: "candidate-form", Digest: "sha256:aaaa1111"},
		Reference:      Identity{Name: "reference-form", Digest: "sha256:bbbb2222"},
		Environment: EnvSummary{
			OS:        "linux",
			Arch:      "amd64",
			GoVersion: "go1.27.4",
			Shell:     "sh",
		},
		Baseline: &nb,
		Layers:   layers,
		Verdict:  CombineLayers(layers),
		Results: []EvidenceResult{
			{Layer: LayerCLI, Name: "F-01", Status: contract.PASS, Detail: "ok", Freshness: Fresh},
			{Layer: LayerCLI, Name: "F-02", Status: contract.PASS, Detail: "ok", Freshness: Reused},
			{Layer: LayerUI, Name: "ui-inventory", Status: contract.PASS, Detail: "ok", Freshness: Fresh},
		},
		Timestamp: "2026-11-25T21:30:00Z",
	}
	rec.ApplySeal()
	return rec
}

func TestSealAndVerifyRoundTrip(t *testing.T) {
	rec := sampleEvidence()
	if len(rec.Seal) != 64 {
		t.Fatalf("Seal length = %d (%q), want 64 hex chars", len(rec.Seal), rec.Seal)
	}
	if !Verify(rec) {
		t.Fatalf("Verify(rec) = false, want true")
	}

	// Pretty-printed JSON must round-trip cleanly and verify via both Verify and VerifyJSON.
	pretty, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatalf("marshal pretty: %v", err)
	}
	var decoded Evidence
	if err := json.Unmarshal(pretty, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !Verify(decoded) {
		t.Fatalf("Verify(decoded) = false, want true")
	}
	ok, err := VerifyJSON(pretty)
	if err != nil || !ok {
		t.Fatalf("VerifyJSON(pretty) = (%v, %v), want (true, nil)", ok, err)
	}

	// Compact JSON and reordered keys must verify identically because seal is over canonical JSON.
	compact, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal compact: %v", err)
	}
	ok, err = VerifyJSON(compact)
	if err != nil || !ok {
		t.Fatalf("VerifyJSON(compact) = (%v, %v), want (true, nil)", ok, err)
	}

	// CanonicalJSON must not contain the seal field itself and must be deterministic.
	canon1 := string(CanonicalJSON(rec))
	canon2 := string(CanonicalJSON(decoded))
	if canon1 != canon2 {
		t.Fatalf("CanonicalJSON not deterministic across round-trip")
	}
	if strings.Contains(canon1, `"seal"`) {
		t.Errorf("CanonicalJSON must omit seal field, got: %s", canon1)
	}
}

func TestSealTamperDetection(t *testing.T) {
	base := sampleEvidence()

	t.Run("unsealed_record_rejected", func(t *testing.T) {
		r := base
		r.Seal = ""
		if Verify(r) {
			t.Fatal("Verify accepted unsealed record")
		}
	})

	t.Run("corrupted_seal_rejected", func(t *testing.T) {
		r := base
		if r.Seal[0] == 'a' {
			r.Seal = "b" + r.Seal[1:]
		} else {
			r.Seal = "a" + r.Seal[1:]
		}
		if Verify(r) {
			t.Fatal("Verify accepted corrupted seal")
		}
	})

	mutations := []struct {
		name   string
		mutate func(r *Evidence)
	}{
		{"conform_version", func(r *Evidence) { r.ConformVersion = "9.9.9" }},
		{"contract_sha256", func(r *Evidence) { r.ContractSHA256 = SHA256Bytes([]byte("other")) }},
		{"candidate_digest", func(r *Evidence) { r.Candidate.Digest = "sha256:tampered" }},
		{"candidate_name", func(r *Evidence) { r.Candidate.Name = "other-cand" }},
		{"reference_digest", func(r *Evidence) { r.Reference.Digest = "sha256:tampered" }},
		{"env_os", func(r *Evidence) { r.Environment.OS = "darwin" }},
		{"env_shell", func(r *Evidence) { r.Environment.Shell = "bash" }},
		{"layer_verdict", func(r *Evidence) {
			cp := make(map[Layer]Verdict, len(r.Layers))
			for k, v := range r.Layers {
				cp[k] = v
			}
			cp[LayerUI] = FAIL
			r.Layers = cp
		}},
		{"overall_verdict", func(r *Evidence) { r.Verdict = FAIL }},
		{"result_status", func(r *Evidence) {
			cp := append([]EvidenceResult(nil), r.Results...)
			cp[0].Status = contract.FAIL
			r.Results = cp
		}},
		{"result_freshness_reused_to_fresh", func(r *Evidence) {
			cp := append([]EvidenceResult(nil), r.Results...)
			cp[1].Freshness = Fresh
			r.Results = cp
		}},
		{"result_detail", func(r *Evidence) {
			cp := append([]EvidenceResult(nil), r.Results...)
			cp[0].Detail = "edited"
			r.Results = cp
		}},
		{"result_removed", func(r *Evidence) {
			r.Results = r.Results[:len(r.Results)-1]
		}},
		{"baseline_noisy", func(r *Evidence) {
			nb := *r.Baseline
			nb.Noisy = []string{"F-01"}
			r.Baseline = &nb
		}},
		{"timestamp", func(r *Evidence) { r.Timestamp = "2026-11-25T21:30:01Z" }},
	}

	for _, tc := range mutations {
		t.Run("tamper_"+tc.name, func(t *testing.T) {
			r := base
			tc.mutate(&r)
			if Verify(r) {
				t.Fatalf("Verify failed to detect tampering of %s", tc.name)
			}
		})
	}

	t.Run("tamper_injected_unknown_json_field", func(t *testing.T) {
		raw, err := json.Marshal(base)
		if err != nil {
			t.Fatal(err)
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatal(err)
		}
		obj["injected"] = "evil"
		tampered, err := json.Marshal(obj)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := VerifyJSON(tampered)
		if err != nil {
			t.Fatalf("VerifyJSON error: %v", err)
		}
		if ok {
			t.Fatal("VerifyJSON failed to detect injected unknown field")
		}
	})
}

func TestSealedEvidenceFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "sealed_evidence.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	ok, err := VerifyJSON(data)
	if err != nil || !ok {
		t.Fatalf("VerifyJSON(sealed_evidence.json) = (%v, %v), want (true, nil)", ok, err)
	}
	var rec Evidence
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if !Verify(rec) {
		t.Fatalf("Verify(sealed_evidence.json) = false, want true")
	}
	if rec.Verdict != PASS {
		t.Errorf("Verdict = %s, want PASS", rec.Verdict)
	}
	if rec.Results[0].Freshness != Fresh || rec.Results[1].Freshness != Reused {
		t.Errorf("unexpected freshness values: %+v", rec.Results)
	}

	// Mutating raw JSON bytes in memory must invalidate VerifyJSON.
	tampered := strings.Replace(string(data), `"fresh"`, `"reused"`, 1)
	ok, err = VerifyJSON([]byte(tampered))
	if err != nil {
		t.Fatalf("VerifyJSON(tampered) err: %v", err)
	}
	if ok {
		t.Fatal("VerifyJSON failed to detect freshness tampering in fixture")
	}
}

func TestCurrentEnvSanitizesShellPath(t *testing.T) {
	env := CurrentEnv("/usr/local/bin/bash -c")
	if env.Shell != "bash" {
		t.Errorf("Shell = %q, want %q", env.Shell, "bash")
	}
	if env.OS == "" || env.Arch == "" {
		t.Errorf("OS/Arch should be populated: %+v", env)
	}
}
