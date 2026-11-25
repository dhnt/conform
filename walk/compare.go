package walk

import "fmt"

// CompareWithDefinition applies the UI gate in definition order.
func CompareWithDefinition(got InventoryResult, definition []Field) (bool, string) {
	if !got.HasSaveButton {
		return false, "UI gate: missing Save button on form page"
	}
	if !got.HasSubmitButton {
		return false, "UI gate: missing Submit button on form page"
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
		if f.ID != "" && f.ID != id {
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
		if kind == "choice" || kind == "multi" || kind == "yes-no" {
			options := want.Options
			if kind == "yes-no" {
				options = []string{"yes", "no"}
			}
			if !equalOptions(f.Options, options) {
				return false, fmt.Sprintf("UI gate: field %q options=%q, expected %q", id, f.Options, options)
			}
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
