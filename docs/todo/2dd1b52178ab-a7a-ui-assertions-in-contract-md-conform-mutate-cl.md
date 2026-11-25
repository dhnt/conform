---
id: 2dd1b52178ab
kind: feature
title: A7a UI assertions in CONTRACT.md + conform mutate CLI + fresh-HOME isolation
seq: 763
status: todo
priority: p0
labels:
    - conform
created: 2026-11-25T21:45:29.900775Z
sprint: 722
sprint_id: ffc7a05d-4229-598a-8834-4c1212246274
sprint_title: 'dhnt/conform, then: does the spec reproduce the app?'
---

Needed before the form contract (A7) can be recorded and before Phase B can grade the UI layer.

1. Contract format: an assertion may use a ```ui block instead of ```console:
   ### U-01
   One line of prose.
   ```ui
   GET http://localhost:{PORT}/?id={ID}
   {"fields":[{"id":"name","label":"Full name","kind":"text","required":true}, ...],"buttons":["Save","Submit"]}
   ```
   (no exit line for ui assertions). Assertion gains Kind ("cli"|"ui") and URL. Placeholders in the URL come from bindings of earlier assertions; an unbound placeholder is FAIL. The runner does an HTTP GET (loopback hosts only: localhost/127.0.0.1/[::1], else INCONCLUSIVE; per-step timeout; non-200 is FAIL), builds walk.Inventory, and compares with walk.CompareWithDefinition(got, expected.fields, expected.buttons). Result name = heading; cert layer = ui for ui assertions, cli otherwise (update cert so per-layer verdicts come from Assertion.Kind).
2. Isolation: contract.Env gains FreshHome bool: each Run gets a new empty temp dir as HOME and TMPDIR (XDG_* unset or pointed inside it), removed afterwards; plus an optional ResetCmd run before each Run. `conform run`, `conform cert` and `conform mutate` take --fresh-home and --reset-cmd CMD. Also kill the step's process group after each step only if --kill-strays is given (default off: apps may legitimately leave a server running for later steps).
3. `conform mutate CONTRACT.md [--path P] [--dir D] [--fresh-home] [--reset-cmd CMD] [--timeout D] [--json]`: baseline run must PASS (else exit 2 with the failing names); seeds cert.DefaultMutants from the contract's CLI assertions and from the inventory fetched by its ui assertions (UI mutants apply to the fetched HTML, then judge with walk); prints one line per mutant 'KILLED|SURVIVED <layer> <name>' and 'strength K/T'; exit 1 if any survived. With --json emit cert.MutationReport.
4. Tests: a fake CLI in testdata that also serves a page (Go test httptest server whose URL the fake CLI prints) covering a passing ui assertion, a missing-field FAIL, an unbound placeholder FAIL, a non-loopback INCONCLUSIVE, fresh-home isolation (state does not leak between runs), and conform mutate reaching strength 1.0 on the fake app.

Rules: conform/CLAUDE.md. Commit trailers Sprint: #722 / Story: #<seq> / Story-ID. Do not run heavy suites; go vet + go test of conform is fine in your workspace. Keep under ~40 minutes.
