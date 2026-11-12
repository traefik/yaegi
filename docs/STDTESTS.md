# Standard library tests under yaegi v2

`cmd/stdtests` runs the external tests of standard library packages (the
`_test.go` files of `package xxx_test` in GOROOT) under yaegi v2, with the
native package (or its AOT copy) as the code under test. The tests of the
Go team become an oracle for the values of a program that go through `fmt`,
`sort`, `io`, `encoding/json`... The GOROOT test suite
(`compare/gotestdir`) tests the language; these tests check the library
calls with the program's own types, which only our own programs checked
before.

Results: [conformance/stdlib.tsv](conformance/stdlib.tsv) (one line per
test) and [conformance/stdlib-summary.txt](conformance/stdlib-summary.txt)
(one line per package).

## How it works (feasibility)

**The test files.** For a package `p`, `go/build` selects the external test
files for the platform (`XTestGoFiles`, no cgo). They keep their package
name (`p_test`, so `%T` prints `p_test.T` as with `go test`) in
`<work>/p/xtest/`, renamed `*_xtest.go` (go build leaves out `*_test.go`).
A generated `main` package next to it imports them and lists their
`TestXxx`, `FuzzXxx` and `ExampleXxx` (an example runs when it has an
`Output:` comment, read with `go/doc` as `go test` does).

**Package testing.** The tests use the native `testing` package, as a plugin
would: `testing.T` is a native type, and the test functions are interpreted
closures that native `testing` calls through the native boundary. What
`testing.T` needs works without change:

- `Run`, `Parallel`, `Cleanup`, `Helper`, `TempDir`, `Setenv`, `Chdir`:
  plain native code;
- `Fatal`, `FailNow`, `Skip`, `SkipNow` call `runtime.Goexit` in native
  code, on the goroutine of the test, under the interpreted test function:
  the goroutine ends with its deferred calls, as in Go (the VM keeps the
  interpreted frames on its own stack; Goexit does not need them);
- a panic in a test fails it and ends the process, as with `go test`.

One part cannot be interpreted: `testing.MainStart` takes an unexported
interface (`testDeps`), and yaegi v2 cannot make a program type implement an
unexported interface of a native package (`reflect.Set: value of type
vm.vmOpaque is not assignable to type testing.testDeps`). So
`cmd/stdtests/testmain` is a small native package (the deps of
`testing/internal/testdeps`, without profiles, coverage or fuzzing) that the
host of the VM (`cmd/stdtests/vmhost`) registers and runs in native mode.
It calls `TestMain` when the files have one, and ends the process with the
code of `m.Run`. Fuzz targets run their `f.Add` seeds, as `go test` without
`-fuzz`; the corpus files of `testdata/fuzz` are not read (in every mode).

**export_test.go.** Files of `package p` (internal tests, and
`export_test.go`, which exports unexported names for the external tests:
`fmt.IsSpace`, `strconv.BitSizeError`...) are out of scope: they are
compiled into the package itself, and yaegi v2 calls the native package that
the host was built with. The runner type checks the external files alone
and removes each top-level declaration that does not type check, again
until they do. A test that loses its code this way is "excluded", with the
first error as the reason (`uses fmt.IsSpace (not exported:
export_test.go)`). The same holds for imports of internal packages, except
four that the runner replaces with small packages of the same API
(`cmd/stdtests/shim`): `internal/testenv` (the subset the tests use; the
functions that need to run a program skip the test, see below),
`internal/race`, `internal/asan`, `internal/msan` (`Enabled` is false).

**testdata/.** The programs run in the directory of the package in GOROOT,
as `go test` runs them, so `testdata/...` paths work. The tests only read
there (GOROOT may be read-only).

**Baseline.** The same program is built by the Go toolchain and run the
same way (`native` column): a test counts as a yaegi v2 failure only when it
passes natively. The tests run with `-test.short`, as the builders of Go
do. `-test.v` output gives the result of each top-level test. A process
that ends in a test (panic, `fatal error`, timeout, a construct that yaegi v2
does not support) gives that status to the test, and runs again for the
tests that did not run. When several tests ran in parallel, each runs alone
to find the one that ended the process.

**Level of the program.** yaegi v2 runs the programs as `yaegi` runs a
program: the standard library as in Go (not the restricted standard
library of plugins), in VM mode and in AOT mode (`-aot`).

## Results (go1.27.3, linux/amd64, November 6, 2026)

Tests that pass natively, and among them the ones that pass in VM mode and
in AOT mode ([conformance/stdlib-summary.txt](conformance/stdlib-summary.txt)).
CI runs the check on linux/amd64, so the checked-in results come from there:

| Package | Run (excluded) | Native pass | VM | AOT |
|---|---|---|---|---|
| fmt | 66 (2) | 65 | 65 | 65 |
| sort | 46 (2) | 45 | 45 | 45 |
| slices | 106 (0) | 106 | 103 | 103 |
| maps | 10 (0) | 10 | 10 | 10 |
| io | 70 (2) | 70 | 66 | 66 |
| bufio | 81 (7) | 81 | 80 | 80 |
| bytes | 163 (0) | 163 | 158 | 158 |
| strings | 124 (4) | 124 | 120 | 120 |
| strconv | 67 (4) | 66 | 65 | 65 |
| errors | 22 (0) | 22 | 22 | 22 |
| encoding/json | 28 (0) | 27 | 27 | 27 |
| encoding/xml | 5 (0) | 5 | 5 | 5 |
| flag | 21 (8) | 19 | 19 | 19 |
| context | 42 (4) | 42 | 40 | 40 |
| math/big | 15 (0) | 15 | 14 | 14 |
| text/template | 9 (0) | 8 | 8 | 8 |
| **total** | **875 (33)** | **868** | **847** | **847** |

The 7 tests that do not pass natively are skipped (`-test.short`, a test
that needs os/exec) or need what the runner leaves out.
encoding/json, encoding/xml, math/big and text/template have few external
tests: most of their tests are internal (`package json`), out of reach.

### Causes of the yaegi v2 failures

[conformance/stdlib-causes.tsv](conformance/stdlib-causes.tsv) gives the
class and the cause of each test; the runner writes them at the start of the
reason column, and "new" for a failure without a known cause.

| Class | Tests | Causes |
|---|---|---|
| wrong: silent wrong result | 2 | context (`String` of a program context); math/big (a function value of the program) |
| allocs: the test counts allocations | 17 | none: the interpreter allocates, the results are right |
| frames: the test counts `runtime.Callers` frames through native code | 2 | none: yaegi v2 does not show native frames (ARCHITECTURE.md) |

No test fails with only a different panic message.

### Bugs fixed with this work

Each has a regression program (`testdata/`, compared with `go run`):

- AOT: an `As(any) bool` method of the program set the target of
  `errors.As` in a copy that was lost, and a native pointer it stored lost
  its identity (`TestAs`, `ExampleAs_custom_match` of errors;
  `testdata/aot/errorsastarget.go`);
- a program struct that embeds a native type with methods
  (`struct{ *big.Int }`), given to native code that asserts it to an
  interface, ended the host process with a fatal error (`TestAliasing` of
  math/big; `testdata/structof_embedded_methods.go`);
- a type assertion of a native value to an interface compared the names of
  the methods, not their signatures: `err.(interface{ Unwrap() []error })`
  was true for `fmt.Errorf("%w", e)` (`TestErrorf` of fmt;
  `testdata/native_assert_signature.go`);
- a pointer of the program to a native struct (`var z big.Float; &z`) and
  the same pointer given back by native code were not equal
  (`ExampleFloat_Copy` of math/big; `testdata/native_struct_ptr_identity.go`);
- the elements of a slice of a byte type of the program with methods
  (`[]byteFormatter`) lost their methods in fmt, also in AOT mode
  (`TestSprintf` of fmt; `testdata/fmt_named_byte_elems.go`).

### Platform

The checked-in results are of darwin/arm64; the nightly CI runs on
linux/amd64 and keeps its TSV as the artifact `stdtests`. A test that
passes on one platform only can be frozen from that artifact.

## What is left out, and why

| What | Why |
|---|---|
| internal tests (`package p`), `export_test.go` | compiled into the package under test; yaegi v2 calls the host's native package |
| tests that use names of `export_test.go` | excluded with the name (`uses p.X`) |
| tests that use an internal package without a shim | excluded with the import |
| tests that need `os/exec`, the go command or the test binary | skipped by the shim of `internal/testenv` (`cmd/stdtests: needs os/exec`), also natively: a plugin cannot run programs |
| benchmarks | not run |
| fuzzing (`-fuzz`), `testdata/fuzz` corpus files | only the `f.Add` seeds run |
| `testing.Testing()` | false in every mode: the programs are not built by `go test` |
| file:line of `t.Log` in VM and AOT mode | the native `testing` sees the Go frames of the VM (`value.go:586`); it changes the log, not the result |

## Run it

```bash
go run ./cmd/stdtests                                    # all packages, native, vm, aot
go run ./cmd/stdtests -pkgs fmt,sort -modes native,vm    # some packages and modes
go run ./cmd/stdtests -pkgs errors -run 'TestAs$' -v     # one test, print each process
go run ./cmd/stdtests -out docs/conformance/stdlib.tsv -summary docs/conformance/stdlib-summary.txt
go run ./cmd/stdtests -check docs/conformance/stdlib.tsv # no regression (nightly CI)
```

The programs and binaries go to `cmd/stdtests/_work/` (ignored by git and
by `./...`). The test files are the ones of the toolchain that runs the
command (`go env GOROOT`). The checked-in results are of the Go release
`GO_VERSION` of the CI.

`-check` fails when a test that passes natively and in a yaegi v2 mode in the
checked-in file does not pass in that mode now, while it still passes
natively, also when it runs again alone. New passes do not fail; check them
in with `-out`. The nightly CI runs it (job "standard library tests").
