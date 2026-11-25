package cert

import (
	"fmt"
	"sort"

	"github.com/dhnt/conform/contract"
)

// NoiseBaseline records which assertions did not consistently PASS across
// reference-vs-reference runs.
type NoiseBaseline struct {
	// Runs is the number of reference runs analyzed.
	Runs int `json:"runs"`
	// Noisy lists assertion names marked noisy on the reference baseline,
	// sorted lexicographically.
	Noisy []string `json:"noisy"`
	// ReferenceRuns captures each reference run's status by assertion name.
	ReferenceRuns []map[string]contract.Status `json:"reference_runs,omitempty"`
}

// IsNoisy reports whether assertion name was marked noisy in the baseline.
func (b NoiseBaseline) IsNoisy(name string) bool {
	for _, n := range b.Noisy {
		if n == name {
			return true
		}
	}
	return false
}

// Baseline analyzes repeated reference runs and marks assertions that disagree
// reference-vs-reference or fail to consistently PASS on the reference as noisy.
//
// Ported from compute_noise_baseline (grader_lib.py): any assertion that is
// missing or non-PASS in any reference run is added to Noisy.
func Baseline(runs [][]contract.Result) NoiseBaseline {
	nb := NoiseBaseline{
		Runs:  len(runs),
		Noisy: []string{},
	}
	if len(runs) == 0 {
		return nb
	}

	allNames := make(map[string]struct{})
	noisySet := make(map[string]struct{})
	refRuns := make([]map[string]contract.Status, len(runs))

	for i, run := range runs {
		m := make(map[string]contract.Status, len(run))
		for _, r := range run {
			allNames[r.Name] = struct{}{}
			if prev, exists := m[r.Name]; exists && prev != r.Status {
				noisySet[r.Name] = struct{}{}
			}
			// Preserve non-PASS if an assertion appears multiple times in one run.
			if prev, exists := m[r.Name]; !exists || prev == contract.PASS {
				m[r.Name] = r.Status
			}
		}
		refRuns[i] = m
	}

	for name := range allNames {
		for _, m := range refRuns {
			st, ok := m[name]
			if !ok || st != contract.PASS {
				noisySet[name] = struct{}{}
				break
			}
		}
	}

	noisy := make([]string, 0, len(noisySet))
	for name := range noisySet {
		noisy = append(noisy, name)
	}
	sort.Strings(noisy)

	nb.Noisy = noisy
	nb.ReferenceRuns = refRuns
	return nb
}

// Grade classifies each candidate result against a reference NoiseBaseline and
// returns the graded results along with their three-valued AND verdict.
//
// Ported from grade_candidate_against_baseline (grader_lib.py):
//   - PASS stays PASS.
//   - FAIL on a noisy assertion is downgraded to INCONCLUSIVE.
//   - FAIL on a non-noisy assertion stays FAIL.
//   - INCONCLUSIVE stays INCONCLUSIVE (annotated if also noisy).
//   - Assertions observed in baseline.ReferenceRuns that are absent from
//     candidate are appended as INCONCLUSIVE (if noisy) or FAIL (otherwise).
func Grade(candidate []contract.Result, baseline NoiseBaseline) ([]contract.Result, Verdict) {
	graded := make([]contract.Result, 0, len(candidate))
	seen := make(map[string]struct{}, len(candidate))

	for _, r := range candidate {
		seen[r.Name] = struct{}{}
		switch r.Status {
		case contract.PASS:
			graded = append(graded, r)
		case contract.FAIL:
			if baseline.IsNoisy(r.Name) {
				graded = append(graded, contract.Result{
					Name:   r.Name,
					Status: contract.INCONCLUSIVE,
					Detail: noisyDetail(r.Detail),
				})
			} else {
				graded = append(graded, r)
			}
		default:
			// INCONCLUSIVE (or empty/unknown status) never becomes FAIL.
			detail := r.Detail
			if baseline.IsNoisy(r.Name) {
				detail = noisyDetail(r.Detail)
			}
			graded = append(graded, contract.Result{
				Name:   r.Name,
				Status: contract.INCONCLUSIVE,
				Detail: detail,
			})
		}
	}

	// Any assertion seen in reference runs but missing from candidate did not run.
	if len(baseline.ReferenceRuns) > 0 {
		missingSet := make(map[string]struct{})
		for _, m := range baseline.ReferenceRuns {
			for name := range m {
				if _, ok := seen[name]; !ok {
					missingSet[name] = struct{}{}
				}
			}
		}
		if len(missingSet) > 0 {
			missing := make([]string, 0, len(missingSet))
			for name := range missingSet {
				missing = append(missing, name)
			}
			sort.Strings(missing)
			for _, name := range missing {
				if baseline.IsNoisy(name) {
					graded = append(graded, contract.Result{
						Name:   name,
						Status: contract.INCONCLUSIVE,
						Detail: "flaky on reference-vs-reference baseline (missing)",
					})
				} else {
					graded = append(graded, contract.Result{
						Name:   name,
						Status: contract.FAIL,
						Detail: "assertion did not run",
					})
				}
			}
		}
	}

	return graded, VerdictFromResults(graded)
}

func noisyDetail(detail string) string {
	if detail == "" {
		return "flaky on reference-vs-reference baseline"
	}
	return fmt.Sprintf("flaky on reference-vs-reference baseline (%s)", detail)
}
