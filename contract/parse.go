package contract

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Assertion is one numbered behavioural assertion parsed from a CONTRACT.md file.
type Assertion struct {
	Name     string
	Prose    string
	Kind     string
	URL      string
	Commands []string
	Expected string
	Exit     int
}

// IsJSON reports whether the assertion expects structured JSON output: the last
// command includes "--json" and the expected output begins with '{' or '['.
func (a Assertion) IsJSON() bool {
	if len(a.Commands) == 0 {
		return false
	}
	last := a.Commands[len(a.Commands)-1]
	exp := strings.TrimSpace(a.Expected)
	return strings.Contains(last, "--json") && (strings.HasPrefix(exp, "{") || strings.HasPrefix(exp, "["))
}

var (
	headingRe = regexp.MustCompile(`^#{2,4}\s+([A-Za-z]+-\d+[A-Za-z0-9_-]*)\s*$`)
	exitRe    = regexp.MustCompile(`^exit:\s*(-?\d+)\s*$`)
)

// Parse reads a CONTRACT.md document and returns its ordered assertions.
func Parse(r io.Reader) ([]Assertion, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")

	var assertions []Assertion
	i := 0
	for i < len(lines) {
		m := headingRe.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if m == nil {
			i++
			continue
		}
		name := m[1]
		i++

		var proseLines []string
		blockKind := ""
		for i < len(lines) {
			s := strings.TrimSpace(lines[i])
			if strings.HasPrefix(s, "```console") {
				blockKind = "cli"
				break
			}
			if s == "```ui" {
				blockKind = "ui"
				break
			}
			if headingRe.MatchString(s) {
				break
			}
			if s != "" && !strings.HasPrefix(s, "<!--") {
				proseLines = append(proseLines, s)
			}
			i++
		}
		if blockKind == "" {
			return nil, fmt.Errorf("%s: missing ```console block", name)
		}
		i++ // skip opening fence

		var consoleLines []string
		closed := false
		for i < len(lines) {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				closed = true
				break
			}
			consoleLines = append(consoleLines, lines[i])
			i++
		}
		if !closed {
			if blockKind == "cli" {
				return nil, fmt.Errorf("%s: unterminated ```console block", name)
			}
			return nil, fmt.Errorf("%s: unterminated ```ui block", name)
		}
		i++ // skip closing ```
		if blockKind == "ui" {
			if len(consoleLines) < 2 || !strings.HasPrefix(strings.TrimSpace(consoleLines[0]), "GET ") {
				return nil, fmt.Errorf("%s: ui block must start with GET URL and a JSON definition", name)
			}
			url := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(consoleLines[0]), "GET "))
			if url == "" {
				return nil, fmt.Errorf("%s: empty UI URL", name)
			}
			assertions = append(assertions, Assertion{Name: name, Prose: strings.Join(proseLines, " "), Kind: "ui", URL: url, Expected: strings.TrimSpace(strings.Join(consoleLines[1:], "\n"))})
			continue
		}

		hasExit := false
		expectedExit := 0
		for i < len(lines) {
			s := strings.TrimSpace(lines[i])
			if headingRe.MatchString(s) {
				break
			}
			if em := exitRe.FindStringSubmatch(s); em != nil {
				n, err := strconv.Atoi(em[1])
				if err != nil {
					return nil, fmt.Errorf("%s: invalid exit code %q: %w", name, em[1], err)
				}
				expectedExit = n
				hasExit = true
				i++
				break
			}
			i++
		}
		if !hasExit {
			return nil, fmt.Errorf("%s: missing 'exit: N' line", name)
		}

		var commands []string
		j := 0
		for j < len(consoleLines) {
			trimmed := strings.TrimLeftFunc(consoleLines[j], unicode.IsSpace)
			if !strings.HasPrefix(trimmed, "$ ") {
				break
			}
			cmd := strings.TrimSpace(trimmed[2:])
			if cmd == "" {
				return nil, fmt.Errorf("%s: empty command in console block", name)
			}
			commands = append(commands, cmd)
			j++
		}
		if len(commands) == 0 {
			return nil, fmt.Errorf("%s: console block must begin with '$ <command>'", name)
		}
		expOut := strings.TrimSpace(strings.Join(consoleLines[j:], "\n"))

		assertions = append(assertions, Assertion{
			Name:     name,
			Prose:    strings.Join(proseLines, " "),
			Kind:     "cli",
			Commands: commands,
			Expected: expOut,
			Exit:     expectedExit,
		})
	}
	return assertions, nil
}
