package spec

import (
	"fmt"
	"regexp"
	"strings"
)

type markdownFence string

// consume reports whether the line opens, closes, or lies inside a fenced block.
func (f *markdownFence) consume(line string) bool {
	if *f != "" {
		if strings.HasPrefix(line, string(*f)) && strings.Trim(line, string(*f)[:1]) == "" {
			*f = ""
		}
		return true
	}
	if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
		n := len(line) - len(strings.TrimLeft(line, line[:1]))
		*f = markdownFence(line[:n])
		return true
	}
	return false
}

var bandValue = regexp.MustCompile(`^(L[1-5]([ \t]+.*)?|none|unmeasured)$`)

type lifecycleStep struct {
	name    string
	effects bool
	band    string
	hasBand bool
}

func checkLifecycle(src string) []Finding {
	var steps []lifecycleStep
	var current *lifecycleStep
	var fence markdownFence
	inTasks := false
	lines := strings.Split(src, "\n")
	frontmatter := strings.TrimSpace(lines[0]) == "---"
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if frontmatter {
			if i > 0 && (line == "---" || line == "...") {
				frontmatter = false
			}
			continue
		}
		// Four leading spaces or a tab make an indented code line.
		if strings.HasPrefix(raw, "    ") || strings.HasPrefix(strings.TrimLeft(raw, " "), "\t") {
			continue
		}
		if fence.consume(line) {
			current = nil // Only metadata before the first fence counts.
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "#", "##":
			inTasks = len(fields) == 2 && fields[0] == "##" && fields[1] == "Tasks"
			current = nil
		case "###":
			current = nil
			if inTasks && len(fields) == 2 {
				steps = append(steps, lifecycleStep{name: fields[1]})
				current = &steps[len(steps)-1]
			}
		default:
			if current != nil {
				if strings.HasPrefix(line, "Effects:") {
					current.effects = true
				}
				if value, ok := strings.CutPrefix(line, "Band:"); ok {
					current.band = strings.TrimSpace(value)
					current.hasBand = true
				}
			}
		}
	}

	var findings []Finding
	seen := make(map[string]bool)
	for _, step := range steps {
		seen[step.name] = true
		if !step.effects {
			findings = append(findings, Finding{Rule: "lifecycle-effects-missing", Subject: step.name, Message: "missing Effects: line before code block", Severity: "error"})
		}
		if !step.hasBand || !bandValue.MatchString(step.band) {
			msg := "missing Band: line before code block"
			if step.hasBand {
				msg = fmt.Sprintf("invalid Band: value %q", step.band)
			}
			findings = append(findings, Finding{Rule: "lifecycle-band-missing", Subject: step.name, Message: msg, Severity: "error"})
		}
	}
	for _, name := range []string{"source", "build", "test", "install", "run"} {
		if !seen[name] {
			findings = append(findings, Finding{Rule: "lifecycle-step-missing", Subject: name, Message: "missing step under ## Tasks", Severity: "error"})
		}
	}
	return findings
}
