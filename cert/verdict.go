package cert

import "github.com/dhnt/conform/contract"

// Verdict is the three-valued certification outcome: PASS, FAIL, or INCONCLUSIVE.
type Verdict = contract.Status

const (
	PASS         Verdict = contract.PASS
	FAIL         Verdict = contract.FAIL
	INCONCLUSIVE Verdict = contract.INCONCLUSIVE
)

// Layer identifies a behavioural layer evaluated during certification.
type Layer string

const (
	LayerCLI Layer = "cli"
	LayerUI  Layer = "ui"
)

// CombineVerdicts computes the three-valued AND across verdicts:
//   - empty input -> INCONCLUSIVE
//   - any FAIL -> FAIL
//   - any INCONCLUSIVE (or unrecognized status) -> INCONCLUSIVE
//   - otherwise -> PASS
func CombineVerdicts(verdicts ...Verdict) Verdict {
	if len(verdicts) == 0 {
		return INCONCLUSIVE
	}
	hasInconclusive := false
	for _, v := range verdicts {
		switch v {
		case FAIL:
			return FAIL
		case PASS:
			// continue
		default:
			// INCONCLUSIVE or any unrecognized/empty status fails closed.
			hasInconclusive = true
		}
	}
	if hasInconclusive {
		return INCONCLUSIVE
	}
	return PASS
}

// VerdictFromResults computes the three-valued AND across a slice of assertion results.
func VerdictFromResults(results []contract.Result) Verdict {
	if len(results) == 0 {
		return INCONCLUSIVE
	}
	verdicts := make([]Verdict, len(results))
	for i, r := range results {
		verdicts[i] = r.Status
	}
	return CombineVerdicts(verdicts...)
}

// CombineLayers combines per-layer verdicts (such as cli and ui) using
// three-valued AND. An empty map produces INCONCLUSIVE.
func CombineLayers(layers map[Layer]Verdict) Verdict {
	if len(layers) == 0 {
		return INCONCLUSIVE
	}
	verdicts := make([]Verdict, 0, len(layers))
	for _, v := range layers {
		verdicts = append(verdicts, v)
	}
	return CombineVerdicts(verdicts...)
}
