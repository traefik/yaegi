# Official Go test suite: yaegi v2 vs yaegi

This directory runs the official Go conformance suite (`test/` of the Go
repository, tag go1.27.3, the Go release of yaegi v2) under these engines:

- go1.27.3, the reference (the go command and the compiler, as testdir does);
- yaegi v2 v1 (`cmd/yaegi` of this repository, formerly `govm`), also with
  `-aot`;
- yaegi v2 v2 (`yaegi -engine=v2`, an experimental second engine built on a
  work branch that is not part of this tree), for information;
- yaegi v0.16.1 (`cmd/yaegi` of github.com/traefik/yaegi, the version of
  `compare/go.mod`), twice: with the default symbols, and with
  `-unsafe -syscall -unrestricted` (all symbol tables, the fairest setting).
  Only the first measure (19 October 2026, `run` and `errorcheck`).

The last measure of all the engines, with all the actions below, is of the
suite of go1.26.9, 23 October 2026: `main` at d418abb (v1), the v2 work
branch at ee82dcf (v2), linux/amd64, 6 jobs, 60 s timeout per test. See [Results by
action](#results-by-action-23-october-2026). `results/govm.tsv` is of
go1.27.3 (below); `go.tsv`, `govm-aot.tsv`, `govm-v2.tsv` and the yaegi
files are still of go1.26.9 (`run.sh` on linux/amd64 measures them again).

**CI gate and frozen pass list.** `gate.sh` runs the suite (all the actions)
under yaegi v2 v1 only and fails when a test that passes in `results/govm.tsv`
does not pass any more (new passes are printed and do not fail). CI runs it
on each change (job "GOROOT test suite"), and keeps its `out/govm.tsv` as the
artifact `gotestdir-govm`. `results/govm.tsv` is that file of a CI
run (suite of go1.27.3, linux/amd64): 2343 frozen passes. To
freeze new passes, copy `out/govm.tsv` of a `gate.sh` run on linux/amd64
(the artifact), or run `run.sh` there, and commit `results/govm.tsv`.

The engine key and the file names of yaegi v2 in the results are `govm`, the
name of yaegi v2 when they were measured.

History of the frozen list: on 19 October 2026 (`main` at 1d12b3e plus
the CI hardening work) `run` had 1015 passes of 1040 and `errorcheck` 450.
On 21 October 2026, `fixedbugs/issue36516.go` was taken out of the frozen
passes: since bfc9660 a loop of `unsafe.Pointer(uintptr(x) + 1)` and `- 1`
allocated (18 per round, the test wants 0). It is back since October 23,
2026: `uintptr(&a[i])` of an array stored as cells is now the address of
the array plus the gc offset of the element (`uregion.at`).
On 23 October 2026, the packages of a main
module without a dot in its path stopped running as native packages: 65 new
frozen passes (64 `rundir` and `fixedbugs/bug248.go`), 2279 in all.
`fixedbugs/issue29919.go` was taken out: it passed by mistake, because the
init of `test/a` did not run at all (see cause 1 below).
With the suite of go1.27.3 (yaegi v2 on Go 1.27): 2343 frozen passes. No test
that passes in both suites stopped passing. `fixedbugs/issue23609.go` and
`fixedbugs/issue26416.go` left the measure: go1.27.3 runs them as
`errorcheck -lang=go1.26`, and an errorcheck with a compiler flag is not
run (see "What is tested").

## How to run

```bash
compare/gotestdir/run.sh [jobs] [timeout]   # default: 8 jobs, 60s
compare/gotestdir/gate.sh [jobs] [timeout]  # yaegi v2 only, against results/govm.tsv; default: 4 jobs, 60s
```

`run.sh` measures go, yaegi v2 (`results/govm.tsv`) and yaegi v2 -aot
(`results/govm-aot.tsv`). With `YAEGI_V2=<path of a yaegi v2 binary built on the v2 work branch>` it
also measures v2 (`results/govm-v2.tsv`). It measures yaegi too (`run` and
`errorcheck` only), unless `GOTESTDIR_NO_YAEGI=1`.

It clones `test/` of golang/go (sparse, once, in
`~/.cache/yaegi-gotestdir/go1.27.3`; set `GOTESTDIR_ROOT` to use another copy
of `test/`), gets the go1.27.3 toolchain, builds the binaries with it and
writes `results/<engine>.tsv`. Columns: file, action, status, seconds,
detail (the start of the output, or for `FAIL_OUTPUT` the line where the
output starts to differ from the `.out` file).

The test directory must not be inside a module (`gotestdir` checks it):
testdir runs the tests in `GOROOT/test`, which has no `go.mod`, so the
toolchain defaults apply (language version, GODEBUG). Inside this checkout,
yaegi v2's `go.mod` would set them. Like testdir, the tests run with
`GOENV=off` and an empty `GOFLAGS`.

Each command runs in its own process group. On timeout the whole group is
killed (also the binary started by `go run`), and `TIMEOUT` means that the
test was killed by the timeout (the timeout is for the whole test, all its
commands). Local paths (test directory, GOROOT, GOMODCACHE, repository,
`$HOME`, `/tmp`) are removed from the details before they are cut; `run.sh`
fails if `/home/` or `/tmp/` is left.

## What is tested

`main.go` follows the rules of `cmd/internal/testdir` of go1.27.3 (the
official runner; its changes since go1.26.9 are for `asmcheck` and Windows
paths only):

- directories: `.`, `ken`, `chan`, `interface`, `internal/runtime/sys`,
  `syntax`, `fixedbugs`, `abi`, `typeparam`, `typeparam/mdempsky`;
- the recipe is the first comment line that is not a build constraint; files
  whose build constraints exclude linux/amd64 are skipped;
- flags of the recipe: `-goexperiment` and `-godebug` add to the
  environment, `-t N` is the timeout of the test (N seconds, as in testdir:
  `initexp.go`, `fixedbugs/issue10958.go`), `-s` makes each file of a
  directory a package, `-gomodversion` sets the go version of `runindir`.
  Other flags (`-gcflags=...`, `-ldflags`, `-l=4`, `-race`...) go to the go
  command or the compiler, and are ignored for the interpreters;
- `asmcheck` (gc assembly) and `skip` are not run. go1.27.3 has no
  `rundircmpout` action.

The actions, by kind (number of tests counted on linux/amd64):

- **run-like**: the program runs; exit status 0 and stdout+stderr identical
  to the `.out` file (empty when there is no `.out` file).
  - `run` (1040): the file. Extra `.go` files in the recipe are part of the
    program, as with `go run a.go b.go` (`cmplxdivide.go` has
    `// run cmplxdivide1.go`): the interpreters get a directory with all the
    files. go uses `go run`.
  - `rundir` (115): the directory `x.dir` of `x.go` holds packages, compiled
    in the order of their file names (a new package for each new package
    name); the last one is the main package and runs, with the arguments of
    the recipe. go compiles each package with `go tool compile -D test` and
    links, as testdir does.
  - `runindir` (10): the directory is copied into a module of its own (in a
    temporary GOPATH, language version 1.14 or `-gomodversion`), and runs
    with `go run .` (go) or as a directory (interpreters).
  - `runoutput` (22): `go run x.go` (always with the go command) writes a
    program; the engine runs this program.
  - `buildrun` (1): go builds the file and runs the binary; the
    interpreters run the file.
  - `buildrundir` (4): all the `.go` and `.s` files of the directory are one
    main package. All 4 have assembly files.
  - `errorcheckandrundir` (1 counted, `fixedbugs/bug248.go`): the package
    before the last must be refused (errorcheck rule below), then the last
    package runs. The 4 others check `-m` or `-d=` diagnostics of gc and
    are not counted.
- **compile-like**: the packages must compile.
  - `compile` (514): the file, `compiledir` (124): all the packages of the
    directory in order, `build` (30): `go build` of the file, `builddir`
    (2): the `.go` and `.s` files of the directory.
  - go compiles as testdir does. yaegi v2 has no "load only" mode, and these
    tests must not run (many have a main function that loops or exits), so
    for yaegi v2 the check is **`loadcheck`**: the packages load with the front
    end of yaegi v2 v1 (go/packages with LoadSyntax, then LoadAllSyntax when a
    package outside the standard library is involved, as `vm.RunMain` does),
    type-check without error, and build their SSA code (go/ssa,
    InstantiateGenerics). It does not check the native symbol tables and
    does not compile bytecode (yaegi v2 compiles a function at its first call).
    It is a check of v1's front end: it is measured once, in `govm.tsv`, and
    not for `-aot` (same front end) nor for v2 (another loader).
- **errorcheck-like**: the engine must refuse the program; see "The
  errorcheck rule" below. As for `errorcheck`, only tests without compiler
  flags are counted.
  - `errorcheck` (522): the file. Tests with compiler flags (`-m`,
    `-d=...`, `-live`, `-lang`...) are not counted: they check gc
    diagnostics, not the language.
  - `errorcheckdir` (21): packages in order as for `rundir`; the last one
    must be refused, the ERROR comments of all the files count. 4 tests with
    `-0 -m` are not counted.
  - `errorcheckoutput` (4): `go run x.go` writes a program, `tmp__.go`,
    that the engine must refuse.
  - `errorcheckwithauto` (0): its 5 tests all check `-m` or `-live`
    diagnostics.

**Test directories for the interpreters.** gc compiles the packages of
`x.dir` with `-D test`: the import path `"./a"` means `test/a`, the path of
the package of `a.go`. An interpreter loads packages with the go command
(go/packages), which refuses relative imports in modules. So the
interpreters get the packages in a module `test`, one directory per
package, named like its path for gc (`test/a`), and the strings `"./a"` of
the import declarations are written `"test/a"` (only these strings; the
line numbers do not change). A file `x_test.go` is copied as `x_test_.go`:
the go command leaves test files out of a package, gc does not. The
interpreter runs (or must refuse) the directory of the last package (of the
package before the last for `errorcheckandrundir`). `buildrundir` and
`builddir` get a module `prog` with the files of the directory.

The go reference passes all these tests except 5 errorcheck tests of the
`types2Failures` list of testdir, which are expected to fail with the
current gc (`shift1.go`, `fixedbugs/issue20233.go`; `errorcheckdir`:
`fixedbugs/issue10700.go`, `fixedbugs/issue18419.go`,
`fixedbugs/issue31053.go`).


**Compiler flags of `run` tests** (`-gcflags=...`, `-race`, `-tags`, 16
tests) are given to `go run` but ignored for the interpreters: they have no
compiler. Most of these flags do not change the output (`-d=checkptr`,
`-l`, `-N`), but some do. `convert5.go` has
`// run -gcflags=-d=converthash=qy`: this flag turns on the new code of gc
for float to integer conversions out of range, and the `.out` file holds the
results of this code. An interpreter that converts like the default
compiled code (yaegi v2) fails this test, and so does plain `go run convert5.go`
without the flag. Such a failure is a limit of the measure, not always a bug.

### The errorcheck rule

testdir compiles the file and requires that every error of gc matches the
`// ERROR "regexp"` comment of its line, and that every ERROR line gets an
error. The interpreters stop earlier (yaegi stops at the first error), so
`gotestdir` uses this rule for every engine, the reference too:

1. the engine must refuse the program at compile time: the exit code of a
   compile error for this engine (go: 2 from `go tool compile`, yaegi v2: 4,
   yaegi: 1) and no `panic:`, `fatal error:`, `[signal` or goroutine dump in
   the output. A crash of the interpreter is `ERR_CRASH`, not a rejection;
2. every error that the engine reports on this file (`file.go:LINE[:COL]:
   msg`, with a left boundary so that `xbug1.go` is not `bug1.go`) must be on
   a line with an ERROR comment (else `ERR_WRONGLINE`);
3. the message of each error must match one of the regexps of its line, as
   in testdir: the text after `file:line:col: ` is matched, `LINE` becomes
   `file.go:N`, and paths of the file in messages become its base name (else
   `ERR_WRONGMSG`);
4. unlike testdir, the engine does not have to report every ERROR line.

Not counted as errors of their own: lines that start with a tab after the
position (the second part of an error, "other declaration of x"), `too many
errors`, and yaegi's suffix `(and N more errors)`.

The wording counts in point 3. gc's type checker (types2) and `go/types`
share almost all messages, but yaegi v2 reports syntax errors with `go/parser`,
whose messages ("expected ';', found ','") are not those of gc's parser
("unexpected comma"); and after a syntax error it also reports the next
errors of `go/parser` and of `go/types`, which gc does not. yaegi has its own
messages.

For the errorcheck-like tests of a package that is not `main`, yaegi v2 refuses
to run a package that loaded without error ("is not a main package", v2:
"expected package main"): this is `ERR_MISSED`, the error was missed.

Three `run` tests call the compiler with the environment of testdir and fail
under plain `go run` too (`bug369`, `issue21317`, `linkobj`): they are left
out.

## Results by action (23 October 2026)

Passed / counted, per engine (`main` at d418abb, the v2 work branch at ee82dcf,
6 jobs, 60 s per test). `-` means not measured: the compile-like tests use
`loadcheck`, the front end of v1 (see above).

| action | go1.26.9 | yaegi v2 v1 | v1 `-aot` | v2 |
|---|---|---|---|---|
| `run` | 1040/1040 | **1023/1040** | 1022/1040 | 1031/1040 |
| `rundir` | 115/115 | **46/115** (109/115 with cause 1 fixed) | 47/115 | 1/115 |
| `runindir` | 10/10 | **6/10** | 6/10 | 6/10 |
| `runoutput` | 22/22 | **20/22** | 19/22 | 21/22 |
| `buildrun` | 1/1 | **1/1** | 1/1 | 1/1 |
| `buildrundir` | 4/4 | **0/4** | 0/4 | 0/4 |
| `errorcheckandrundir` | 1/1 | **0/1** (1/1 with cause 1 fixed) | 0/1 | 0/1 |
| `compile` | 514/514 | **496/514** | - | - |
| `compiledir` | 124/124 | **120/124** | - | - |
| `build` | 30/30 | **30/30** | - | - |
| `builddir` | 2/2 | **2/2** | - | - |
| `errorcheck` | 520/522 | **450/522** | 450/522 | 459/522 |
| `errorcheckdir` | 18/21 | **17/21** | 17/21 | 17/21 |
| `errorcheckoutput` | 4/4 | **3/4** | 3/4 | 4/4 |

### yaegi v2 v1 failures by cause (all actions but `errorcheck`)

The `errorcheck` failures (72: wording of `go/parser` and follow-on errors)
are described in [yaegi v2 failures](#yaegi-v2-failures) below.

1. **Loader: the packages of a main module whose path has no dot ran as
   native packages** (66 tests, now fixed).
   In a module `test` (the import paths of gc for test directories), v1
   took `test/a` for a package of the standard library ("no dot in the
   first path element"): `govm: unsupported: native func test/a.F not in
   registry`. v1 now uses the list of the standard library packages of the
   release (`stdtypes.IsStd`). 65 of the 66 tests pass and are frozen. Left:
   - `fixedbugs/issue42401.go` (`rundir`): `//go:linkname s test/a.s` on a
     variable of another interpreted package; the write does not reach
     `a.s` ("panic: FAIL");
   - `fixedbugs/issue29919.go` (`rundir`) passed by mistake before the fix:
     the init of `test/a` (imported with `_`) did not run. It now runs, and
     the test fails on its last check: the frame after `test/a.init` is
     `main.init` (the init driver of the VM), not a `runtime.` function.
2. **Wrong output or wrong panic** (5):
   - `fixedbugs/issue31636.go` (`rundir`): package inits run in the order
     c, b, a; the spec wants the order of the imports (a, b, c);
   - `fixedbugs/issue18911.go` (`rundir`): `panic: interface conversion:
     interface {} is struct { x int }, not struct { x int } (types from
     different scopes)`;
   - `chan/select5.go` (`runoutput`): `invalid order 1 after 100`, a
     `select` evaluates its cases in a wrong order in the generated program;
   - `slice3.go` (`runoutput`): `array[0:0:0]` gives a wrong pointer
     (`351608 0 0`, want `0-9 0 0`). v2 passes;
   - `fixedbugs/issue50788.go` (`errorcheckdir`): go/types gives the cycle
     lines of "invalid recursive type T" as separate errors; the test wants
     them in the message, as gc prints them (wording).
3. **Wording** (1): `nul1.go` (`errorcheckoutput`): go/parser says
   "expected declaration, found 'ILLEGAL'", the test wants "NUL". v2
   passes.
4. **Too slow or flaky** (1): `typeparam/chansimp.go` (`rundir`) timed out
   at 6 jobs but passes alone in 2.4 s (the scratch copy with `x.test`
   passes it too). The 2 `run` timeouts are listed below. With `-aot`,
   `rangegen.go` (`runoutput`, 44 s under v1, 45 s under go: the generated
   program is big) and `fixedbugs/issue79186.go` (`run`, 35 s under v1)
   also time out.
5. **Out of scope for an interpreter** (34):
   - function without body (22): gc accepts a function declaration without
     body when it compiles a file alone; `go build` refuses it ("missing
     function body"), and so does the front end of yaegi v2. `compile`:
     `func2.go`, `import.go`, `live1.go`, `parentype.go`,
     `fixedbugs/bug089.go`, `bug150.go`, `bug245.go`, `bug443.go`,
     `issue17596.go`, `issue28430.go`, `issue31010.go`, `issue31915.go`,
     `issue43677.go`, `issue45323.go`, `issue45344.go`, `issue53454.go`,
     `issue58826.go`, `issue74836.go`; `compiledir`: `import2.go`,
     `fixedbugs/bug392.go`, `issue43479.go`; `rundir`:
     `fixedbugs/issue9608.go`;
   - assembly (6): `buildrundir`: `asmhdr.go`, `linknameasm.go`,
     `retjmp.go`, `fixedbugs/issue37513.go`; `runindir`:
     `fixedbugs/issue15609.go`, `issue74648.go`;
   - cgo (1): `fixedbugs/issue47185.go` (`runindir`);
   - linker or compiler feature (1): `fixedbugs/issue20014.go`
     (`runindir -goexperiment fieldtrack -ldflags -k=...`: field tracking
     of the linker);
   - import path refused by the go command (1): `fixedbugs/issue27836.go`
     (`compiledir`, files `Þfoo.go`, `Þmain.go`);
   - the 3 `errorcheckdir` tests that gc fails too (`types2Failures`):
     `fixedbugs/issue10700.go`, `issue18419.go`, `issue31053.go`.

The 17 `run` failures of v1: too slow `abi/uglyfib.go`,
`fixedbugs/issue13169.go` (they time out alone too); crash `fixedbugs/issue13160.go` (nil pointer in
a goroutine, v2 passes), `fixedbugs/issue34395.go` and `nilptr.go` (index
out of range [0] with length 0 on large arrays); missing feature
`runtime/cgo.Incomplete` (`fixedbugs/bug514.go`, `issue42032.go`,
`issue42076.go`, `issue46903.go`; v2 passes); runtime introspection
`finprofiled.go`, `heapsampling.go`, `fixedbugs/issue4618.go`
(AllocsPerRun); out of scope: cgo (`fixedbugs/issue34968.go`,
`issue36705.go`, `issue47227.go`), compiler flags (`convert5.go`,
`maymorestack.go`).

**v1 `-aot`** has the results of v1, plus the timeouts above.

**v2** (for information): its loader reads the other packages of the
program from export data, so 114 of the 115 `rundir` tests stop with
`could not import test/a (can't find export data for test/a)`. On `run` it
passes 8 tests that v1 fails (`runtime/cgo.Incomplete`, `issue13160`,
`issue34395`, `issue4618`, `finprofiled`...) and fails `closure.go`
("newfunc allocated unexpectedly"). On `errorcheck` it has 8 `ERR_MISSED`
(`directive.go`, `fixedbugs/issue18459.go`... are accepted) and 2 crashes
(`fixedbugs/issue49767.go`: `reflect.ChanOf: element size too large`,
`fixedbugs/issue49814.go`: out of memory).

## First measure (19 October 2026): `run` and `errorcheck`

| | go1.26.9 | yaegi v2 | yaegi | yaegi (all symbols) |
|---|---|---|---|---|
| `run` passed (of 1040) | 1040 | **866 (83%)** | 573 (55%) | 628 (60%) |
| `run`, wrong output with no error | 0 | **24** | 58 | 60 |
| `run`, timeout | 0 | 8 | 10 | 12 |
| `errorcheck` passed: right lines and messages (of 522) | 520 | **450 (86%)** | 151 (29%) | 156 (30%) |
| `errorcheck`, rejected with every error on an ERROR line | 521 | **488** | 288 | 299 |
| `errorcheck`, rejected at compile time | 522 | **522** | 348 | 334 |
| `errorcheck`, interpreter crash (`ERR_CRASH`) | 0 | **0** | 54 | 61 |
| `errorcheck`, accepted (error missed) | 0 | **0** | 118 | 125 |
| `errorcheck`, timeout | 0 | 0 | 2 | 2 |

Each `errorcheck` line contains the line above it: "passed" is
`PASS`, "every error on an ERROR line" adds `ERR_WRONGMSG`, "rejected at
compile time" adds `ERR_WRONGLINE`.

The 2 `errorcheck` failures of go1.26.9 (`shift1.go`,
`fixedbugs/issue20233.go`) are in the `types2Failures` list of testdir: they
are expected to fail with the current gc.

yaegi v2 rejects all 522 programs at compile time and accepts none by mistake.
Its 72 other failures are wording and extra errors: 38 have the error on the
right line with the message of `go/parser` (`ERR_WRONGMSG`), and 34 report a
second error on a line without an ERROR comment (`ERR_WRONGLINE`), mostly
`go/parser` going on after the first syntax error ("expected ';', found
'EOF'"), or `go/types` errors after a syntax error ("could not import os",
"declared and not used") that gc does not print.

The first version of this measure counted any failure that named an ERROR
line as a pass (yaegi v2 499, yaegi 306, yaegi with all symbols 319). It also
counted the crashes of yaegi on an ERROR line, and it did not look at the
messages. The yaegi numbers above are much lower: they count only real
compile-time rejections, with the messages that the tests expect.

`run` tests, yaegi v2 against yaegi with all symbols: 582 pass under both, 284
only under yaegi v2, 46 only under yaegi, 128 fail under both.

| Directory (`run`) | yaegi v2 | yaegi (all symbols) |
|---|---|---|
| `.` | 93/145 | 66/145 |
| `abi` | 35/36 | 32/36 |
| `chan` | 17/17 | 7/17 |
| `fixedbugs` | 535/641 | 418/641 |
| `interface` | 10/11 | 6/11 |
| `ken` | 37/40 | 37/40 |
| `typeparam` | 133/141 | 61/141 |
| `typeparam/mdempsky` | 6/9 | 1/9 |

## yaegi v2 failures

**The yaegi v2 command (then `govm`) runs with the restricted symbol table.** The native
registry is yaegi's restricted table, also without `-restricted`. So
`os.Exit(n)` panics (`panic: os.Exit(n)`, exit status 2) and `os/exec` is
missing. This contradicts the doc comment of `cmd/govm` (now `cmd/yaegi`) ("A program calling
os.Exit keeps its code"). It explains 25 of the 46 tests that pass only under
yaegi (21 `os/exec`, 4 `os.Exit`).

**Semantic bugs** (wrong output or a wrong panic):

- integer division overflow is not wrapped: `int8(-128) / int8(-1)` gives
  `128` when the values come from parameters (`divmod.go`, `divide.go`).
  yaegi is right. Smallest program:

  ```go
  func f(x, y int8) { println(x / y) } // prints 128, want -128
  func main()       { f(-128, -1) }
  ```

- types declared inside a generic function become `struct {}`
  (`typeparam/nested.go`, `typeparam/mdempsky/15.go`, `17.go`,
  `typeparam/issue49547.go`);
- out-of-range float to integer conversions (`ken/convert.go`; `convert5.go`
  also, but it needs a compiler flag, see above: its output under yaegi v2 is not
  the output of plain `go run` either);
- `print`/`println` of nil pointers, maps, slices and funcs prints `<nil>`
  instead of `0x0` and `[0/0]0x0` (`print.go`, `goprint.go`, `deferprint.go`);
- `reflect` methods on interpreted types (`reflectmethod1.go` to
  `reflectmethod7.go`, `fixedbugs/issue27695*.go`, `issue16331.go`);
- other wrong results: negative index bounds checks (`issue30116.go`),
  comparisons that must panic (`issue8606.go`, `interface/noeq.go`),
  zero-size objects (`zerosize.go`, `bug352.go`), `string_lit.go`,
  `literal.go`, `blank.go`, `const.go`, `shift3.go`, `defernil.go`.

**Expected limits of an interpreter:** runtime introspection
(`runtime.Caller`, stack traces, line numbers in panics), GC and finalizer
timing, `testing.AllocsPerRun`, `maymorestack`; `unsafe` code that reads
memory as bytes (StringHeader, SliceHeader); cgo; the SSA instruction
`SliceToArrayPointer` is not supported yet (3 tests).

## yaegi failures

Mostly internal panics (`nil type`, nil pointer dereference,
`reflect.Type is nil`), generics (61/141 in `typeparam`), channels (7/17),
missing builtins (`clear`), and 125 invalid programs accepted.
