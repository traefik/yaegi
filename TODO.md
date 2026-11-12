# TODO

The open work on yaegi v2, by priority. Each fix needs a regression test that
fails before it (see "Contributing" in the [README](README.md#contributing)).

## 0. Migration from go-vm to yaegi v2

Done: the interpreter formerly named go-vm is imported on branch `v2` on top
of `master` (yaegi v0). The module is `github.com/traefik/yaegi/v2`, the CLI
is `cmd/yaegi`, the study tools are in the separate module `compare/`, and
the docs and messages say "yaegi v2". Left:

- [ ] **Nightly CI.** GitHub runs scheduled workflows only on the default
  branch (`master`, yaegi v0), so the nightly run of `ci.yml` does not run
  for `v2`. Pull requests to `v2` and runs by hand work. Fix it when `v2`
  becomes the default branch, or with a scheduled workflow on `master` that
  starts `ci.yml` on `v2`.
- [ ] **Release of v2.** There is no release workflow on `v2` (the v0
  `release.yml` and `.goreleaser.yml` stay on `master`). Write one for
  `v2.x.y` tags, and decide which binaries it ships.
- [ ] **CLI subcommands.** The v0 CLI has `run`, `test` and `extract`
  subcommands that `cmd/yaegi` does not have: decide which ones to add.
- [ ] **Measured results.** The dated results (`compare/catalog/results/`,
  `compare/gotestdir/results/`, `docs/conformance/summary.txt`,
  `docs/conformance/stdlib-summary.txt`) were measured under the old name
  and keep it (the engine key `govm` of the catalog and gotestdir tools
  too). The next measure writes "yaegi v2".
- [ ] **Lint.** `v2` has no `.golangci.yml` (CI runs `go vet` and
  staticcheck). The v0 config does not fit this code: decide what to run.
- [ ] **Issue templates.** The v0 issue templates (`.github/ISSUE_TEMPLATE`)
  are not on `v2`.
- [ ] **Upstream.** Plan how and when `v2` is proposed to traefik/yaegi
  (needs the maintainers' approval).

## 1. Before an experimental flag in Traefik

- [ ] **Silent differences in the default ("VM") mode.** The reflect battery
  (`testdata/modes/check.sh`, nightly CI) runs 31 programs against the
  compiled program, in default mode and AOT mode; `testdata/modes/known.tsv`
  lists the cases that fail today. Remove its lines as fixes merge. Left:
  - decide whether a plugin that imports a template package, log/slog,
    encoding/gob, encoding/xml or encoding/json/v2 runs the standard library
    as in AOT mode (else the integration always uses AOT mode);
  - encoding/json/v2 forced native (`-mode encoding/json/v2=native`, or a
    host without the AOT copies) ignores the methods of the plugin's types
    (MarshalJSONTo, MarshalJSON, MarshalText) and their names, and can fail
    on a recursive type, as native json v1 did before `vm/lostmethods.go`.
    Give v2 the load-time check of lostmethods.go;
  - without the AOT copies, the old differences of fmt and templates are not
    checked by `vm/lostmethods.go`, and neither is gob;
  - in AOT mode, when the host logs a plugin value with a `LogValue` method,
    a group becomes one string (`g="[user=bob n=1]"`, not
    `g.user=bob g.n=1`). Convert a group of the copy into a native group
    (`vmObj.logValue` in `vm/optwrap.go`);
  - `encoding/xml` with a value that holds `xml.Name` or `xml.Attr` and an
    inner method, and the methods of `xml.Decoder` and `xml.Encoder`, are
    refused in VM mode (right in AOT mode). The twin would have to take the
    host's xml types as the copy's own, as it does for `json.Number`
    (`sameValType`);
  - a pointer to an interpreted `[]any` or `map[string]any` given to a
    routed call (`json.Marshal(&[]any{c})`) reaches the copy as a native
    copy of the elements: their methods are lost. `[]any{c}` without the
    pointer is right (`testdata/jsonanytext.go`);
  - a native map held in a plugin map type that native code gives back in an
    interface comes back unnamed.
- [ ] **Human review of `vm/`** (about 40,000 lines).

## 2. Guard rails

yaegi v2 is not a sandbox (see [docs/SANDBOX.md](docs/SANDBOX.md)). These
items make the guard rails stronger; none of them makes yaegi v2 a security
boundary.

- [ ] **Blocking native calls and the budget.** `Cancel` does not stop a
  goroutine blocked in a native call (network read, locks, waits). It uses
  no CPU and `MaxGoroutines` bounds how many can hang, but a request can
  hang. This is a design change: each blocking function needs a version for
  plugin code that also waits on the account of the call, and network reads
  need deadlines set by the per-plugin transport and dialer. Goroutines of
  native code that run no plugin code are not counted in `MaxGoroutines`.
- [ ] **Stronger isolation of native calls.** Native code reached by a
  plugin is not checked by the VM's write rules (`vm/hostmem.go`). Find a
  way to bound what native code can change on behalf of a restricted
  plugin.
- [ ] **Per-plugin state in the AOT copies.** Some package-level state of the
  AOT copies (gob registry, `flag.CommandLine`, type caches keyed by type
  address) is shared by all plugins of the process. A fix needs aotgen to
  rewrite these variables into per-machine state (as `aotvm.AOTArgs` does for
  `os.Args`), and goroutines started by AOT code to carry the machine of
  their parent. Type caches keyed by address should move to `vm.TypeCache`.
- [ ] **Fatal runtime errors.** With `Options.PanicOnFatal`, the fatal errors
  that the VM sees before they happen become panics of the plugin
  (`vm/fatal.go`). The same errors in native code, out of memory, and a
  deadlock of all goroutines still end the host, as in compiled Go. Document
  a memory limit (`GOMEMLIMIT`, cgroup) and a restart policy for Traefik.
- [ ] **What `MaxMemory` does not count**: allocations below 64 KiB
  (`memMin`), map growth, calls made by native code or by reflect's
  `Method`, and native functions that allocate from what they read
  (`encoding/json`, `io.Copy` into a `bytes.Buffer`, `fmt`).
- [ ] **`Options.Sandbox` is a best effort.** Its capability checks cover a
  subset of the os and net functions. Rename it, or make it a real allow
  list (checks for every function that takes a path, a filter in the
  per-plugin transport and dialer) before Traefik maps any manifest setting
  on it.
- [ ] **Optional out-of-process mode for providers only** (seccomp,
  Landlock, namespaces): their interface (`Init`, `Provide` with a channel
  of `json.Marshaler`, `Stop`) can be serialized. Not for middlewares (see
  `docs/SANDBOX.md`).
- [ ] **Load-time checks for the `GoList` loader.** The static checks of
  `vm/loadbomb.go` and `vm/loadinst.go` and `LoadLimits` run in the default
  loader only; with `Options.GoList` (or `Src`) only `LoadTimeout` bounds a
  load. Traefik uses the default loader.

## 3. Correctness

- [ ] **The native boundary: the probes that still differ from Go**
  (`docs/BOUNDARY.md`, `testdata/boundary/known.tsv`). Silent wrong results
  first: named types of the program that reach native code as their
  underlying type, optional methods outside the families of
  `vm/optwrap.go`, the AOT copies of gob and text/template, go/ast nodes of
  the program, a value-receiver fmt.Scanner in default mode, pkix, fmt of
  `*big.Int` with `-aot` on Go 1.27, context.String. Crashes: channels of
  the program, sort.Slice of a non-slice, a reflect.Type of the program in a
  native struct. Then the error messages. Fix one cause, then remove its
  lines from known.tsv. When a Go release changes the standard library, run
  `go run ./cmd/boundaryscan` with its toolchain.
- [ ] **Failures of the standard library tests** (`cmd/stdtests`,
  `docs/conformance/stdlib-causes.tsv`): silent wrong results in sort, fmt,
  math/big in AOT mode, optional methods (io, bufio), context and
  testing/quick. Also widen the packages: the internal tests are out of
  reach, and many packages have few external tests.
- [ ] **Native slices of structs, pointers or interfaces are copied**:
  writes through them do not reach native memory (slices of scalars are
  views). An element of these types has no VM form that points into the
  native array, so `&s[i]`, `s[i].F = v`, append, copy and range would each
  need a native path on the hot slice opcodes. Decide first if it is worth
  the cost.
- [ ] **Libraries that fail when the AOT copy falls back**: when a plugin
  vendors another version of an AOT library, the VM interprets it.
  go-toml/v2 stops on an `unsafe.Pointer` the VM does not know, and
  validator needs the assembly of `golang.org/x/sys/cpu`
  (`knownFallbackFailures` in `cmd/aotcheck`).
- [ ] **A plugin can recover from VM bugs**: `protected` (`vm/exec.go`)
  turns every Go panic of the interpreter loop into a panic of the program.
  A fix must tell a VM bug from the run-time errors that the VM lets Go
  raise on purpose (`OpDivI`, `OpRemI`, panics of native code), so it needs
  a list of the intended sites, not a filter on the frames.
- [ ] **Racy reads that still tear.** `vm/wordcell.go` protects the
  three-word cells (pointer, map, chan, func, interface) with a striped
  seqlock. Left, each with a test in the style of `testdata/tornwords.go`:
  slice variables (R may be a `[]Value`, a `*zslice` or a `reflect.Value`);
  a struct or array cell whose R itself is replaced; the cost of `copy` of
  `[]*T` (a CAS per element); measure the cost again on `BenchmarkPerfOps`
  and W1 to W3.
- [ ] **Panic values that still differ from gc**: `%T` of some recovered
  run-time errors is a type of the VM (`vm.boundsError`,
  `vm.runtimeError`, `*vm.typeAssertError`); native code that recovers the
  panic of a callback on its own goroutine gets a `*VMPanic` whose `Error()`
  starts with `panic: `; the first line of the yaegi command for an
  interpreted type with an `Error` or `String` method, or a named string
  type.
- [ ] **VM mode without the AOT copies: a template over an interpreted
  channel gives an error** (`{{range .C}}`, `{{len .C}}`). It is never
  silently wrong (`testdata/chaninany.go`). Routing `Execute` to the AOT copy
  does not work (different `Template` types), and a stand-in channel fed by a
  goroutine would lose values.
- [ ] **Init order: gc makes more data static than `initWork`**
  (`vm/initorder.go`). A package whose only data gc makes static (a `[]byte`
  of a constant string, an inlined call to a composite literal) and with no
  import that has init work can be initialized in another order. Rare.
- [ ] Generic APIs still missing: an `atomic.Pointer` that lives in native
  memory, and `sql.Null[T]` for a `T` that is a pointer, a slice or a map of
  a plugin type.

## 4. Performance

- [ ] **Measure W1 to W3 again** on a quiet machine
  (`compare/bench/run.sh final 10 300ms`); the current times were measured on
  a loaded machine (docs/BENCHMARKS.md).
- [ ] **Trampolines still missing**: `...any` arguments other than basic
  types, the methods of packages outside `typesPkgs` of
  `vm/natsigs_gen.go`, and interface parameters other than `error`,
  `io.Reader`, `io.Writer` and `context.Context`.
- [ ] **Slice views are 2.8× slower than copies**: copy the slices freshly
  returned by a native call, keep views for reads of native memory. Only
  functions known to return a new slice (`strings.Split`...) may be copied.
- [ ] **Load time**: the largest catalog plugin loads in about 226 ms, most
  of it in go/types and SSA; only a cache of the result on disk would remove
  it.
- [ ] **AOT to VM callbacks without `reflect.Value`**: methods of interpreted
  values that AOT wrappers call still go through `Obj.Call`; a typed call per
  signature needs a change of the wrappers that cmd/aotgen writes.
- [ ] Memory and size, to watch: heap +25 % against yaegi v0 in the 10 minute
  soak, binary +19 MB.

## 5. Go releases and platforms

- [ ] **Measure the other engines on the GOROOT suite of go1.27.3** with
  `compare/gotestdir/run.sh` on linux/amd64 (only `results/govm.tsv` is of
  go1.27.3 today).
- [ ] **`golang.org/x/sys/cpu` from the host**, as `x/sys/unix` already is
  (`vm/xsys.go`), so wazero, validator and crypto libraries see the host's
  CPU features. Also: tables for the unix systems outside
  `stdtypes.Platforms` (netbsd, dragonfly, solaris, aix) if needed.

## 6. Code health

- [ ] Split the plugin API of package `vm` from the ABI of the generated code
  (`Instr`, `RV`, `RT`, `RegisterAOT`, `AOTServe`...): a new package (for
  example `vm/abi`), a change of `cmd/aotgen` and `cmd/stdsyms`, and a
  regeneration. Decide the package boundary first.
- [ ] Move `VM_OPT`, `VM_TRACE`, `VM_PROFILE` from environment variables to
  `Options`, or at least read them only in `cmd/yaegi` and `compare/bench`,
  so a host that imports `vm` is never changed by its environment.
- [ ] Review the state keyed by goroutine address (budget accounts,
  `gstate`).
- [ ] Find dead intrinsics in `vm/vmreflect.go` with coverage of `yaegi` runs
  over the testdata, conformance and catalog programs.
- [ ] Unify the plugin's own reflect with the reflect twin, to give VM mode
  the whole reflect API.
- [ ] Aliasing of unnamed composites in AOT code (`Unbox` copies structs and
  arrays), and `Value.Bytes` on a `[]byte` stored as `[]Value`.
- [ ] The repository is heavy (generated data, AOT copies, test corpora).
  The `go 1.27.0` line of `go.mod` is on purpose (README, "Supported Go releases").
- [ ] Eval mode (`vm.Session`, `yaegi -e`, `yaegi -i`) is a prototype; its
  limits are listed at the top of `vm/eval.go` (a method must be declared in
  the same input as its type; no imports outside the standard library).

## 7. Integration into Traefik

This work touches traefik/traefik and needs the maintainers' approval.

- [ ] Release the old plugins at each reload with `Plugin.CloseContext`
  (`vm/close.go`, `docs/RELOAD.md`). Left: whether a plugin whose
  configuration did not change keeps its `Plugin`.
- [ ] An adapter in Traefik's plugin loader (the API mapping is in the
  README), behind an experimental flag, opt-in per plugin.
- [ ] AOT mode only for plugins, with the AOT copies chosen from the catalog.
- [ ] A Traefik test that calls `vm.CheckAOT(false)`, so a Go update without
  a yaegi v2 update fails Traefik's CI; the full Go patch release in
  Traefik's `.go-version`; and `Options.StaleAOTFallback` with a log of
  `Plugin.Notes` in the adapter.
- [ ] Automatic fallback to yaegi v0, plugin by plugin, with a log line
  saying why (a `*vm.LoadError` or `*vm.PreflightError`); yaegi v0 on the
  platforms without data.
- [ ] Plugins without a `go.mod`: set `Options.Root`, and for local plugins
  only `Options.GoPath` (GOPATH mode, as with yaegi v0). Still refused: a
  `go.mod` whose module path does not hold the import path, and a dependency
  of a `go.mod` plugin that is in `<goPath>/src` but not vendored.
- [ ] Keep the double `useUnsafe` gate; never set `Unrestricted` or
  `RealProcess` from plugin data.
- [ ] Import only package `host`, start from `host.Options()`, pass `Env`
  (empty by default, or a per-plugin filter as `pluginhost.EnvFilter`),
  `GoPanic`, `Stdout` and `Stderr`, and load with `host.Load`. Decide
  whether `host.Options` also sets `StaleAOTFallback`.
- [ ] Shadow traffic against yaegi v0 for one release, and a soak on real
  traffic.
