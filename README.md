# Go interpreter

[![Build Status](https://semaphoreci.com/api/v1/projects/9ae7b3d7-85f8-492f-aa43-62039d5342b7/2465344/badge.svg)](https://semaphoreci.com/containous/dyngo)

A Go interpreter in go

## Tests

Tests are simple standalone go programs to be run by `gi` executable.

Scripts are converted to go test examples for execution by `go test` as well.
To create a new test, simply add a new .gi file, specifying expected output at end of program in a `// Output:` comment block like in the following example:

```go
package main

func main() {
	println("Hello")
}

// Output:
// Hello
```

Then in `_test/`, run `make` to re-generate `interp/eval_test.go`

When developing/debugging, I'm running `gi` on a single script, using `-a` and `-c` options to display AST and CFG graphs, and instrumenting code with temporary println statements to diagnose problems.
