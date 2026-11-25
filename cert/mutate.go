package cert

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dhnt/conform/contract"
	"github.com/dhnt/conform/walk"
	"golang.org/x/net/html"
)

// Environment is the candidate observed by the contract and UI gates. Reset
// restores candidate state before each run; it is required for stateful CLIs.
type Environment struct {
	CLI        contract.Env
	HTML       string
	Definition []walk.Field
	Buttons    []string
	Reset      func() error
}

// Mutant.Apply changes a private copy of the candidate environment and returns
// cleanup for resources such as a PATH shim directory.
type Mutant struct {
	Name  string
	Layer Layer
	Apply func(*Environment) (func(), error)
}

type MutationResult struct {
	Name   string `json:"name"`
	Layer  Layer  `json:"layer"`
	Killed bool   `json:"killed"`
}

type MutationReport struct {
	Mutants  []MutationResult `json:"mutants"`
	Killed   int              `json:"killed"`
	Total    int              `json:"total"`
	Strength float64          `json:"strength"`
}

// Mutate kills a mutant when a previously passing CLI assertion or UI gate
// becomes non-passing. An invalid baseline is an error, not a mutation kill.
func Mutate(ctx context.Context, assertions []contract.Assertion, env Environment, mutants []Mutant) (MutationReport, error) {
	var report MutationReport
	if err := reset(env); err != nil {
		return report, err
	}
	baseline, err := evaluate(ctx, assertions, env)
	if err != nil {
		return report, fmt.Errorf("baseline: %w", err)
	}
	var failed []string
	for i, ok := range baseline {
		if !ok {
			if i < len(assertions) {
				failed = append(failed, assertions[i].Name)
			} else {
				failed = append(failed, "UI comparison")
			}
		}
	}
	if len(failed) > 0 {
		return report, fmt.Errorf("baseline not PASS: %s", strings.Join(failed, ", "))
	}
	for i, m := range mutants {
		if m.Name == "" || m.Apply == nil || m.Layer != LayerCLI && m.Layer != LayerUI {
			return report, fmt.Errorf("mutant %d: invalid name, layer, or Apply", i)
		}
		if err := reset(env); err != nil {
			return report, fmt.Errorf("mutant %s reset: %w", m.Name, err)
		}
		candidate := env
		candidate.CLI.Extra = append([]string(nil), env.CLI.Extra...)
		cleanup, err := m.Apply(&candidate)
		if err != nil {
			return report, fmt.Errorf("mutant %s apply: %w", m.Name, err)
		}
		observed, evalErr := evaluate(ctx, assertions, candidate)
		if cleanup != nil {
			cleanup()
		}
		if evalErr != nil {
			return report, fmt.Errorf("mutant %s: %w", m.Name, evalErr)
		}
		killed := false
		for j := range baseline {
			if baseline[j] && !observed[j] {
				killed = true
				break
			}
		}
		report.Mutants = append(report.Mutants, MutationResult{Name: m.Name, Layer: m.Layer, Killed: killed})
		if killed {
			report.Killed++
		}
	}
	report.Total = len(mutants)
	if report.Total > 0 {
		report.Strength = float64(report.Killed) / float64(report.Total)
	}
	return report, nil
}

func reset(env Environment) error {
	if env.Reset != nil {
		return env.Reset()
	}
	return nil
}

func evaluate(ctx context.Context, assertions []contract.Assertion, env Environment) ([]bool, error) {
	results := contract.Run(ctx, assertions, env.CLI)
	pass := make([]bool, 0, len(results)+1)
	for _, r := range results {
		pass = append(pass, r.Status == contract.PASS)
	}
	if env.HTML != "" {
		got, err := walk.Inventory(strings.NewReader(env.HTML))
		if err != nil {
			return nil, err
		}
		ok, _ := walk.CompareWithDefinition(got, env.Definition, env.Buttons)
		pass = append(pass, ok)
	}
	return pass, nil
}

var jsonKey = regexp.MustCompile(`"([A-Za-z_][A-Za-z0-9_-]*)"\s*:`)

// DefaultMutants seeds one missing-key mutant per distinct JSON assertion key,
// one wrong-exit mutant per assertion, and UI mutations from a passing inventory.
func DefaultMutants(assertions []contract.Assertion, inventory walk.InventoryResult) []Mutant {
	var out []Mutant
	for _, a := range assertions {
		if len(a.Commands) == 0 {
			continue
		}
		command, pattern := commandPattern(a.Commands[len(a.Commands)-1])
		if command == "" {
			continue
		}
		seen := make(map[string]bool)
		if a.IsJSON() {
			for _, match := range jsonKey.FindAllStringSubmatch(a.Expected, -1) {
				key := match[1]
				if key == "__open__" || seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, cliMutant(a.Name+"/key/"+key, command, pattern, "key", key))
			}
		} else if strings.TrimSpace(a.Expected) != "" {
			out = append(out, cliMutant(a.Name+"/output", command, pattern, "output", ""))
		}
		out = append(out, cliMutant(a.Name+"/exit", command, pattern, "exit", ""))
	}
	return append(out, defaultUIMutants("", inventory)...)
}

// DefaultUIMutants targets the HTML fetched by one UI assertion.
func DefaultUIMutants(assertion string, inventory walk.InventoryResult) []Mutant {
	return defaultUIMutants(assertion, inventory)
}

func defaultUIMutants(assertion string, inventory walk.InventoryResult) []Mutant {
	prefix := "UI/"
	if assertion != "" {
		prefix = assertion + "/"
	}
	var out []Mutant
	for _, f := range inventory.Fields {
		id := f.ID
		out = append(out, uiMutant(prefix+"field/"+id+"/label", assertion, id, "label", ""))
		out = append(out, uiMutant(prefix+"field/"+id+"/required", assertion, id, "required", ""))
		out = append(out, uiMutant(prefix+"field/"+id+"/hidden", assertion, id, "hidden", ""))
		for _, option := range f.Options {
			out = append(out, uiMutant(prefix+"field/"+id+"/option/"+option, assertion, id, "option", option))
		}
	}
	for _, button := range inventory.Buttons {
		out = append(out, uiMutant(prefix+"button/"+button, assertion, "", "button", button))
	}
	return out
}

func commandPattern(command string) (string, string) {
	parts := strings.Fields(command)
	if len(parts) == 0 || !regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(parts[0]) {
		return "", ""
	}
	pattern := ""
	for _, part := range parts[1:] {
		if strings.ContainsAny(part, "{}'\"$;|&") {
			break
		}
		if pattern != "" {
			pattern += " "
		}
		pattern += part
	}
	return parts[0], pattern
}

func cliMutant(name, command, pattern, mode, key string) Mutant {
	return Mutant{Name: name, Layer: LayerCLI, Apply: func(env *Environment) (func(), error) {
		path := env.CLI.PathPrefix
		if path == "" {
			path = os.Getenv("PATH")
		} else {
			path += string(os.PathListSeparator) + os.Getenv("PATH")
		}
		real, err := exec.LookPath(command)
		if path != os.Getenv("PATH") {
			for _, dir := range filepath.SplitList(path) {
				candidate := filepath.Join(dir, command)
				if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() && info.Mode()&0111 != 0 {
					real = candidate
					err = nil
					break
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("find %s: %w", command, err)
		}
		dir, err := os.MkdirTemp("", "conform-mutant-")
		if err != nil {
			return nil, err
		}
		// Arguments are data: the shim uses fixed shell source and environment
		// variables rather than interpolating candidate paths into shell code.
		shim := `#!/bin/sh
out=$(mktemp) || exit 125
trap 'rm -f "$out"' EXIT
"$CONFORM_MUTANT_REAL" "$@" >"$out"
code=$?
case " $* " in
  *"$CONFORM_MUTANT_PATTERN"*)
    case "$CONFORM_MUTANT_MODE" in
      key) sed "s/\"$CONFORM_MUTANT_KEY\"[[:space:]]*:/\"__mutant_missing_key__\":/g" "$out" ;;
      output) printf '__mutant_output__\n' ;;
      exit) cat "$out"; if [ "$code" -eq 0 ]; then code=1; else code=0; fi ;;
    esac ;;
  *) cat "$out" ;;
esac
exit "$code"
`
		if err := os.WriteFile(filepath.Join(dir, command), []byte(shim), 0700); err != nil {
			os.RemoveAll(dir)
			return nil, err
		}
		env.CLI.PathPrefix = dir + string(os.PathListSeparator) + env.CLI.PathPrefix
		env.CLI.Extra = append(env.CLI.Extra, "CONFORM_MUTANT_REAL="+real, "CONFORM_MUTANT_PATTERN="+pattern, "CONFORM_MUTANT_MODE="+mode, "CONFORM_MUTANT_KEY="+key)
		return func() { _ = os.RemoveAll(dir) }, nil
	}}
}

func uiMutant(name, assertion, id, mode, value string) Mutant {
	return Mutant{Name: name, Layer: LayerUI, Apply: func(env *Environment) (func(), error) {
		if assertion != "" {
			env.CLI.UITransform = func(gotName string, fetched []byte) ([]byte, error) {
				if gotName != assertion {
					return fetched, nil
				}
				return mutateHTML(fetched, id, mode, value)
			}
			return nil, nil
		}
		mutated, err := mutateHTML([]byte(env.HTML), id, mode, value)
		if err != nil {
			return nil, err
		}
		env.HTML = string(mutated)
		return nil, nil
	}}
}

func mutateHTML(source []byte, id, mode, value string) ([]byte, error) {
	root, err := html.Parse(bytes.NewReader(source))
	if err != nil {
		return nil, err
	}
	var target *html.Node
	visitNode(root, func(n *html.Node) {
		if target == nil && n.Type == html.ElementNode && attrNode(n, "data-field") == id && id != "" {
			target = n
		}
	})
	if target == nil && id != "" {
		visitNode(root, func(n *html.Node) {
			if target == nil && n.Type == html.ElementNode && (attrNode(n, "name") == id || attrNode(n, "id") == id) {
				target = n
			}
		})
	}
	if mode == "button" {
		visitNode(root, func(n *html.Node) {
			if target != nil || n.Type != html.ElementNode {
				return
			}
			if n.Data == "button" {
				label := strings.Join(strings.Fields(nodeText(n)), " ")
				if label == "" {
					label = strings.Join(strings.Fields(attrNode(n, "aria-label")), " ")
				}
				if label == value {
					target = n
				}
			} else if n.Data == "input" {
				typeName := strings.ToLower(attrNode(n, "type"))
				label := strings.TrimSpace(attrNode(n, "value"))
				if label == "" && typeName == "submit" {
					label = "Submit"
				}
				if label == "" && typeName == "reset" {
					label = "Reset"
				}
				if label == "" {
					label = strings.TrimSpace(attrNode(n, "aria-label"))
				}
				if (typeName == "submit" || typeName == "reset" || typeName == "button") && label == value {
					target = n
				}
			}
		})
	}
	if target == nil {
		return nil, fmt.Errorf("UI target %q not found", id+value)
	}
	switch mode {
	case "hidden", "button":
		setAttr(target, "hidden", "")
	case "label":
		var label *html.Node
		visitNode(root, func(n *html.Node) {
			if label == nil && n.Type == html.ElementNode && n.Data == "label" && attrNode(n, "for") == id {
				label = n
			}
		})
		visitNode(target, func(n *html.Node) {
			if label == nil && n.Type == html.ElementNode && n.Data == "label" && !strings.Contains(attrNode(n, "class"), "desk-option") {
				label = n
			}
		})
		if label == nil {
			label = target
		}
		for child := label.FirstChild; child != nil; {
			next := child.NextSibling
			label.RemoveChild(child)
			child = next
		}
		setAttr(label, "aria-label", "__mutant_label__")
	case "required":
		wasRequired := false
		visitNode(target, func(n *html.Node) {
			for _, a := range n.Attr {
				if a.Key == "required" || a.Key == "aria-required" && a.Val == "true" {
					wasRequired = true
				}
			}
		})
		if strings.Contains(nodeText(target), "*") {
			wasRequired = true
		}
		if wasRequired {
			visitNode(target, func(n *html.Node) {
				removeAttr(n, "required")
				removeAttr(n, "aria-required")
				if n.Type == html.TextNode {
					n.Data = strings.ReplaceAll(n.Data, "*", "")
				}
			})
		} else {
			setAttr(target, "aria-required", "true")
		}
	case "option":
		var option *html.Node
		visitNode(target, func(n *html.Node) {
			if option == nil && n.Type == html.ElementNode && (n.Data == "option" || n.Data == "label" && strings.Contains(attrNode(n, "class"), "desk-option")) && (strings.TrimSpace(nodeText(n)) == value || strings.HasPrefix(strings.TrimSpace(nodeText(n)), value+"recommended")) {
				option = n
			}
		})
		if option == nil || option.Parent == nil {
			return nil, fmt.Errorf("option %q not found", value)
		}
		option.Parent.RemoveChild(option)
	}
	var b strings.Builder
	if err := html.Render(&b, root); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func visitNode(n *html.Node, fn func(*html.Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		visitNode(c, fn)
	}
}

func attrNode(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func setAttr(n *html.Node, key, value string) {
	removeAttr(n, key)
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: value})
}

func removeAttr(n *html.Node, key string) {
	attrs := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Key != key {
			attrs = append(attrs, a)
		}
	}
	n.Attr = attrs
}

func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(nodeText(c))
	}
	return b.String()
}
