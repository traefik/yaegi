# yaegi API tests (reference only)

These are yaegi's Go unit tests, copied as a reference for the public API an
eventual VM-based replacement should honor (`interp.New`, `Options`, `Use`,
`Eval`, `EvalPath`, `CompilePath`, symbol extraction...). They import
`github.com/traefik/yaegi/...` and test yaegi itself: **they do not compile
against the VM** and are not meant to. The `_` prefix of this directory keeps
them out of `go build ./...` and `go test ./...`.

| Directory | Content |
|---|---|
| `master/` | `interp/*_test.go`, `cmd/yaegi/*_test.go`, `extract/*_test.go` of yaegi master |
| `fork-<branch>/` | from fork branches: added test files, and `*.diff` patches of modified test files against the branch's merge base |
| `upstream-pr<N>/` | same, from upstream pull requests (`extract/testdata/` added by a PR is kept next to its tests) |

`interp/interp_file_test.go` (`TestFile`) and
`interp/interp_consistent_test.go` (`TestInterpConsistencyBuild`,
`TestInterpErrorConsistency`) define how the `_test/` programs are checked; the
lists the conformance runner needs from them (consistency skip reasons, gc
error messages) are copied into `cmd/runtests/yaegilists.go`, including the
additions made by the imported branches and pull requests.

Provenance of every file: `../MANIFEST.tsv`.
