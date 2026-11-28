---
id: 1fe89580517c
kind: enhancement
title: 'conform mutate: stop each mutant at the first killing assertion and run mutants in parallel'
seq: 768
status: todo
priority: p2
labels:
    - mutate
    - performance
created: 2026-11-28T16:49:52.970406Z
---

Problem: cert/mutate.go Mutate evaluates EVERY assertion of the contract for EVERY mutant (contract.Run over all assertions), one mutant after another. With one output mutant and one exit mutant per assertion the cost is about 2N full contract runs: Sprint 744 agent-identity 46 mutants took 1649 s; Sprint 745 hitl, 39 assertions (7 cited go tests), 78 mutants at about 38 s each, about 50 min. A slow harness is a defect (e2e must finish in minutes).
Behaviour wanted, same verdicts: (1) a mutant is killed as soon as a baseline-passing assertion fails, so stop evaluating that mutant there; assertions are order-dependent (later ones read earlier state), so run assertions in order up to and including the first failing one; an unkilled mutant still runs the whole contract. Mutants targeting assertion k are killed at k, so the cost of a targeted mutant falls from N assertions to about k. (2) Run independent mutants concurrently with a bounded worker count (flag, default GOMAXPROCS/2), each in its own scratch directory and fresh HOME; Environment.Reset and the shared --dir must become per-worker. (3) Report the same MutationReport (killed/total/strength, per-mutant name and layer), plus wall time.
Verification: unit tests with a fixture contract showing identical reports for serial vs parallel and for early-stop vs full evaluation, and that a mutant surviving to the end still evaluates every assertion; rerun the hitl and agent-identity mutate targets and record the wall time. Filed from Sprint 745 (operator asked for this todo).
