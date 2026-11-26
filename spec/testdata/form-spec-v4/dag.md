---
name: form
type: dag
default: run
---

# Form lifecycle

## Tasks

### source
Read the form specification supplied in this folder.
Effects: read
Band: none

```sh
cat CONTRACT.md
```

### build
Check the specification before using a form implementation.
Effects: read, exec
Band: L3

```sh
conform spec check .
```

### test
Run the form contract against the installed implementation.
Effects: read, write, exec, net
Band: unmeasured

```sh
conform run CONTRACT.md --fresh-home
```

### install
Check that the form implementation is available on the command path.
Effects: read
Band: none

```sh
command -v form
```

### run
Pass the requested arguments to the installed form command.
Effects: read, write, exec, net
Band: none

```sh
printf '%s\n' "$ARGS" | xargs form
```

### serve
Start the form server.
Effects: read, write, exec, net
Band: unmeasured

```sh
form serve
```
