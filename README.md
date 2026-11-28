# conform

The invariant behavioural judge for shaped apps.

An app counts as built or shaped only when it passes a fixed suite under a
declared scope, with sealed evidence. conform is that suite. Builders, shapers
and graders run the same code, and no app can change it.

- **Behaviour, not artifacts.** conform runs the app's commands and checks
  what they do. It never compares source. Similarity scores are diagnostics
  only.
- **CLI first.** The `CONTRACT.md` CLI contract is the golden standard. A thin
  UI gate checks the page inventory (fields, labels, kinds, options, buttons)
  and a few real interactions against the CLI.
- **Certification practice.** A reference-vs-reference noise baseline, a
  mutation harness that measures how strong the suite is, a per-layer verdict
  combined with AND, and a sealed evidence record.

## Contract format

```markdown
### F-02
Re-surface a form without exposing its definition or answers.
```console
$ form open {ID} --json --fields id,status,url
{"id":"{ID}","status":"pending","url":"http://localhost:{PORT}/?id={ID}"}
```
exit: 0
```

Placeholders (`{ID}`, `{PORT}`, `{TIME}`, `{KEY}`) bind on first match and
apply in later steps. `--json` output is compared structurally; `"..."` marks
an open, implementation-varying field. Plain text is checked only where the
contract gives it.

UI assertions use a `ui` block with a GET URL and the expected form inventory:

````markdown
### U-01
The rendered form exposes its required controls.
```ui
GET http://localhost:{PORT}/?id={ID}
{"fields":[{"id":"name","label":"Full name","kind":"text","required":true}],"buttons":["Save"]}
```
````

`conform run CONTRACT.md --fresh-home` gives each run a new HOME and TMPDIR.
`--reset-cmd CMD` runs a command before each run. `--kill-strays` stops background
processes left by a CLI step; leave it off when later assertions need a server.
`conform mutate CONTRACT.md --path BIN_DIR --fresh-home` runs the default CLI
and UI mutants and reports the killed fraction. Add `--json` for a
`cert.MutationReport`.

## Spec check

`conform spec check DIR [--json]` statically checks a spec folder (`CONTRACT.md`,
`features.yaml`, `examples/`, and `dag.md`) without running commands. It exits 0
for a clean check or warnings alone, 1 for any error finding, and 2 for usage or
unreadable input. Text output prefixes warnings with `warning:`. JSON findings
include a `severity` of `error` or `warning`.

- `feature-no-acceptance`: a feature in `features.yaml` whose status is not `tbd` has no acceptance item.
- `example-unreferenced`: a file under `examples/` never appears in an assertion command of `CONTRACT.md`.
- `flag-undefined`: a `--flag` used across three or more distinct verbs in assertion commands is never mentioned in the `CONTRACT.md` rules text.
- `closed-undeclared`: the `CONTRACT.md` rules text never contains the word `closed` while at least one JSON object expected output lacks `"...":"..."`.
- `envelope-key-missing`: a constant top-level string key present in at least 90% of JSON object expected outputs is missing from another.
- `rule-by-example` flags rules-text lines outside code fences that contain a list ending in `, ...)` or `, …)`, the abbreviation `etc.`, or the phrase `and so on`.
- `lifecycle-missing` warns when the folder has no `dag.md`.
- `lifecycle-step-missing` flags each missing target among `source`, `build`, `test`, `install`, and `run`. Declare targets as `### NAME` headings under `## Tasks`.
- `lifecycle-effects-missing` flags any target missing an `Effects:` line before its code block.
- `lifecycle-band-missing` flags any target missing a valid `Band:` line before its code block. Use `L1` through `L5` with optional explanatory text, or the exact value `none` or `unmeasured`.

When the folder has a `SKILL.md`, the check also reads its frontmatter `metadata` and the `- KI-<n> [status]` entries under `## Known issues` (and in `references/known-issues.md`, if present). Shapeware spec §4.2, §4.3b, §5 and §6 define the keys.

- `resolves-unknown-ki`: a feature's `resolves: KI-<n>` names no known issue.
- `resolves-no-acceptance`: a feature with `resolves:` has no acceptance line, whatever its status.
- `resolves-ki-not-fixed`: a `verified` feature resolves a known issue that is not `fixed`.
- `ki-fixed-without-fixed-by`: a `fixed` known issue has no `fixed-by: <feature-id>`.
- `ki-fixed-by-unknown`, `ki-fixed-by-not-verified`, `ki-fixed-by-mismatch`: `fixed-by:` names a missing feature, a feature that is not `verified`, or a feature whose `resolves:` is not this issue.
- `candidate-files-invalid`: a glob in `metadata.candidate-files` or `metadata.build-outputs` is empty, absolute, has an empty, `.` or `..` segment, uses `**`, or is malformed.
- `candidate-files-not-lifted`: `metadata.candidate-files` is set but `metadata.implementation` is not.
- `gotest-form`: an acceptance line or CLI assertion that runs `<launcher> gotest PKG NAME` must take exactly one package (`.`, or `./` followed by a clean relative path with no `..` segment and no `...` pattern) and a top-level `Test...` name, expect exactly `PASS NAME`, and (in `CONTRACT.md`) exit 0.

After running a file named `CONTRACT.md`, `conform run` prints a one-line advisory to stderr with the combined error and warning count if the spec check succeeds with findings.

## Use

```sh
go install github.com/dhnt/conform/cmd/conform@latest
conform version
```

bashy ships conform as `bashy conform`.

## Layout

| Package | Role |
|---|---|
| `contract` | `CONTRACT.md` parser, normalization, runner |
| `walk` | DOM inventory: Go HTML walk + embedded JS asset, parity-tested |
| `cert` | noise baseline, mutation harness, verdict, sealed evidence |
| `spec` | static soundness checks for a spec folder |
| `cli` | the command line (`Run`), shared by `cmd/conform` and `bashy conform` |
| `cmd/conform` | the standalone binary |

## License

MIT
