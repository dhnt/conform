package cert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhnt/conform/contract"
	"github.com/dhnt/conform/walk"
)

func TestDefaultMutantsKillFakeCLIAndUI(t *testing.T) {
	f, err := os.Open("../contract/testdata/form-v1/CONTRACT.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	assertions, err := contract.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile("testdata/form.html")
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := walk.Inventory(strings.NewReader(string(page)))
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), "state")
	bin, err := filepath.Abs("../contract/testdata/fakecli/bin")
	if err != nil {
		t.Fatal(err)
	}
	env := Environment{
		CLI:  contract.Env{PathPrefix: bin, Extra: []string{"FAKE_STATE_DIR=" + state}},
		HTML: string(page),
		Definition: []walk.Field{
			{ID: "name", Label: "Full name", Kind: "text", Required: true},
			{ID: "contact", Label: "Preferred contact", Kind: "choice", Required: true, Options: []string{"email", "phone"}},
		},
		Buttons: []string{"Submit answers"},
		Reset:   func() error { return os.RemoveAll(state) },
	}
	mutants := DefaultMutants(assertions, inventory)
	if len(mutants) < len(assertions)*2+5 {
		t.Fatalf("too few seeded mutants: %d", len(mutants))
	}
	report, err := Mutate(context.Background(), assertions, env, mutants)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range report.Mutants {
		if !result.Killed {
			t.Errorf("survived: %s (%s)", result.Name, result.Layer)
		}
	}
	if report.Killed != report.Total || report.Strength != 1 {
		t.Fatalf("strength = %v (%d/%d)", report.Strength, report.Killed, report.Total)
	}
}

func TestMutateRejectsFailingBaselineAndReportsSurvivor(t *testing.T) {
	a := []contract.Assertion{{Name: "X-01", Commands: []string{"form list"}, Exit: 0}}
	bin, err := filepath.Abs("../contract/testdata/fakecli/bin")
	if err != nil {
		t.Fatal(err)
	}
	env := Environment{CLI: contract.Env{PathPrefix: bin}}
	survivor := Mutant{Name: "noop", Layer: CLI, Apply: func(*Environment) (func(), error) { return nil, nil }}
	report, err := Mutate(context.Background(), a, env, []Mutant{survivor})
	if err != nil || report.Killed != 0 || report.Total != 1 || report.Strength != 0 {
		t.Fatalf("survivor report = %+v, %v", report, err)
	}
	a[0].Exit = 7
	if _, err := Mutate(context.Background(), a, env, []Mutant{survivor}); err == nil {
		t.Fatal("expected failing baseline error")
	}
}

func TestUIMutantsOnDeskPage(t *testing.T) {
	page, err := os.ReadFile("../walk/testdata/desk_form.html")
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := walk.Inventory(strings.NewReader(string(page)))
	if err != nil {
		t.Fatal(err)
	}
	env := Environment{HTML: string(page), Definition: inventory.Fields, Buttons: inventory.Buttons}
	report, err := Mutate(context.Background(), nil, env, DefaultMutants(nil, inventory))
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range report.Mutants {
		if !result.Killed {
			t.Errorf("survived: %s", result.Name)
		}
	}
}
