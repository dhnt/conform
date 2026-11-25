// Package conform is the invariant behavioural judge for shaped apps.
//
// An app counts as built or shaped only when it passes a fixed suite under a
// declared scope, with sealed evidence. conform is that suite: the same code
// runs for the builder, the shaper and the grader.
//
// Packages:
//
//   - contract: CONTRACT.md parser, output normalization and runner; results
//     are reported by assertion name as PASS, FAIL or INCONCLUSIVE.
//   - walk: one DOM-inventory algorithm, as a Go HTML walk and an embedded JS
//     asset for in-page probes, kept equal by a parity test.
//   - cert: reference-vs-reference noise baseline, mutation harness, per-layer
//     verdict combined with AND, and the sealed evidence record.
//
// Dependency direction is one way: conform imports no app, no bashy package
// and no yoke command package. bashy imports conform and ships it.
package conform

// Version is the conform suite version recorded in every evidence record.
const Version = "0.1.0"
