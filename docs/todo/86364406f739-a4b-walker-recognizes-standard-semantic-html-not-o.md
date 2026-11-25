---
id: 86364406f739
kind: feature
title: A4b walker recognizes standard semantic HTML, not only desk-kit markup
seq: 762
status: done
priority: p0
labels:
    - conform
created: 2026-11-25T21:30:03.329434Z
weave: 4
assignee: codex-gpt6-sol
sprint: 722
sprint_id: ffc7a05d-4229-598a-8834-4c1212246274
sprint_title: 'dhnt/conform, then: does the spec reproduce the app?'
closed: 2026-11-25T21:37:44.196104Z
closed_by: claude-opus5.5
---

Problem: walk (A4, 1f4bd85) finds fields only inside div[data-field] and hard-codes a Save button (InventoryResult.HasSaveButton). A Phase B candidate writes its own page from the spec, so the UI layer must judge plain, accessible HTML behaviourally, independent of markup conventions.

Change (Go and walker.js, parity kept):
- A field is any visible form control group: input (not hidden/submit/button), select, textarea; radio/checkbox inputs group by name (or by enclosing fieldset/[role=radiogroup|group]).
- Field id: data-field if present, else name, else id.
- Label, in order: label[for=id], wrapping label, aria-labelledby, aria-label, fieldset legend, the enclosing data-field's first label; trailing '*' and whitespace trimmed; aria-hidden text ignored.
- Kind: textarea -> textarea; select -> choice (multiple -> multi); radio group -> choice, or yes-no when the options are exactly yes/no (case-insensitive); checkbox group -> multi; single checkbox -> yes-no; type=file -> file; other inputs -> text; data-kind overrides when present.
- Required: required attribute, aria-required=true on the control or group.
- Options: option/radio/checkbox visible labels (value as fallback); placeholder options with an empty value are skipped.
- Buttons: replace HasSaveButton/HasSubmitButton with Buttons []string (visible labels, normalized). CompareWithDefinition(got, definition, wantButtons []string) checks fields and that each wanted button label is present (case-insensitive).
- Keep data-field/data-kind/desk-* as hints only.

Tests: the existing desk_form + edges fixtures still produce the same fields; add testdata/plain.html (the same six form fields written as plain semantic HTML with label for, fieldset/legend radio and checkbox groups, select, file) whose inventory equals desk_form's; parity green on all fixtures.

Source context: conform/CLAUDE.md rules; walk/ package on main. Commit trailers Sprint: #722 / Story: #<seq> / Story-ID. Record your verify command (GOWORK=off go vet ./... && go test ./...) and its result. Keep it under 30 minutes.
