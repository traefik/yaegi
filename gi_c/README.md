# gi_c

Proof of concept for a go interpreter, using previous private works

# Install

dependencies: graphviz for AST / CFG visualization

```
make
make test
```

# Notes

Language scope is very limited, just enough to run the demo script `loop2.go`. Interesting enough to get an idea of performances compared with binaries, with or without JIT compilation.

the machine code generator is using GNU-lightning (installed with `make deps`).

Do not expect many improvements here, the real work will take place in Go, and will try to reuse as much as possible existing Go assets.

--
Marc
