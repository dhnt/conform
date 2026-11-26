---
id: 20ee398e412f
kind: bug
title: conform --fresh-home leaks daemons started under the temporary HOME
seq: 764
status: todo
priority: p2
labels:
    - conform
created: 2026-11-26T00:03:20.469259Z
---

Observed 2026-11-25 (Sprint 722 A7 on the gate lane): conform mutate --fresh-home against the form reference left 146 'form serve' daemons running ~1h, one per fresh HOME (the app legitimately starts a background server that outlives each step). Run() removes the temp HOME but never stops processes started under it; --kill-strays only covers the step's own process group and the daemon detaches.
Root cause: product gap in contract.Run isolation (fresh-home cleanup is directory-only).
Proposed fix: after a fresh-home Run, terminate processes whose cwd/exe/args or environment reference the temp HOME (Linux: /proc/*/environ HOME=...; macOS: best effort via ps args), or start each Run in its own session and kill that session; test with a fake CLI that daemonizes.
Workaround used in Sprint 722 grading: pkill by the workspace/HOME path after each run.
