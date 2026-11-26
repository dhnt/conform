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
`features.yaml`, `examples/`) without running any commands. It exits 0 when
clean, 1 when findings are reported, and 2 on usage or unreadable input.

- `feature-no-acceptance`: a feature in `features.yaml` whose status is not `tbd` has no acceptance item.
- `example-unreferenced`: a file under `examples/` never appears in an assertion command of `CONTRACT.md`.
- `flag-undefined`: a `--flag` used across three or more distinct verbs in assertion commands is never mentioned in the `CONTRACT.md` rules text.
- `closed-undeclared`: the `CONTRACT.md` rules text never contains the word `closed` while at least one JSON object expected output lacks `"...":"..."`.
- `envelope-key-missing`: a constant top-level string key present in at least 90% of JSON object expected outputs is missing from another.

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
