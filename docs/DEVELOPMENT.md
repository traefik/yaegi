# Developing yaegi v2

How the repository is organized, how to build and test it, what CI runs,
and how yaegi v2 follows new Go releases.

## Layout

The root module (`github.com/traefik/yaegi/v2`) holds the library, its
command, its generators and its tests. The comparison and study tools,
which need yaegi v0, are a separate module in `compare/`: they are not part
of what `go get` downloads.

| Path | Content |
|---|---|
| `host/` | the entry point of a host that loads plugins it does not trust: the safe options (`Options`), their check (`Check`) and `Load`; it imports the AOT copies |
| `vm/` | the VM library: loader, compiler, executor, native boundary, VM reflect, reflect twin, AOT runtime, plugin API (`plugin.go`), guard rails |
| `cmd/yaegi/` | the command: run a Go main package, `-e`, `-i` |
| `stdtypes/`, `cmd/stdexport/` | standard library types (gc export data) and the sources of a few standard packages, embedded per platform so that code loads without a toolchain; their generator (`go generate ./stdtypes`) |
| `stdsyms/`, `cmd/stdsyms/` | native symbol table of the standard library for the Go release of the host; its generator (`go generate ./stdsyms`) and the extractor it uses (`internal/extract`, derived from yaegi v0). `stdsyms/sideeffect`: the tables of expvar, net/http/pprof and testing/quick, whose init changes the host |
| `vreflect/` | facade of the reflect twin: the API of `reflect`, imported by AOT copies in its place |
| `cmd/aotgen/`, `aot/`, `natsyms/` | AOT generator; the generated copies (standard library and third-party, each with its license) and the native symbol tables of non-standard packages; one tree per Go release, `aot/go1_NN/` with its `REPORT.txt`, and `aot/all` |
| `cmd/aotcheck/`, `testdata/aot/` | AOT test programs, run in native, interpreted and AOT modes against `go run` |
| `cmd/aotgosrc/`, `internal/gosources/` | the manifest of the Go sources of the AOT copies, per Go patch release |
| `cmd/hostgraph/`, `testdata/hostgraph/` | checks that the AOT copies fit the module graph of Traefik |
| `cmd/boundaryscan/`, `internal/boundary/`, `testdata/boundary/` | the inventory of the native boundary of the standard library ([BOUNDARY.md](BOUNDARY.md)) and its probes |
| `cmd/stdtests/` | the external tests of standard library packages, run natively, in VM mode and in AOT mode ([STDTESTS.md](STDTESTS.md)) |
| `cmd/runtests/` | the conformance runner over the yaegi v0 test programs, and the Go tests that compare `testdata/` with `go run` |
| `cmd/loadtime/`, `cmd/loadmem/` | plugin load times; heap, RSS and request times of one plugin |
| `cmd/progen/`, `internal/progen/` | random programs compared with Go |
| `cmd/stdscan/`, `cmd/reflscan/` | what interpreting the standard library from source requires; reflect API used by packages |
| `testdata/` | programs compared byte for byte with `go run`; `testdata/modes/` is the reflect battery |
| `tests/yaegi/` | the conformance programs and API tests of yaegi v0, with provenance (`MANIFEST.tsv`) |
| `plugins/` | small Traefik-like plugins used by tests and benchmarks |
| `compare/` | separate module: the Traefik plugin catalog harness (`catalog/`), the GOROOT test suite (`gotestdir/`), benchmarks against native and yaegi v0 (`bench/`), the study of programs under yaegi v0 and v2 (`yaegi/`), larger plugins (`complex/`) |
| `docs/` | architecture and reference documents, measured results |
| `scripts/` | setup, AOT generation driver, test importer |

## Build, run, test

The tools run offline (`cmd/runtests` and `cmd/aotcheck` force
`GOPROXY=off`). Fill the module cache once with `scripts/setup.sh` (it needs
the network), then work with `GOFLAGS=-mod=readonly GOPROXY=off
GOSUMDB=off`. `go.mod` and `go.sum` (root and `compare/`) are tidy, so
`-mod=readonly` (the CI setting) works offline.

```bash
sh scripts/setup.sh                                           # once: modules, build, vet
go build ./... && go vet ./...
go run ./cmd/yaegi testdata/fib.go                            # VM mode
go run ./cmd/yaegi -aot testdata/aot/json.go                  # AOT mode
go run ./cmd/yaegi -mode encoding/json=aot testdata/aot/json.go # one package in AOT mode
go run ./cmd/yaegi -src slices,maps,iter testdata/g3.go       # interpret these std packages from source
go run ./cmd/yaegi -stats -budget 1000000 testdata/loop.go    # timings; execution budget
go run ./cmd/yaegi -restricted testdata/fib.go                # Traefik plugin restrictions
sh scripts/aotgen.sh                                          # regenerate aot/ and natsyms/
go run ./cmd/aotcheck -v                                      # AOT programs in every mode against go run
go run ./cmd/boundaryscan                                     # inventory of the native boundary
testdata/modes/check.sh                                       # reflect battery: no new silent difference

go test ./...                                     # testdata vs go run, conformance sample, AOT checks
YAEGI_CONFORMANCE=1 go test ./cmd/runtests        # whole conformance suite, regressions vs docs/conformance
go run ./cmd/runtests                             # full conformance run
go run ./cmd/stdtests                             # standard library tests: native, VM, AOT
go test -run 'TestGoCompare/collections/' ./cmd/runtests        # one area of testdata/gocompare, every mode
YAEGI_PROGEN_N=2000 go test -run TestGeneratedPrograms ./cmd/runtests # 2000 random programs vs go
go run ./cmd/progen -seed 42                      # the random program of a seed

cd compare && go test ./...                       # the comparison module
cd compare && go run ./catalog/run -j 4           # Traefik plugin catalog (compare/catalog/README.md)
compare/bench/run.sh final                        # benchmarks against native and yaegi v0
```

Environment variables of the VM: `VM_OPT=none` (or `-pool,-tuple,...`)
turns off optimizations, `VM_TRACE=1` prints the Go stacks of VM panics,
`VM_PROFILE=1` turns on the instruction profile.

**Tests that compare with Go.** Most tests run a program with `go` and
with yaegi v2 and compare the outputs: a change that makes the VM differ
from Go fails them.

- `testdata/*.go` (`TestTestdata`): the regression tests of past fixes;
- `testdata/gocompare/<area>/*.go` (`TestGoCompare`): programs by area of
  the language and of the standard library, each run in every mode
  (default, `-aot`, `-opaque`, `-restricted`); a new file is a new test
  (rules in `testdata/gocompare/README.md`);
- random programs (`TestGeneratedPrograms`, `internal/progen`): arithmetic,
  conversions, shifts, panics, strings, and the value model (copies,
  pointers, aliasing, methods, interfaces). Pull requests run 40 fixed
  seeds; the nightly job "differential" runs 2000 new seeds;
- `testdata/aot/` (`cmd/aotcheck`), the reflect battery
  (`testdata/modes/check.sh`), the GOROOT test suite (`compare/gotestdir`)
  and the conformance suite (`cmd/runtests`).

**Options for tests and studies.** `Options.Src` (`yaegi -src`) interprets
standard library packages from source (needs the Go toolchain; correct but
slow). `Options.OpaqueAny` (`yaegi -opaque`, implied by `-aot`) passes
interpreted values to native `any` parameters as opaque handles.
`Options.GoList` loads with `go list` instead of the embedded data.

**32-bit.** The VM runs code for the platform it was built for. To try a
32-bit VM on an amd64 Linux machine:
`GOARCH=386 go build -o yaegi386 ./cmd/yaegi && ./yaegi386 testdata/intsize.go`.

## CI

`.github/workflows/ci.yml` runs on each pull request and every night. Each
pull request runs the gate (about 30 runner minutes): linux/amd64 (`go mod
verify`, `go mod tidy -diff`, gofmt, vet and all tests, for the root module
and `compare/`), the generated data, build and vet of the 32-bit
platforms, govulncheck and staticcheck. The nightly run also runs:

- linux/arm64, macOS arm64, Windows amd64, linux/386, and linux/arm and
  linux/s390x under qemu-user;
- `go build` and `go vet` for every platform of `gorelease.Platforms`;
- `go test -race` on `vm` and `vreflect`;
- the GOROOT test suite (`compare/gotestdir/gate.sh`): no test of the
  frozen pass list may stop passing;
- the standard library tests (`cmd/stdtests -check`) and the reflect
  battery (`testdata/modes/check.sh`);
- "next Go": the newest release candidate, the latest Go release and Go
  tip regenerate the generated data and run the checks of a port;
- "Go patch releases": the manifest of the Go sources of the AOT copies
  must be up to date.

The label `ci:full` on a pull request, or a run by hand, runs every job.
GitHub runs scheduled workflows on the default branch only. Actions are
pinned by commit SHA.

## Following a new Go release

Yaegi v2 supports two Go releases, N-1 and N. To follow a new release N+1:

1. Add it to `Window` in `internal/gorelease` and drop the oldest release.
   Set the `go` line of `go.mod` to the new oldest release (and update
   `golang.org/x/tools` if needed).
2. With the toolchain of N+1: `go generate ./stdtypes ./stdsyms ./vm`.
   Each run writes the files of its release and removes those of the
   releases outside the window.
3. With the same toolchain: `sh scripts/aotgen.sh` (AOT copies in
   `aot/go1_NN/` and `aot/all/go1_NN.go`).
4. Handle new SSA instructions in the VM, if any (`vm/compile.go`):
   `TestSSAInstructionsHandled` and `TestCompileCorpus` find them, and
   `TestRuntimeCouplings` fails when a start-up probe of a runtime internal
   fails ([ARCHITECTURE.md](ARCHITECTURE.md)). Behaviors of the standard
   library that changed go in `vm/release_go1NN.go`.
5. `go test ./...` with both toolchains, and `go run ./cmd/runtests`: no
   conformance regression.
6. Set `GO_VERSION` and `GO_VERSION_PREV` in `.github/workflows/ci.yml`.

The generated data has the build constraint of its release, so a build
only takes the data of its own release. `stdtypes`, `stdsyms` and `aot/all`
each have a `release_guard.go` that only builds with a release outside the
window and does not compile, with a message that says what to regenerate.
`TestSupportWindow` checks that `go.mod`, the build guards, the generated
data and the CI workflow agree on the window.

## Go patch releases

The AOT copies are copies of the sources of one Go patch release. A host
built with a newer patch release gets the security fixes in its native
standard library, not in the copies. In AOT mode, `vm.Load` compares the
version of the copies (`vm.AOTGoVersions()`) with the version of the host:

- **Same or older patch release than the copies**: AOT mode.
- **Newer patch release, same sources**: AOT mode. The manifest
  `aot/all/gosources.txt` has the hash of the sources of each copied
  package for each patch release; when none changed, no regeneration is
  needed.
- **Newer patch release, changed sources, or unknown version**: `vm.Load`
  refuses AOT mode, unless the host sets `Options.StaleAOTFallback` (the
  load goes on in VM mode, and `Plugin.Notes` says so) or
  `Options.AllowStaleAOT` (the old copies run; for an urgent release only).

`sh scripts/aotgen.sh` writes the manifest entry of the Go version it runs
with, and `go run ./cmd/aotgosrc -update` adds the newer patch releases
from the Go module proxy. When a patch release changed a copied package,
regenerate the copies with it.

A host such as Traefik should pin the full Go patch release it builds
with, move it together with yaegi v2, call `vm.CheckAOT(false)` in a test
of its release CI, and load with `StaleAOTFallback`.

Each Go release is supported with its default `GOEXPERIMENT`: a build that
flips `jsonv2` stops, since the generated data is for the default.
