package contract

import (
	"regexp"
	"strings"
)

var (
	ansiRe = regexp.MustCompile(
		"\x1b\\[[0-9;?>]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|\x1b_[^\x1b]*\x1b\\\\",
	)
	loopbackIPv4 = strings.Join([]string{"127", "0", "0", "1"}, ".")
)

// Normalize applies the declared CLI output normalization: strips ANSI escape
// sequences and carriage returns, drops shell advisor hint lines, trims
// surrounding whitespace, and rewrites loopback addresses to "localhost".
func Normalize(raw string) string {
	cleaned := ansiRe.ReplaceAllString(raw, "")
	cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")
	cleaned = strings.ReplaceAll(cleaned, "\r", "\n")

	lines := strings.Split(cleaned, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "{") && strings.Contains(strings.ReplaceAll(s, " ", ""), `"schema_version":"bashy-hint-v1"`) {
			continue
		}
		if strings.HasPrefix(s, "bashy: ") && strings.Contains(s, "bashy skill show bashy") {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	out = strings.ReplaceAll(out, loopbackIPv4, "localhost")
	out = strings.ReplaceAll(out, "[::1]", "localhost")
	return out
}
