# CLAUDE.md — conform

Guidance for agents working in `github.com/dhnt/conform`.

## Rules

- **Dependency direction is one way.** conform imports no app, no bashy
  package and no yoke command package (nothing under `github.com/qiangli/`).
  `deps_test.go` enforces it; extending its allow-list needs a reason in the
  commit message.
- **Stdlib first.** Allowed third-party modules: `golang.org/x/net/html` (Go
  walker) and `github.com/dop251/goja` (test-only JS parity runtime).
- **Behavioural only.** Never compare artifacts (source, binaries, markup
  bytes). Results are reported by assertion name: PASS, FAIL, INCONCLUSIVE.
- **Public repo.** No machine names, logins, addresses or home paths. No
  hidden grader data: hidden cases live with the grader, outside this repo.
- **Small, tested changes.** Each package has unit tests with fixtures under
  `testdata/`. `go vet ./... && go test ./...` must pass with `GOWORK=off`.
- Build and heavy tests on the gate host lanes, not the dev box.

## Branches

`main` is the integration branch. Delivery commits carry the umbrella
provenance trailers (`Sprint:` / `Story:` / `Story-ID:`).
