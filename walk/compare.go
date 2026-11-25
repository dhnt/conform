package walk

import (
	"fmt"
	"strings"
)

// CompareWithDefinition applies the UI gate in definition order.
func CompareWithDefinition(got InventoryResult, definition []Field, wantButtons []string) (bool, string) {
	for _, want := range wantButtons {
		found := false
		for _, button := range got.Buttons {
			if strings.EqualFold(button, clean(want)) {
				found = true
				break
			}
		}
		if !found {
			return false, fmt.Sprintf("UI gate: missing button %q on form page", want)
		}
	}
	if len(got.Fields) != len(definition) {
		return false, fmt.Sprintf("UI gate: rendered %d fields, definition has %d", len(got.Fields), len(definition))
	}
	for i, want := range definition {
		f := got.Fields[i]
		id := want.ID
		if id == "" {
			id = fmt.Sprintf("#%d", i)
		}
		if want.ID != "" && f.ID != id {
			return false, fmt.Sprintf("UI gate: field[%d] id=%q, expected %q", i, f.ID, id)
		}
		if f.Label != want.Label {
			return false, fmt.Sprintf("UI gate: field %q label=%q, expected %q", id, f.Label, want.Label)
		}
		kind := want.Kind
		if kind == "" {
			kind = "text"
		}
		if f.Kind != kind {
			return false, fmt.Sprintf("UI gate: field %q kind=%q, expected %q", id, f.Kind, kind)
		}
		if f.Required != want.Required {
			return false, fmt.Sprintf("UI gate: field %q required=%t, expected %t", id, f.Required, want.Required)
		}
		if kind == "choice" || kind == "multi" || kind == "yes-no" && want.Options != nil {
			options := want.Options
			if !equalOptions(f.Options, options) {
				return false, fmt.Sprintf("UI gate: field %q options=%q, expected %q", id, f.Options, options)
			}
		}
		if kind == "yes-no" && want.Options == nil && len(f.Options) > 0 && !yesNoOptions(f.Options) {
			// A yes-no field offering options must offer exactly yes and no; a
			// single checkbox offers none.
			return false, fmt.Sprintf("UI gate: field %q options=%q, expected yes and no", id, f.Options)
		}
	}
	return true, "ok"
}

func equalOptions(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func yesNoOptions(options []string) bool {
	if len(options) != 2 {
		return false
	}
	a, b := strings.ToLower(clean(options[0])), strings.ToLower(clean(options[1]))
	return a == "yes" && b == "no" || a == "no" && b == "yes"
}
