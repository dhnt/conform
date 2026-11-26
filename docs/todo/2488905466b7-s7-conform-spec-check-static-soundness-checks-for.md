---
id: 2488905466b7
kind: feature
title: 'S7 conform spec check: static soundness checks for a shapeware spec folder'
seq: 765
status: todo
priority: p0
labels:
    - conform
created: 2026-11-26T09:21:19.79687Z
sprint: 723
sprint_id: a7f3f8dd-c83a-5191-9739-676397a50c45
sprint_title: 'Spec to source calibration: sound spec, validator, minimal model, judge band'
---

Implements umbrella Sprint 723 story d379a8098c56 (S7, static part only). Add `conform spec check DIR [--json]`.

DIR is a spec folder: `CONTRACT.md`, `features.yaml`, `examples/`. The command reads files only; it runs nothing.

## Code
- New package `spec` (`spec/check.go`): `Check(dir string) ([]Finding, error)`; `Finding{Rule, Subject, Message string}`. Reuse `contract.Parse` for the assertions. Stdlib only: no YAML library; read `features.yaml` with a small line scanner for the shape in the fixtures (top-level `- id:`, `status:`, `acceptance:` followed by `    - ` items).
- `cli/cli.go`: route `spec check`; update the usage text. Output: one line per finding, `RULE SUBJECT: message`, sorted by rule then subject; `--json` prints the array (`[]` when clean). Exit 0 clean, 1 findings, 2 usage or unreadable input.
- README: a short "Spec check" section listing the rules.

## Rules (exactly these five)
"Rules text" below means every line of CONTRACT.md that is outside an assertion section (an assertion section runs from its `### NAME` heading to the next heading of any level).
1. `feature-no-acceptance`: a feature in features.yaml whose status is not `tbd` has no acceptance item. Subject: the feature id.
2. `example-unreferenced`: a file under `examples/` whose relative path (for example `examples/contact.yaml`) appears in no assertion command of CONTRACT.md. Subject: the path.
3. `flag-undefined`: a `--flag` used in assertion commands with three or more distinct verbs (the verb is the second word of the command, as in `form add`) that the rules text never mentions. Subject: the flag.
4. `closed-undeclared`: the rules text never contains the word `closed` (case-insensitive) while at least one JSON object expected output lacks the open marker `"...":"..."`. One finding; subject `CONTRACT.md`; the message lists the assertion names.
5. `envelope-key-missing`: a top-level key that carries one identical string value (for example `"schema_version":"bashy-form-v1"`) in at least 90 % of the JSON object expected outputs, and is absent from another. Keys whose value varies (ids, statuses) never count. Subject: the key; the message lists the assertions missing it.

## Fixtures (already committed with this story)
`spec/testdata/form-spec-v1` and `spec/testdata/form-spec-v2` are the bashy form spec before and after its v2 revision. Do not edit them.

## Acceptance
- On `form-spec-v1` the findings are exactly: `example-unreferenced examples/contact.yaml`, `flag-undefined --fields`, `closed-undeclared CONTRACT.md`.
- On `form-spec-v2` there are no findings, exit 0.
- Unit tests in `spec/` cover both fixtures plus one small synthetic case per rule (rules 1 and 5 included); a CLI test covers the exit codes and `--json`.
- `GOWORK=off go vet ./... && GOWORK=off go test -count=1 ./...` passes; `gofmt -l .` prints nothing; `deps_test.go` still passes with no new module.

## Constraints
- Touch only `spec/`, `cli/`, `README.md`. No change to `contract`, `walk`, `cert`, the version or go.mod.
- Follow CLAUDE.md in this repo. Public repo: no host names, logins or home paths.
- Commit on your work branch with trailers `Sprint: #723`, `Story:` and `Story-ID:` for this story, and `Agent:` with your registered name. Do not push to main; the conductor reviews, gates on the test lane and merges.
- Record the verify command and its result before you submit.
