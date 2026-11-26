// Package spec runs static soundness checks over a shapeware spec folder
// (CONTRACT.md, features.yaml, examples/) without executing any commands.
package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dhnt/conform/contract"
)

// Finding is a single static soundness violation in a spec folder.
type Finding struct {
	Rule    string `json:"rule"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

var (
	anyHeadingRe       = regexp.MustCompile(`^#{1,6}(?:\s+.*)?$`)
	assertionHeadingRe = regexp.MustCompile(`^#{2,4}\s+([A-Za-z]+-\d+[A-Za-z0-9_-]*)\s*$`)
	flagSyntaxRe       = regexp.MustCompile(`^--[A-Za-z0-9][A-Za-z0-9_-]*$`)
	closedWordRe       = regexp.MustCompile(`(?i)\bclosed\b`)
	templateValueRe    = regexp.MustCompile(`(\.\.\.|\{(?:ID|PORT|TIME|KEY)\})`)

	openObjTailRe  = regexp.MustCompile(`,\s*\.\.\.\s*\}`)
	openObjEmptyRe = regexp.MustCompile(`\{\s*\.\.\.\s*\}`)
	openArrTailRe  = regexp.MustCompile(`,\s*\.\.\.\s*\]`)
)

// Check statically validates the spec folder at dir (CONTRACT.md, features.yaml,
// and examples/) and returns findings sorted by Rule then Subject.
func Check(dir string) ([]Finding, error) {
	if dir == "" {
		return nil, errors.New("empty spec directory")
	}

	contractBytes, err := os.ReadFile(filepath.Join(dir, "CONTRACT.md"))
	if err != nil {
		return nil, err
	}
	assertions, err := contract.Parse(bytes.NewReader(contractBytes))
	if err != nil {
		return nil, fmt.Errorf("parse CONTRACT.md: %w", err)
	}

	featuresBytes, err := os.ReadFile(filepath.Join(dir, "features.yaml"))
	if err != nil {
		return nil, err
	}
	features := parseFeatures(string(featuresBytes))

	examples, err := listExamples(dir)
	if err != nil {
		return nil, err
	}

	rulesText := extractRulesText(string(contractBytes))
	findings := []Finding{}

	// Rule 1: feature-no-acceptance
	for _, f := range features {
		if f.id == "" {
			continue
		}
		if !strings.EqualFold(f.status, "tbd") && f.acceptance == 0 {
			msg := "feature has no acceptance items"
			if f.status != "" {
				msg = fmt.Sprintf("status %q has no acceptance items", f.status)
			}
			findings = append(findings, Finding{
				Rule:    "feature-no-acceptance",
				Subject: f.id,
				Message: msg,
			})
		}
	}

	// Rule 2: example-unreferenced
	for _, rel := range examples {
		if !assertionsMentionPath(assertions, rel) {
			findings = append(findings, Finding{
				Rule:    "example-unreferenced",
				Subject: rel,
				Message: "not referenced in any assertion command of CONTRACT.md",
			})
		}
	}

	// Rule 3: flag-undefined
	verbsByFlag := collectFlagVerbs(assertions)
	for flag, verbSet := range verbsByFlag {
		if len(verbSet) < 3 {
			continue
		}
		if !rulesMentionFlag(rulesText, flag) {
			verbs := make([]string, 0, len(verbSet))
			for v := range verbSet {
				verbs = append(verbs, v)
			}
			sort.Strings(verbs)
			findings = append(findings, Finding{
				Rule:    "flag-undefined",
				Subject: flag,
				Message: fmt.Sprintf("used across %d verbs (%s) without mention in CONTRACT.md rules", len(verbs), strings.Join(verbs, ", ")),
			})
		}
	}

	// Parse CLI JSON object expected outputs for Rules 4 and 5.
	jsonObjs := collectJSONObjectOutputs(assertions)

	// Rule 4: closed-undeclared
	if !closedWordRe.MatchString(rulesText) {
		var closedNames []string
		for _, ja := range jsonObjs {
			if !ja.isOpen {
				closedNames = append(closedNames, ja.name)
			}
		}
		if len(closedNames) > 0 {
			findings = append(findings, Finding{
				Rule:    "closed-undeclared",
				Subject: "CONTRACT.md",
				Message: fmt.Sprintf("rules text never mentions closed objects; closed expected outputs in %s", strings.Join(closedNames, ", ")),
			})
		}
	}

	// Rule 5: envelope-key-missing
	if total := len(jsonObjs); total > 0 {
		keySet := make(map[string]struct{})
		for _, ja := range jsonObjs {
			for k := range ja.obj {
				if k == "..." || k == "__open__" {
					continue
				}
				keySet[k] = struct{}{}
			}
		}
		keys := make([]string, 0, len(keySet))
		for k := range keySet {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			present := 0
			constVal := ""
			hasConst := false
			varies := false
			var missing []string

			for _, ja := range jsonObjs {
				rawVal, ok := ja.obj[k]
				if !ok {
					missing = append(missing, ja.name)
					continue
				}
				present++
				strVal, isStr := rawVal.(string)
				if !isStr || templateValueRe.MatchString(strVal) {
					varies = true
					continue
				}
				if !hasConst {
					constVal = strVal
					hasConst = true
				} else if strVal != constVal {
					varies = true
				}
			}

			if !varies && hasConst && len(missing) > 0 && present*10 >= 9*total {
				findings = append(findings, Finding{
					Rule:    "envelope-key-missing",
					Subject: k,
					Message: fmt.Sprintf("carries %q in %d/%d JSON object outputs; missing from %s", constVal, present, total, strings.Join(missing, ", ")),
				})
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Rule != findings[j].Rule {
			return findings[i].Rule < findings[j].Rule
		}
		if findings[i].Subject != findings[j].Subject {
			return findings[i].Subject < findings[j].Subject
		}
		return findings[i].Message < findings[j].Message
	})

	return findings, nil
}

type feature struct {
	id         string
	status     string
	acceptance int
}

func parseFeatures(src string) []feature {
	text := strings.ReplaceAll(src, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")

	var features []feature
	var cur *feature
	inAcceptance := false

	flush := func() {
		if cur != nil {
			features = append(features, *cur)
			cur = nil
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		if indent == 0 && (line == "-" || strings.HasPrefix(line, "- ")) {
			flush()
			cur = &feature{}
			inAcceptance = false
			rest := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			if k, v, ok := strings.Cut(rest, ":"); ok {
				applyFeatureField(cur, strings.TrimSpace(k), cleanYAMLScalar(v), &inAcceptance)
			}
			continue
		}

		if cur == nil || indent == 0 {
			continue
		}

		if inAcceptance && (trimmed == "-" || strings.HasPrefix(trimmed, "- ")) {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if item != "" {
				cur.acceptance++
			}
			continue
		}

		if indent <= 2 && !strings.HasPrefix(trimmed, "-") {
			if k, v, ok := strings.Cut(trimmed, ":"); ok {
				inAcceptance = false
				applyFeatureField(cur, strings.TrimSpace(k), cleanYAMLScalar(v), &inAcceptance)
			}
		}
	}
	flush()
	return features
}

func applyFeatureField(f *feature, key, val string, inAcceptance *bool) {
	switch key {
	case "id":
		f.id = val
	case "status":
		f.status = val
	case "acceptance":
		if val == "" {
			*inAcceptance = true
		} else if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
			inner := strings.TrimSpace(val[1 : len(val)-1])
			if inner != "" {
				f.acceptance++
			}
		}
	}
}

func cleanYAMLScalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	if idx := strings.Index(s, " #"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	return s
}

func listExamples(dir string) ([]string, error) {
	exDir := filepath.Join(dir, "examples")
	info, err := os.Stat(exDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", exDir)
	}

	var out []string
	err = filepath.WalkDir(exDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func extractRulesText(src string) string {
	text := strings.ReplaceAll(src, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")

	var out []string
	inAssertion := false
	fenceLen := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if n := leadingBackticks(trimmed); n >= 3 {
			if fenceLen == 0 {
				fenceLen = n
			} else if n >= fenceLen && strings.Trim(trimmed, "` \t") == "" {
				fenceLen = 0
			}
		} else if fenceLen == 0 && anyHeadingRe.MatchString(trimmed) {
			inAssertion = assertionHeadingRe.MatchString(trimmed)
		}

		if !inAssertion {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func leadingBackticks(s string) int {
	n := 0
	for n < len(s) && s[n] == '`' {
		n++
	}
	return n
}

func assertionsMentionPath(assertions []contract.Assertion, rel string) bool {
	for _, a := range assertions {
		for _, cmd := range a.Commands {
			if commandMentionsPath(cmd, rel) {
				return true
			}
		}
	}
	return false
}

func commandMentionsPath(cmd, rel string) bool {
	start := 0
	for {
		idx := strings.Index(cmd[start:], rel)
		if idx < 0 {
			return false
		}
		pos := start + idx
		end := pos + len(rel)
		beforeOK := pos == 0 || !isPathChar(cmd[pos-1]) || (pos >= 2 && cmd[pos-1] == '/' && cmd[pos-2] == '.' && (pos == 2 || !isPathChar(cmd[pos-3])))
		afterOK := end == len(cmd) || !isPathChar(cmd[end])
		if beforeOK && afterOK {
			return true
		}
		start = pos + 1
	}
}

func isPathChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '-' || b == '.' || b == '/'
}

type cmdWord struct {
	val             string
	leadingUnquoted string
}

func collectFlagVerbs(assertions []contract.Assertion) map[string]map[string]struct{} {
	verbsByFlag := make(map[string]map[string]struct{})
	for _, a := range assertions {
		for _, cmd := range a.Commands {
			for _, words := range splitSimpleCommands(cmd) {
				idx := 0
				for idx < len(words) && isEnvAssign(words[idx].leadingUnquoted) && words[idx].leadingUnquoted == words[idx].val {
					idx++
				}
				rem := words[idx:]
				if len(rem) < 2 {
					continue
				}
				verb := rem[1].val
				if verb == "" || strings.HasPrefix(verb, "-") {
					continue
				}
				for _, w := range rem[2:] {
					if w.leadingUnquoted == "--" && w.val == "--" {
						break
					}
					if !strings.HasPrefix(w.leadingUnquoted, "--") {
						continue
					}
					flag, _, _ := strings.Cut(w.leadingUnquoted, "=")
					if !flagSyntaxRe.MatchString(flag) {
						continue
					}
					if verbsByFlag[flag] == nil {
						verbsByFlag[flag] = make(map[string]struct{})
					}
					verbsByFlag[flag][verb] = struct{}{}
				}
			}
		}
	}
	return verbsByFlag
}

func splitSimpleCommands(s string) [][]cmdWord {
	var cmds [][]cmdWord
	var curCmd []cmdWord
	var valBuf, leadBuf strings.Builder
	inWord := false
	seenQuote := false
	var quote byte

	flushWord := func() {
		if inWord {
			curCmd = append(curCmd, cmdWord{
				val:             valBuf.String(),
				leadingUnquoted: leadBuf.String(),
			})
			valBuf.Reset()
			leadBuf.Reset()
			inWord = false
			seenQuote = false
		}
	}
	flushCmd := func() {
		flushWord()
		if len(curCmd) > 0 {
			cmds = append(cmds, curCmd)
			curCmd = nil
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				valBuf.WriteByte(c)
			}
		case quote == '"':
			if c == '"' {
				quote = 0
			} else if c == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\' || s[i+1] == '$' || s[i+1] == '`') {
				i++
				valBuf.WriteByte(s[i])
			} else {
				valBuf.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote = c
			inWord = true
			seenQuote = true
		case c == '\\' && i+1 < len(s):
			i++
			inWord = true
			valBuf.WriteByte(s[i])
			if !seenQuote {
				leadBuf.WriteByte(s[i])
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flushWord()
		case c == ';' || c == '|' || c == '&':
			flushCmd()
		default:
			inWord = true
			valBuf.WriteByte(c)
			if !seenQuote {
				leadBuf.WriteByte(c)
			}
		}
	}
	flushCmd()
	return cmds
}

func isEnvAssign(tok string) bool {
	k, _, ok := strings.Cut(tok, "=")
	if !ok || k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		if i == 0 && !(c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
		if !(c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func rulesMentionFlag(rulesText, flag string) bool {
	start := 0
	for {
		idx := strings.Index(rulesText[start:], flag)
		if idx < 0 {
			return false
		}
		pos := start + idx
		end := pos + len(flag)
		beforeOK := pos == 0 || !isFlagIdentChar(rulesText[pos-1])
		afterOK := end == len(rulesText) || !isFlagIdentChar(rulesText[end])
		if beforeOK && afterOK {
			return true
		}
		start = pos + 1
	}
}

func isFlagIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_'
}

type jsonObjectAssertion struct {
	name   string
	obj    map[string]any
	isOpen bool
}

func collectJSONObjectOutputs(assertions []contract.Assertion) []jsonObjectAssertion {
	var out []jsonObjectAssertion
	for _, a := range assertions {
		if a.Kind != "cli" {
			continue
		}
		exp := strings.TrimSpace(a.Expected)
		if !strings.HasPrefix(exp, "{") {
			continue
		}
		obj, ok := parseExpectedJSONObject(exp)
		if !ok {
			continue
		}
		dotsVal, hasDots := obj["..."]
		_, hasOpen := obj["__open__"]
		dotsStr, dotsIsStr := dotsVal.(string)
		isOpen := (hasDots && dotsIsStr && dotsStr == "...") || hasOpen
		out = append(out, jsonObjectAssertion{
			name:   a.Name,
			obj:    obj,
			isOpen: isOpen,
		})
	}
	return out
}

func parseExpectedJSONObject(raw string) (map[string]any, bool) {
	norm := openObjTailRe.ReplaceAllString(raw, `, "__open__": "..."}`)
	norm = openObjEmptyRe.ReplaceAllString(norm, `{"__open__": "..."}`)
	norm = openArrTailRe.ReplaceAllString(norm, `, "..."]`)

	dec := json.NewDecoder(bytes.NewReader([]byte(norm)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, false
	}
	obj, ok := v.(map[string]any)
	return obj, ok
}
