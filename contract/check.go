package contract

import (
	"fmt"
	"strings"
)

// Status is the behavioural verdict for a single assertion.
type Status string

const (
	PASS         Status = "PASS"
	FAIL         Status = "FAIL"
	INCONCLUSIVE Status = "INCONCLUSIVE"
)

// Result is the outcome of checking or running one assertion.
type Result struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
}

// Check evaluates an assertion against observed command output and exit code.
// Newly matched placeholders are committed into b only when the assertion
// passes; on failure, b is left unchanged.
func Check(a Assertion, stdout string, exit int, b Bindings) Result {
	normOut := Normalize(stdout)
	if exit != a.Exit {
		return Result{
			Name:   a.Name,
			Status: FAIL,
			Detail: fmt.Sprintf("exit %d != expected %d (out: %s)", exit, a.Exit, truncate(normOut, 140)),
		}
	}

	expected := strings.TrimSpace(a.Expected)
	if expected == "" {
		// Documented plain-text freedom when expected output is omitted.
		return Result{
			Name:   a.Name,
			Status: PASS,
			Detail: "ok (exit matched; plain text open)",
		}
	}

	trial := b.Clone()
	if a.IsJSON() {
		expObj, err := parseExpectedJSON(expected)
		if err != nil {
			return Result{
				Name:   a.Name,
				Status: FAIL,
				Detail: fmt.Sprintf("invalid contract JSON: %v", err),
			}
		}
		actObj, err := decodeJSONStrict(normOut)
		if err != nil {
			return Result{
				Name:   a.Name,
				Status: FAIL,
				Detail: fmt.Sprintf("output is not valid JSON (%v): %s", err, truncate(normOut, 140)),
			}
		}
		ok, why := matchJSON(expObj, actObj, trial, "$")
		if !ok {
			return Result{
				Name:   a.Name,
				Status: FAIL,
				Detail: why,
			}
		}
		b.merge(trial)
		return Result{
			Name:   a.Name,
			Status: PASS,
			Detail: "ok",
		}
	}

	ok, why := matchTemplate(expected, normOut, trial)
	if !ok {
		return Result{
			Name:   a.Name,
			Status: FAIL,
			Detail: why,
		}
	}
	b.merge(trial)
	return Result{
		Name:   a.Name,
		Status: PASS,
		Detail: "ok",
	}
}

func truncate(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}
