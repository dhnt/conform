package cert

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/dhnt/conform/contract"
)

// Freshness records whether a result was freshly executed for this
// certification or reused from a prior run.
type Freshness string

const (
	Fresh  Freshness = "fresh"
	Reused Freshness = "reused"
)

// Identity identifies a candidate or reference under test.
// Digest is caller-supplied (for example "sha256:...").
type Identity struct {
	Name   string `json:"name,omitempty"`
	Digest string `json:"digest"`
}

// EnvSummary captures non-identifying runtime environment metadata suitable
// for public evidence records (no hostnames, logins, addresses, or home paths).
type EnvSummary struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version,omitempty"`
	Shell     string `json:"shell,omitempty"`
}

// CurrentEnv returns an EnvSummary populated from the current Go runtime and
// the sanitized basename of shell (if non-empty).
func CurrentEnv(shell string) EnvSummary {
	env := EnvSummary{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
	}
	if fields := strings.Fields(shell); len(fields) > 0 {
		env.Shell = filepath.Base(fields[0])
	}
	return env
}

// EvidenceResult is one assertion outcome recorded in an Evidence record.
type EvidenceResult struct {
	Layer     Layer           `json:"layer"`
	Name      string          `json:"name"`
	Status    contract.Status `json:"status"`
	Detail    string          `json:"detail"`
	Freshness Freshness       `json:"freshness"`
}

// ResultsForLayer converts contract results for a layer into EvidenceResults
// tagged with the given freshness (Fresh or Reused).
func ResultsForLayer(layer Layer, results []contract.Result, freshness Freshness) []EvidenceResult {
	out := make([]EvidenceResult, len(results))
	for i, r := range results {
		out[i] = EvidenceResult{
			Layer:     layer,
			Name:      r.Name,
			Status:    r.Status,
			Detail:    r.Detail,
			Freshness: freshness,
		}
	}
	return out
}

// ResultsByAssertion tags each result with the corresponding assertion's layer.
func ResultsByAssertion(assertions []contract.Assertion, results []contract.Result, freshness Freshness) []EvidenceResult {
	out := make([]EvidenceResult, 0, len(results))
	for _, r := range results {
		layer := assertionLayer(assertions, r.Name)
		out = append(out, EvidenceResult{Layer: layer, Name: r.Name, Status: r.Status, Detail: r.Detail, Freshness: freshness})
	}
	return out
}

// LayerVerdicts derives each present layer's verdict from assertion kinds.
func LayerVerdicts(assertions []contract.Assertion, results []contract.Result) map[Layer]Verdict {
	grouped := map[Layer][]Verdict{}
	for _, r := range results {
		layer := assertionLayer(assertions, r.Name)
		grouped[layer] = append(grouped[layer], r.Status)
	}
	layers := map[Layer]Verdict{}
	for layer, verdicts := range grouped {
		layers[layer] = CombineVerdicts(verdicts...)
	}
	return layers
}

func assertionLayer(assertions []contract.Assertion, name string) Layer {
	for _, a := range assertions {
		if a.Name == name && a.Kind == "ui" {
			return LayerUI
		}
	}
	return LayerCLI
}

// Evidence is a sealed certification record.
type Evidence struct {
	ConformVersion string            `json:"conform_version"`
	ContractSHA256 string            `json:"contract_sha256"`
	Candidate      Identity          `json:"candidate"`
	Reference      Identity          `json:"reference"`
	Environment    EnvSummary        `json:"environment"`
	Baseline       *NoiseBaseline    `json:"baseline,omitempty"`
	Layers         map[Layer]Verdict `json:"layers"`
	Verdict        Verdict           `json:"verdict"`
	Results        []EvidenceResult  `json:"results"`
	Timestamp      string            `json:"timestamp"`
	Seal           string            `json:"seal,omitempty"`
}

// ApplySeal computes the canonical JSON SHA-256 seal and stores it on r.Seal.
func (r *Evidence) ApplySeal() {
	r.Seal = Seal(*r)
}

// SHA256Bytes returns the lowercase hex SHA-256 digest of data.
func SHA256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// SHA256File returns the lowercase hex SHA-256 digest of the file at path.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CanonicalJSON serializes record to deterministic canonical JSON with the
// Seal field omitted, lexicographically sorted object keys at every nesting
// level, no HTML escaping, and no insignificant whitespace.
func CanonicalJSON(record Evidence) []byte {
	cp := record
	cp.Seal = ""

	raw, err := json.Marshal(cp)
	if err != nil {
		// Evidence contains only strings, ints, maps, and slices; marshal cannot fail.
		panic(fmt.Sprintf("cert: marshal evidence: %v", err))
	}
	canon, err := canonicalizeJSONBytes(raw)
	if err != nil {
		panic(fmt.Sprintf("cert: canonicalize evidence: %v", err))
	}
	return canon
}

// Seal computes the SHA-256 hex digest over the canonical JSON representation
// of record (excluding the Seal field itself).
func Seal(record Evidence) string {
	return SHA256Bytes(CanonicalJSON(record))
}

// Verify reports whether record carries a valid Seal matching its canonical JSON.
func Verify(record Evidence) bool {
	if record.Seal == "" {
		return false
	}
	want := Seal(record)
	return subtle.ConstantTimeCompare([]byte(record.Seal), []byte(want)) == 1
}

// VerifyJSON verifies a raw JSON-encoded Evidence document directly against its
// "seal" field. Unlike Verify on a decoded struct, VerifyJSON also detects
// injected unknown JSON fields while remaining invariant to whitespace and key
// ordering.
func VerifyJSON(data []byte) (bool, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return false, err
	}
	if obj == nil {
		return false, errors.New("evidence JSON must be an object")
	}
	// Reject trailing data after the top-level JSON object.
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return false, errors.New("unexpected trailing JSON data")
	}

	sealRaw, ok := obj["seal"]
	if !ok {
		return false, nil
	}
	sealStr, ok := sealRaw.(string)
	if !ok || sealStr == "" {
		return false, nil
	}
	delete(obj, "seal")

	var buf bytes.Buffer
	if err := writeCanonicalValue(&buf, obj); err != nil {
		return false, err
	}
	want := SHA256Bytes(buf.Bytes())
	return subtle.ConstantTimeCompare([]byte(sealStr), []byte(want)) == 1, nil
}

func canonicalizeJSONBytes(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := writeCanonicalValue(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonicalValue(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if x {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case json.Number:
		buf.WriteString(string(x))
	case string:
		if err := writeCanonicalString(buf, x); err != nil {
			return err
		}
	case []any:
		buf.WriteByte('[')
		for i, elem := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalValue(buf, elem); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalString(buf, k); err != nil {
				return err
			}
			buf.WriteByte(':')
			if err := writeCanonicalValue(buf, x[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("unsupported JSON value type %T", v)
	}
	return nil
}

func writeCanonicalString(buf *bytes.Buffer, s string) error {
	var tmp bytes.Buffer
	enc := json.NewEncoder(&tmp)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	b := tmp.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	buf.Write(b)
	return nil
}
