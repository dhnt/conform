package conform_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestDependencyDirection enforces the one-way rule: conform imports no app,
// no bashy package and no yoke command package (nothing under
// github.com/qiangli/), and keeps third-party dependencies explicit.
func TestDependencyDirection(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "./...").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	allowed := []string{"github.com/dhnt/conform", "golang.org/x/net/html", "github.com/dop251/goja", "github.com/dlclark/regexp2", "github.com/go-sourcemap/sourcemap", "github.com/google/pprof", "golang.org/x/text"}
	for _, p := range strings.Fields(string(out)) {
		ok := false
		for _, a := range allowed {
			if p == a || strings.HasPrefix(p, a+"/") {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("forbidden or undeclared dependency %s (extend the allow-list only with a reason)", p)
		}
	}
}
