# Conformance results

Two sets of results:

- `results.tsv`, `summary.txt`: the yaegi conformance programs (below);
- `stdlib.tsv`, `stdlib-summary.txt`: the external tests of standard
  library packages, natively, in VM mode and in AOT mode, written by
  `go run ./cmd/stdtests -out docs/conformance/stdlib.tsv -summary docs/conformance/stdlib-summary.txt`
  (see [../STDTESTS.md](../STDTESTS.md)). `stdlib-causes.tsv` gives the
  class and the issue of each yaegi v2 failure.

## yaegi conformance programs

`results.tsv` and `summary.txt` are written by
`go run ./cmd/runtests -out docs/conformance/results.tsv -summary docs/conformance/summary.txt`.
`YAEGI_CONFORMANCE=1 go test ./cmd/runtests` compares a new run with `results.tsv`.

## Known differences with the expected output

- `type5.go` and `type6.go` (master, `Output:`) print `main.T`, like `go run`.
  Their expected output is `int`: this is a yaegi limit (yaegi's reflect
  cannot create named types), and yaegi itself skips these two programs in
  its consistency test with `go run`. yaegi v2 counts them as failures because
  the runner compares with the `Output:` comment. They passed before the
  program's own reflect calls used the VM reflect (19 October 2026): master
  `Output:` went from 860 to 858 / 868 for this reason only.
