---
id: cbbb535b5818
kind: feature
title: 'T1a conform spec check: lifecycle rules (dag.md has the five steps, each with effects and a band)'
seq: 767
status: todo
priority: p0
labels:
    - conform
created: 2026-11-26T13:07:44.632319Z
sprint: 731
sprint_id: 88c254ad-f4a4-5c54-a0fd-ac469894c684
sprint_title: 'The full shaping trip on form: spec to source to artifact to install to run and serve, with the band needed at each stage'
---

Part of umbrella Sprint 731 story a9e27a3da490 (operator ruling 2026-11-26: the lifecycle is part of the spec). Format: umbrella docs/shapeware-spec-v0.md section 5a. Example of a real lifecycle file: bashy examples/desk/form/dag.md (frontmatter, `## Tasks`, one `### <step>` per target, a one-line purpose, an `Effects:` line, then a fenced code block).

## Code inspected (conform main, 2026-11-26)
- `spec/check.go`: `Check(dir)` reads CONTRACT.md, features.yaml and examples/; six rules; findings sorted by rule then subject.
- conform imports nothing from bashy (deps_test.go); the dag.md reader here is a small Markdown scan, not bashy's parser.

## Change
`spec.Check` also reads `dag.md` in the spec folder when it exists, and reports:
1. `lifecycle-step-missing`: one finding per missing required step among `source`, `build`, `test`, `install`, `run` (a step is a `### NAME` heading under `## Tasks`). Subject: the step name.
2. `lifecycle-effects-missing`: a required step with no `Effects:` line before its code block. Subject: the step name.
3. `lifecycle-band-missing`: a required step with no `Band:` line before its code block. A band line is `Band: L<1-5>` optionally followed by free text, or `Band: none` (no model needed), or `Band: unmeasured`. Subject: the step name. An unparseable value is the same finding with the value in the message.
4. `lifecycle-missing`: no `dag.md` in the folder. Subject: `dag.md`. This one is a WARNING: add a `Severity` field to `Finding` (`error` or `warning`, default `error` for every existing rule) and make `conform spec check` exit 1 only when at least one error exists; print warnings with a `warning:` prefix, and include `severity` in `--json`.
Extra steps (`serve`, `show`, others) are allowed and are checked for rules 2 and 3 only if present.
The `conform run` advisory counts errors and warnings together.

## Fixtures and acceptance
- Add `spec/testdata/form-spec-v4` = a copy of `form-spec-v3` plus a `dag.md` with the five steps and `serve`, each with `Effects:` and `Band:` lines (use `Band: L3`, `Band: none`, `Band: unmeasured` at least once each). It reports nothing and exits 0.
- `form-spec-v3` (no dag.md) now reports exactly one warning, `lifecycle-missing dag.md`, and exits 0. v1 and v2 keep their existing findings and gain the same warning.
- Unit tests: one synthetic case per rule 1 to 3, the severity and exit-code behaviour, `--json` carrying `severity`.
- `GOWORK=off go vet ./... && GOWORK=off go test -count=1 ./...` passes on a claimed gate lane (never the dev box); `gofmt -l .` empty; `deps_test.go` passes with no new module.

## Constraints
- Touch only `spec/`, `cli/`, `README.md`. No version or go.mod change.
- Public repo: no host names, logins or home paths.
- Commit on your work branch with trailers `Sprint:`, `Story:`, `Story-ID:` for this story and `Agent:` with your registered name. Record the verify command and result before you submit. Do not push to main; the conductor gates and merges.
