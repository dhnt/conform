package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Bindings maps placeholder names ("ID", "PORT", "TIME", "KEY") to their bound
// values across sequential assertions. A nil Bindings map is treated as empty
// for reads and does not retain new bindings.
type Bindings map[string]string

// Clone returns a shallow copy of b. Always returns a non-nil map.
func (b Bindings) Clone() Bindings {
	cp := make(Bindings, len(b))
	for k, v := range b {
		cp[k] = v
	}
	return cp
}

func (b Bindings) merge(other Bindings) {
	if b == nil {
		return
	}
	for k, v := range other {
		b[k] = v
	}
}

var (
	placeholderTokenRe = regexp.MustCompile(`\{(ID|PORT|TIME|KEY)\}`)
	templateTokenRe    = regexp.MustCompile(`(\.\.\.|\{(?:ID|PORT|TIME|KEY)\})`)

	placeholderPatterns = map[string]string{
		"ID":   `[A-Za-z0-9][A-Za-z0-9_-]{0,63}`,
		"PORT": `[0-9]{2,5}`,
		"TIME": `[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.+-]+Z?`,
		"KEY":  `[A-Za-z0-9_-]{8,128}`,
	}
	placeholderPrefixRe map[string]*regexp.Regexp
	placeholderFullRe   map[string]*regexp.Regexp

	openObjTailRe  = regexp.MustCompile(`,\s*\.\.\.\s*\}`)
	openObjEmptyRe = regexp.MustCompile(`\{\s*\.\.\.\s*\}`)
	openArrTailRe  = regexp.MustCompile(`,\s*\.\.\.\s*\]`)
)

func init() {
	placeholderPrefixRe = make(map[string]*regexp.Regexp, len(placeholderPatterns))
	placeholderFullRe = make(map[string]*regexp.Regexp, len(placeholderPatterns))
	for k, pat := range placeholderPatterns {
		placeholderPrefixRe[k] = regexp.MustCompile("^" + pat)
		placeholderFullRe[k] = regexp.MustCompile("^" + pat + "$")
	}
}

// Substitute replaces bound {ID}, {PORT}, {TIME}, and {KEY} placeholders in cmd.
// If any referenced placeholder is not bound in b, Substitute returns the
// partially substituted string and an error listing the missing names.
func Substitute(cmd string, b Bindings) (string, error) {
	missingSet := make(map[string]struct{})
	rendered := placeholderTokenRe.ReplaceAllStringFunc(cmd, func(tok string) string {
		key := tok[1 : len(tok)-1]
		val, ok := b[key]
		if !ok {
			missingSet[key] = struct{}{}
			return tok
		}
		return val
	})
	if len(missingSet) > 0 {
		missing := make([]string, 0, len(missingSet))
		for k := range missingSet {
			missing = append(missing, k)
		}
		sort.Strings(missing)
		return rendered, fmt.Errorf("unbound placeholder(s) in command: %s", strings.Join(missing, ", "))
	}
	return rendered, nil
}

type tokenKind int

const (
	tokLiteral tokenKind = iota
	tokWildcard
	tokPlaceholder
)

type templateToken struct {
	kind tokenKind
	val  string
}

func tokenizeTemplate(tmpl string) []templateToken {
	locs := templateTokenRe.FindAllStringIndex(tmpl, -1)
	var toks []templateToken
	addWildcard := func() {
		if len(toks) > 0 && toks[len(toks)-1].kind == tokWildcard {
			return
		}
		toks = append(toks, templateToken{kind: tokWildcard})
	}
	pos := 0
	for _, loc := range locs {
		if loc[0] > pos {
			toks = append(toks, templateToken{kind: tokLiteral, val: tmpl[pos:loc[0]]})
		}
		part := tmpl[loc[0]:loc[1]]
		if part == "..." {
			addWildcard()
		} else {
			toks = append(toks, templateToken{kind: tokPlaceholder, val: part[1 : len(part)-1]})
		}
		pos = loc[1]
	}
	if pos < len(tmpl) {
		toks = append(toks, templateToken{kind: tokLiteral, val: tmpl[pos:]})
	}
	return toks
}

func matchTemplate(template, actual string, b Bindings) (bool, string) {
	if template == "..." {
		return true, ""
	}
	toks := tokenizeTemplate(template)
	trial := b.Clone()
	if !matchTokens(toks, 0, actual, 0, trial) {
		return false, fmt.Sprintf("expected %q, got %q", template, actual)
	}
	b.merge(trial)
	return true, ""
}

func matchTokens(toks []templateToken, ti int, actual string, pos int, bound Bindings) bool {
	if ti == len(toks) {
		return pos == len(actual)
	}
	tok := toks[ti]
	switch tok.kind {
	case tokLiteral:
		if !strings.HasPrefix(actual[pos:], tok.val) {
			return false
		}
		return matchTokens(toks, ti+1, actual, pos+len(tok.val), bound)

	case tokWildcard:
		if ti == len(toks)-1 {
			return true
		}
		for end := len(actual); end >= pos; end-- {
			if end < len(actual) && !utf8.RuneStart(actual[end]) {
				continue
			}
			if matchTokens(toks, ti+1, actual, end, bound) {
				return true
			}
		}
		return false

	case tokPlaceholder:
		if existing, ok := bound[tok.val]; ok {
			if !strings.HasPrefix(actual[pos:], existing) {
				return false
			}
			return matchTokens(toks, ti+1, actual, pos+len(existing), bound)
		}
		loc := placeholderPrefixRe[tok.val].FindStringIndex(actual[pos:])
		if loc == nil {
			return false
		}
		fullRe := placeholderFullRe[tok.val]
		for length := loc[1]; length >= 1; length-- {
			cand := actual[pos : pos+length]
			if !fullRe.MatchString(cand) {
				continue
			}
			bound[tok.val] = cand
			if matchTokens(toks, ti+1, actual, pos+length, bound) {
				return true
			}
			delete(bound, tok.val)
		}
		return false
	}
	return false
}

func decodeJSONStrict(raw string) (any, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing JSON token")
		}
		return nil, err
	}
	return v, nil
}

func parseExpectedJSON(raw string) (any, error) {
	norm := openObjTailRe.ReplaceAllString(raw, `, "__open__": "..."}`)
	norm = openObjEmptyRe.ReplaceAllString(norm, `{"__open__": "..."}`)
	norm = openArrTailRe.ReplaceAllString(norm, `, "..."]`)
	return decodeJSONStrict(norm)
}

func keyPriority(k string) int {
	switch k {
	case "id":
		return 0
	case "ownerKey":
		return 1
	case "close_at":
		return 2
	case "url":
		return 3
	default:
		return 5
	}
}

func jsonTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case json.Number, float64, int, int64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func numbersEqual(a, b json.Number) bool {
	if a == b {
		return true
	}
	if ai, aerr := a.Int64(); aerr == nil {
		if bi, berr := b.Int64(); berr == nil {
			return ai == bi
		}
	}
	af, aerr := strconv.ParseFloat(string(a), 64)
	bf, berr := strconv.ParseFloat(string(b), 64)
	if aerr != nil || berr != nil {
		return false
	}
	return af == bf
}

func matchJSON(expected, actual any, b Bindings, path string) (bool, string) {
	switch exp := expected.(type) {
	case string:
		if exp == "..." {
			return true, ""
		}
		actStr, ok := actual.(string)
		if !ok {
			return false, fmt.Sprintf("%s: expected string %q, got %s (%v)", path, exp, jsonTypeName(actual), actual)
		}
		ok, why := matchTemplate(exp, actStr, b)
		if !ok {
			return false, fmt.Sprintf("%s: %s", path, why)
		}
		return true, ""

	case nil:
		if actual != nil {
			return false, fmt.Sprintf("%s: expected null, got %v", path, actual)
		}
		return true, ""

	case bool:
		actBool, ok := actual.(bool)
		if !ok || actBool != exp {
			return false, fmt.Sprintf("%s: expected %v, got %v", path, exp, actual)
		}
		return true, ""

	case json.Number:
		actNum, ok := actual.(json.Number)
		if !ok || !numbersEqual(exp, actNum) {
			return false, fmt.Sprintf("%s: expected %s, got %v", path, exp, actual)
		}
		return true, ""

	case []any:
		actSlice, ok := actual.([]any)
		if !ok {
			return false, fmt.Sprintf("%s: expected array, got %s", path, jsonTypeName(actual))
		}
		openTail := false
		if len(exp) > 0 {
			if lastStr, isStr := exp[len(exp)-1].(string); isStr && lastStr == "..." {
				openTail = true
			}
		}
		reqItems := exp
		if openTail {
			reqItems = exp[:len(exp)-1]
			if len(actSlice) < len(reqItems) {
				return false, fmt.Sprintf("%s: expected at least %d items, got %d", path, len(reqItems), len(actSlice))
			}
		} else if len(actSlice) != len(reqItems) {
			return false, fmt.Sprintf("%s: expected %d items, got %d", path, len(reqItems), len(actSlice))
		}
		for idx, expItem := range reqItems {
			ok, why := matchJSON(expItem, actSlice[idx], b, fmt.Sprintf("%s[%d]", path, idx))
			if !ok {
				return false, why
			}
		}
		return true, ""

	case map[string]any:
		actMap, ok := actual.(map[string]any)
		if !ok {
			return false, fmt.Sprintf("%s: expected object, got %s", path, jsonTypeName(actual))
		}
		_, hasDots := exp["..."]
		_, hasOpen := exp["__open__"]
		isOpen := hasDots || hasOpen

		expKeys := make([]string, 0, len(exp))
		expKeySet := make(map[string]struct{}, len(exp))
		for k := range exp {
			if k == "..." || k == "__open__" {
				continue
			}
			expKeys = append(expKeys, k)
			expKeySet[k] = struct{}{}
		}
		sort.Slice(expKeys, func(i, j int) bool {
			pi, pj := keyPriority(expKeys[i]), keyPriority(expKeys[j])
			if pi != pj {
				return pi < pj
			}
			return expKeys[i] < expKeys[j]
		})

		for _, k := range expKeys {
			actVal, exists := actMap[k]
			if !exists {
				return false, fmt.Sprintf("%s: missing required field %q", path, k)
			}
			ok, why := matchJSON(exp[k], actVal, b, path+"."+k)
			if !ok {
				return false, why
			}
		}
		if !isOpen {
			var extra []string
			for k := range actMap {
				if _, ok := expKeySet[k]; !ok {
					extra = append(extra, k)
				}
			}
			if len(extra) > 0 {
				sort.Strings(extra)
				return false, fmt.Sprintf("%s: unexpected extra field(s) %v", path, extra)
			}
		}
		return true, ""

	default:
		return false, fmt.Sprintf("%s: unsupported expected type %T", path, expected)
	}
}
