package spec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dhnt/conform/contract"
)

// knownIssue is one "- KI-<n> [status] ..." entry with its indented fields
// (shapeware spec §4.2).
type knownIssue struct {
	id      string
	status  string // open when the bracket is missing (§11)
	fixedBy string
}

var (
	kiHeaderRe   = regexp.MustCompile(`^-\s+(KI-\d+)\b(?:\s+\[([^\]]*)\])?`)
	kiFieldRe    = regexp.MustCompile(`^\s+([a-z][a-z-]*):\s*(.*)$`)
	goTestNameRe = regexp.MustCompile(`^Test[A-Za-z0-9_]+$`)
)

// checkShapeware applies the SKILL.md-aware rules: known issue <-> feature
// links (§4.2, §5), the lifted-surface candidate file set (§4.3b) and pinned
// go test lines (§6). A folder without SKILL.md has no known issues and no
// metadata.
func checkShapeware(dir string, features []feature, assertions []contract.Assertion) ([]Finding, error) {
	skill, err := readOptional(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return nil, err
	}
	moved, err := readOptional(filepath.Join(dir, "references", "known-issues.md"))
	if err != nil {
		return nil, err
	}
	meta, body := splitSkill(skill)
	issues := parseKnownIssues(knownIssuesSection(body))
	for _, ki := range parseKnownIssues(moved) {
		if _, dup := issueByID(issues, ki.id); !dup {
			issues = append(issues, ki)
		}
	}

	var findings []Finding
	findings = append(findings, checkIssueLinks(features, issues)...)
	findings = append(findings, checkCandidateFiles(meta)...)
	findings = append(findings, checkGoTests(features, assertions, implGoMod(dir))...)
	return findings, nil
}

func readOptional(p string) (string, error) {
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n"), nil
}

// splitSkill returns the frontmatter metadata keys (string scalars) and the body.
func splitSkill(src string) (map[string]string, string) {
	meta := map[string]string{}
	lines := strings.Split(src, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return meta, src
	}
	inMeta := false
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "---" {
			return meta, strings.Join(lines[i+1:], "\n")
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			k, _, _ := strings.Cut(line, ":")
			inMeta = strings.TrimSpace(k) == "metadata"
			continue
		}
		if inMeta {
			if k, v, ok := strings.Cut(strings.TrimSpace(line), ":"); ok {
				meta[strings.TrimSpace(k)] = cleanYAMLScalar(v)
			}
		}
	}
	return map[string]string{}, src // unterminated frontmatter: no metadata
}

// knownIssuesSection returns the lines under "## Known issues" up to the next
// heading of level 1 or 2, ignoring fenced blocks.
func knownIssuesSection(body string) string {
	var out []string
	var fence markdownFence
	in := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence.consume(trimmed) {
			continue
		}
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") || line == "#" || line == "##" {
			in = strings.EqualFold(strings.TrimSpace(strings.TrimLeft(line, "#")), "Known issues") && strings.HasPrefix(line, "## ")
			continue
		}
		if in {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func parseKnownIssues(src string) []knownIssue {
	var issues []knownIssue
	var cur *knownIssue
	var fence markdownFence
	for _, line := range strings.Split(src, "\n") {
		if fence.consume(strings.TrimSpace(line)) {
			cur = nil
			continue
		}
		if m := kiHeaderRe.FindStringSubmatch(line); m != nil {
			status := strings.ToLower(strings.TrimSpace(m[2]))
			if status == "" {
				status = "open"
			}
			issues = append(issues, knownIssue{id: m[1], status: status})
			cur = &issues[len(issues)-1]
			continue
		}
		if cur == nil {
			continue
		}
		if m := kiFieldRe.FindStringSubmatch(line); m != nil {
			if m[1] == "fixed-by" {
				cur.fixedBy = cleanYAMLScalar(m[2])
			}
			continue
		}
		if strings.TrimSpace(line) != "" && line[0] != ' ' && line[0] != '\t' {
			cur = nil
		}
	}
	return issues
}

func issueByID(issues []knownIssue, id string) (knownIssue, bool) {
	for _, ki := range issues {
		if ki.id == id {
			return ki, true
		}
	}
	return knownIssue{}, false
}

func featureByID(features []feature, id string) (feature, bool) {
	for _, f := range features {
		if f.id == id {
			return f, true
		}
	}
	return feature{}, false
}

func isVerified(status string) bool { return strings.EqualFold(status, "verified") }

// checkIssueLinks enforces the bug -> example -> fix chain (§4.2, §5): a
// feature's resolves: names a known issue, carries an example, and once
// verified the issue is fixed; a fixed issue names its verified feature.
func checkIssueLinks(features []feature, issues []knownIssue) []Finding {
	var out []Finding
	add := func(rule, subject, msg string) {
		out = append(out, Finding{Severity: "error", Rule: rule, Subject: subject, Message: msg})
	}
	for _, f := range features {
		if f.id == "" || f.resolves == "" {
			continue
		}
		ki, ok := issueByID(issues, f.resolves)
		if !ok {
			add("resolves-unknown-ki", f.id, fmt.Sprintf("resolves: %s, which is not a known issue in SKILL.md \"## Known issues\" or references/known-issues.md", f.resolves))
		}
		if f.acceptance == 0 {
			add("resolves-no-acceptance", f.id, fmt.Sprintf("resolves: %s but has no acceptance line; the correct behaviour needs at least one example", f.resolves))
		}
		if ok && isVerified(f.status) && ki.status != "fixed" {
			add("resolves-ki-not-fixed", f.id, fmt.Sprintf("is verified and resolves %s, which is still %s; mark %s fixed with fixed-by: %s in the same change", ki.id, ki.status, ki.id, f.id))
		}
	}
	for _, ki := range issues {
		if ki.status != "fixed" {
			continue
		}
		if ki.fixedBy == "" {
			add("ki-fixed-without-fixed-by", ki.id, "status fixed needs fixed-by: <feature-id>, the verified feature that resolves it")
			continue
		}
		f, ok := featureByID(features, ki.fixedBy)
		switch {
		case !ok:
			add("ki-fixed-by-unknown", ki.id, fmt.Sprintf("fixed-by: %s names no feature in features.yaml", ki.fixedBy))
		case !isVerified(f.status):
			status := f.status
			if status == "" {
				status = "tbd"
			}
			add("ki-fixed-by-not-verified", ki.id, fmt.Sprintf("fixed-by: %s is %s; a fixed issue needs a verified feature", ki.fixedBy, status))
		case f.resolves != ki.id:
			got := f.resolves
			if got == "" {
				got = "nothing"
			}
			add("ki-fixed-by-mismatch", ki.id, fmt.Sprintf("fixed-by: %s, but that feature resolves %s, not %s", ki.fixedBy, got, ki.id))
		}
	}
	return out
}

// checkCandidateFiles validates the lifted-surface globs (§4.3b): repo-relative,
// no "..", well-formed, and only on a surface that names its implementation.
func checkCandidateFiles(meta map[string]string) []Finding {
	var out []Finding
	for _, key := range []string{"candidate-files", "build-outputs"} {
		val, ok := meta[key]
		if !ok {
			continue
		}
		subject := "metadata." + key
		globs := strings.Fields(val)
		if len(globs) == 0 {
			out = append(out, Finding{Severity: "error", Rule: "candidate-files-invalid", Subject: subject, Message: "is empty; list at least one repo-relative glob"})
		}
		for _, g := range globs {
			if msg := badGlob(g); msg != "" {
				out = append(out, Finding{Severity: "error", Rule: "candidate-files-invalid", Subject: subject, Message: fmt.Sprintf("%q %s", g, msg)})
			}
		}
		if key == "candidate-files" && meta["implementation"] == "" {
			out = append(out, Finding{Severity: "error", Rule: "candidate-files-not-lifted", Subject: subject, Message: "is for a lifted surface; metadata.implementation is not set"})
		}
	}
	return out
}

func badGlob(g string) string {
	if strings.HasPrefix(g, "/") || strings.Contains(g, `\`) {
		return "is not a repo-relative slash path"
	}
	for _, seg := range strings.Split(g, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return "has an empty, \".\" or \"..\" segment"
		}
		if seg == "**" {
			return "uses \"**\"; a glob segment never crosses \"/\""
		}
	}
	if _, err := path.Match(g, ""); err != nil {
		return "is not a valid glob"
	}
	return ""
}

// checkGoTests enforces the pinned go test form (§6): "<launcher> gotest PKG
// NAME" expects exactly "PASS NAME". mod is the implementation's go.mod, or
// nil when the folder stands alone.
func checkGoTests(features []feature, assertions []contract.Assertion, mod *goMod) []Finding {
	var out []Finding
	for _, f := range features {
		for i, line := range f.lines {
			cmd, want, ok := strings.Cut(line, " => ")
			if !ok {
				continue
			}
			if msg := goTestProblem(cmd, strings.TrimSpace(want), mod); msg != "" {
				out = append(out, Finding{Severity: "error", Rule: "gotest-form", Subject: fmt.Sprintf("%s#%d", f.id, i+1), Message: msg})
			}
		}
	}
	for _, a := range assertions {
		if a.Kind != "cli" || len(a.Commands) == 0 {
			continue
		}
		msg := goTestProblem(a.Commands[len(a.Commands)-1], strings.TrimSpace(a.Expected), mod)
		if msg == "" && isGoTest(a.Commands[len(a.Commands)-1]) && a.Exit != 0 {
			msg = fmt.Sprintf("exit %d; a pinned go test passes with exit 0", a.Exit)
		}
		if msg != "" {
			out = append(out, Finding{Severity: "error", Rule: "gotest-form", Subject: a.Name, Message: msg})
		}
	}
	return out
}

func isGoTest(cmd string) bool {
	words := strings.Fields(cmd)
	for i, w := range words {
		if i > 0 && w == "gotest" {
			return true
		}
	}
	return false
}

// goTestProblem returns "" when cmd is not a gotest line or is well formed.
func goTestProblem(cmd, want string, mod *goMod) string {
	words := strings.Fields(cmd)
	at := -1
	for i, w := range words {
		if i > 0 && w == "gotest" {
			at = i
			break
		}
	}
	if at < 0 {
		return ""
	}
	args := words[at+1:]
	if len(args) != 2 {
		return fmt.Sprintf("gotest takes exactly PKG NAME, got %d arguments", len(args))
	}
	pkg, name := args[0], args[1]
	if msg := badGoPackage(pkg, mod); msg != "" {
		return fmt.Sprintf("package %q %s", pkg, msg)
	}
	if !goTestNameRe.MatchString(name) {
		return fmt.Sprintf("%q is not a top-level Go test name (Test...)", name)
	}
	if want != "PASS "+name {
		return fmt.Sprintf("expects %q; a pinned go test expects exactly \"PASS %s\"", want, name)
	}
	return ""
}

// badGoPackage accepts one package, in one of three forms (§6):
//   - "." (the root package) or "./" followed by a clean relative path: no
//     empty, "." or ".." segment and no "..." pattern. The path may lie in a
//     nested module of the implementation (a subdirectory with its own
//     go.mod); the launcher runs it from that module's root. Inside its
//     implementation the path is also resolved through symlinks and must
//     stay inside; standing alone the check is lexical.
//   - a dependency import path, such as github.com/qiangli/yoke/pkg/chat,
//     whose module the implementation's go.mod requires. When the folder
//     stands alone (mod is nil) go.mod cannot be read, so a syntactically
//     valid import path is accepted.
func badGoPackage(pkg string, mod *goMod) string {
	if pkg == "." {
		return ""
	}
	if rest, ok := strings.CutPrefix(pkg, "./"); ok {
		if rest == "" {
			return "must be \".\", a ./ path inside the implementation or a dependency import path"
		}
		if msg := badPathSegments(rest, "a clean ./ path"); msg != "" {
			return msg
		}
		return escapesImpl(rest, mod)
	}
	if strings.HasPrefix(pkg, "/") || strings.HasPrefix(pkg, "-") || strings.Contains(pkg, `\`) {
		return "must be \".\", a ./ path inside the implementation or a dependency import path"
	}
	if msg := badPathSegments(pkg, "an import path"); msg != "" {
		return msg
	}
	first, _, _ := strings.Cut(pkg, "/")
	if strings.HasPrefix(first, ".") || !strings.Contains(first, ".") {
		return "must be \".\", a ./ path inside the implementation or a dependency import path (its first element has a dot, e.g. github.com/...)"
	}
	if mod == nil {
		return ""
	}
	if under(pkg, mod.path) {
		return fmt.Sprintf("is in the implementation's own module %s; name it as a ./ path", mod.path)
	}
	for _, r := range mod.requires {
		if under(pkg, r) {
			return ""
		}
	}
	return "is not in a module required by the implementation's go.mod; a dependency test must run at the pinned version"
}

// badPathSegments rejects an empty, "." or ".." segment, a "..." pattern
// and characters outside an import path's.
func badPathSegments(p, form string) string {
	for _, seg := range strings.Split(p, "/") {
		switch {
		case seg == "" || seg == "." || seg == "..":
			return "must be " + form + " with no empty, \".\" or \"..\" segment"
		case strings.Contains(seg, "..."):
			return "names a package pattern; a pin names exactly one package"
		case !importSegRe.MatchString(seg):
			return fmt.Sprintf("has segment %q; use only letters, digits and -._~+", seg)
		}
	}
	return ""
}

var importSegRe = regexp.MustCompile(`^[A-Za-z0-9._~+-]+$`)

// escapesImpl resolves a clean ./path through symlinks, as the launcher
// does, when the folder sits inside its implementation, and rejects one that
// leaves it. A path that does not exist is left to the launcher (FAIL NAME).
// Standing alone (mod is nil) the check is lexical only.
func escapesImpl(rest string, mod *goMod) string {
	if mod == nil || mod.root == "" {
		return ""
	}
	root, err := filepath.EvalSymlinks(mod.root)
	if err != nil {
		return ""
	}
	got, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(rest)))
	if err != nil {
		return ""
	}
	if rel, err := filepath.Rel(root, got); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "resolves outside the implementation through a symlink"
	}
	return ""
}

func under(pkg, module string) bool {
	return module != "" && (pkg == module || strings.HasPrefix(pkg, module+"/"))
}

// goMod is what the gotest lint reads from the implementation's go.mod.
type goMod struct {
	root     string   // the implementation root, the directory holding go.mod
	path     string   // the module directive
	requires []string // every required module path
}

// implGoMod returns the go.mod at the root of the repository the folder
// sits in (the nearest ancestor holding .git), the implementation the
// launcher runs. It returns nil when the folder stands alone: no repository
// above it, or none with a go.mod at its root.
func implGoMod(dir string) *goMod {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	for d := abs; ; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			b, err := os.ReadFile(filepath.Join(d, "go.mod"))
			if err != nil {
				return nil
			}
			m := parseGoMod(string(b))
			m.root = d
			return m
		}
		parent := filepath.Dir(d)
		if parent == d {
			return nil
		}
		d = parent
	}
}

// parseGoMod reads the module directive and the require directives, single
// or in a block; comments are dropped and quoted paths unquoted.
func parseGoMod(src string) *goMod {
	m := &goMod{}
	inRequire := false
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if inRequire {
			if f[0] == ")" {
				inRequire = false
			} else {
				m.requires = append(m.requires, strings.Trim(f[0], `"`+"`"))
			}
			continue
		}
		switch {
		case f[0] == "module" && len(f) >= 2:
			m.path = strings.Trim(f[1], `"`+"`")
		case f[0] == "require" && len(f) >= 2 && f[1] == "(":
			inRequire = true
		case f[0] == "require" && len(f) >= 3:
			m.requires = append(m.requires, strings.Trim(f[1], `"`+"`"))
		}
	}
	return m
}
