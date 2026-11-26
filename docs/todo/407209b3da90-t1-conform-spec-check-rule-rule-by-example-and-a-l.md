---
id: 407209b3da90
kind: feature
title: 'T1 conform: spec check rule rule-by-example, and a lint advisory in conform run'
seq: 766
status: todo
priority: p0
labels:
    - conform
created: 2026-11-26T12:20:42.489048Z
sprint: 729
sprint_id: 23ff5447-840d-5644-a78d-e4b4bd799d92
sprint_title: 'Shaping tooling: the spec linter catches rules given by example, and the judge advises builders to lint'
---

Operator direction 2026-11-26: bashy's tooling should help agents succeed at shaping, for example by advising them to run the spec linter. Evidence: in umbrella Sprint 723 a build accepted `stripe-api-key` because the form contract gives its secret-name rule by example only ("secret-named fields (`api_key`, `db_password`, ...)").

## Code inspected (conform main, 2026-11-26)
- `spec/check.go`: `Check(dir) ([]Finding, error)` with five rules; "rules text" = lines of CONTRACT.md outside assertion sections (`extractRulesText`).
- `cli/spec.go`: `conform spec check DIR [--json]`. `cli/cli.go`: `runContract` parses `conform run CONTRACT.md [flags]`.
- Fixtures: `spec/testdata/form-spec-v1`, `form-spec-v2`.

## Change
1. New rule `rule-by-example` in `spec.Check`: a line of the rules text that gives an open-ended list where a definition is needed. Flag a rules-text line when, outside code fences, it contains one of: a parenthesised or comma list ending in `...` or `…` (for example "(`api_key`, `db_password`, ...)"), `etc.` , `and so on`, or `e.g.`/`such as` followed by a list and no closing definition marker. Keep it simple and deterministic: implement exactly these patterns, case-insensitive: (a) `, ...)` or `, …)` ; (b) `\betc\.` ; (c) `\band so on\b`. Do NOT flag the JSON open marker (`"...": "..."`, `"key": "..."`) or `...` inside backticks that contain a double quote. Subject: `CONTRACT.md:<line number>`. Message: the matched text and "define the set (the decision rule) and give boundary examples".
2. Advisory in `conform run`: after the run, if the contract file is named CONTRACT.md and `spec.Check` on its directory succeeds with one or more findings, print ONE line to stderr: `conform: note: spec check reports N finding(s) for this spec (run "conform spec check DIR"); where a rule is underspecified, do not guess silently`. Never print it with zero findings or when `spec.Check` errors (for example no features.yaml). It must not change stdout, the `--json` output or the exit code.
3. README: add the rule to the Spec check list and one sentence on the advisory.

## Acceptance
- On `form-spec-v2`: exactly one new finding, `rule-by-example CONTRACT.md:<line>` for the secret-named line; the three v1 findings on `form-spec-v1` stay and v1 gains the same kind of finding only if its text matches (record what it reports in the test).
- Add fixture `spec/testdata/form-spec-v3` = a copy of v2 with that one line replaced by a definition (no ellipsis); it must report nothing.
- Unit tests: one synthetic case per pattern (a), (b), (c); negatives for the JSON open marker and for `"..."` in backticks. CLI test: `run` prints the advisory on a spec with findings, prints nothing extra on a clean one, and stdout/`--json`/exit code are unchanged.
- `GOWORK=off go vet ./... && GOWORK=off go test -count=1 ./...` passes on a claimed gate lane (never the dev box); `gofmt -l .` empty; `deps_test.go` passes with no new module.

## Constraints
- Touch only `spec/`, `cli/`, `README.md`. No version or go.mod change.
- Public repo: no host names, logins or home paths.
- Commit on your work branch with trailers `Sprint:`, `Story:`, `Story-ID:` for this story and `Agent:` with your registered name. Record the verify command and result before you submit. Do not push to main; the conductor gates and merges.
