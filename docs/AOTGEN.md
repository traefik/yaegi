# aotgen: compile reflect-based libraries ahead of time against the VM

Legend: **[M]** measured, **[E]** estimated, **[O]** opinion.

## Summary

**AOT copies give the output of `go run` for 24 of the 25 libraries we evaluated.** [M] (The README explains where AOT mode sits among the execution modes.)

- `cmd/aotgen` copies a library and rewrites its reflection against the VM's reflect twin (`vreflect`). The copy is compiled to native code and sees the plugin's types and methods.
- 47 of the 49 planned packages compile. The two exceptions are json-iterator and its dependency modern-go/reflect2.
- Out of 25 plugin programs, **24 give in AOT an output that is byte-for-byte identical to `go run`**. These programs define types with `MarshalJSON`, `String`, `UnmarshalYAML`, `GobEncode`, `Format`, hooks, a `driver.Driver`, etc. The 25th, json-iterator, matches too: AOT mode falls back to interpreted mode for it.
  - In native mode, 0 of 25 are identical (the reflect wall).
  - In interpreted mode (stdlib and library from source), 3 of 25 are identical.
- Cost per call in AOT: **1.9× to 17.5× compiled Go**, depending on the library, median ≈ 4.5×. sort is the exception (×50): its cost is dominated by callbacks into the plugin.
  - When native mode can be measured, AOT does as well: fmt 2.8 vs 2.7 µs, cast 2.7 vs 2.8 µs.
  - It even does better: mapstructure 18 vs 24 µs, validator 11.5 vs 17.9 µs, asn1 17 vs 21 µs. Native mode converts all data at the boundary through reflect.
  - Compared to interpreted mode, when it works, AOT is 1.7× (flag) to 17× (xml) cheaper. sort is the exception (see the costs).
- **json-iterator: interpreted.** reflect2 cannot be converted: it uses `go:linkname` to the internals of `reflect` and `unsafe` arithmetic on the Go memory layout. AOT mode falls back to interpreted mode for it, and interpreted mode now gives the output of `go run` (the VM emulates the linknames of reflect2, `vm/reflect2.go`, with `useUnsafe`). goccy/go-json gets the same diagnosis, but it is not in the active module cache (see below).
- The reflect twin covers the whole `reflect` API (227 exported members, 0 missing). Its gaps are semantic: they are listed below.

## Usage

```bash
export GOFLAGS=-mod=mod GOPROXY=off GOSUMDB=off   # local module cache only
go build -o bin/aotgen ./cmd/aotgen
bin/aotgen [flags] importpath...      # or: scripts/aotgen.sh (the 28 targets of this report)
```

| Flag | Default | Role |
|---|---|---|
| `-o` | `aot` | directory of the AOT copies (`aot/<import path>/`) |
| `-native-out` | `natsyms` | native symbol tables of non-std targets (native mode) |
| `-native` | `true` | also generate these native tables |
| `-report` | `<o>/REPORT.txt` | report per package |
| `-keep` | | packages to keep native even if they use reflect |
| `-nobuild` | `false` | do not compile the copies (the report then has no build column) |
| `-clean` | `true` | empty the output directories first |

Running on the VM side (`cmd/yaegi`):

```bash
go run ./cmd/yaegi -aot prog.go                                    # all registered AOT copies
go run ./cmd/yaegi -mode encoding/json=aot,fmt=native prog.go      # mode per import path
go run ./cmd/yaegi -opaque -mode gopkg.in/yaml.v3=interp prog.go   # interpreted from source
```

`vm.ModeFor` resolves the mode of an import path:

- an explicit mode (`-mode`, `Options.Modes`) wins;
- otherwise, with `-aot`, a registered AOT copy;
- otherwise, native for the stdlib and interpreted for the rest.

An `aot` mode with no registered copy (json-iterator) falls back to this default. This is the base for hybrid modes per plugin and per import path. Their per-plugin API is not exposed yet: `Options.Modes` is per `Machine`.

### Version check of the non-standard copies

A copy is generated from one version of a library: the one in yaegi v2's `go.mod`. A plugin may vendor another version, or a fork. So the import path alone is not enough (`vm/aotsource.go`, since 19 October 2026):

- aotgen records in each registration (`vm.Package`) the `Module` and `Version` the copy comes from, and `Sources`: the hash (`internal/srchash`) of the sources of the packages the copy stands for. These are the packages of its import closure that belong to its own module, and the packages of other modules that aotgen copies too. The hash covers the `.go` files that `go mod vendor` copies (no `_test.go`, no file tagged `ignore`), on every platform, so it is the same in the module cache and in a vendor directory.
- At load (`vm.Load`, with or without the Go toolchain, and `RunMain`/`yaegi`), a package whose mode would be `aot` keeps its copy only when the plugin has the same module, at **exactly** the same version, not replaced in `go.mod`, and the same sources for each package of `Sources`. The version comes from `vendor/modules.txt` (or from go list). Exact match is the safe default: two versions with the same API can still behave differently, and a fork may keep the version line of the original.
- Otherwise the package is interpreted from the plugin's own sources, so the plugin runs its own code. This also applies to an explicit `-mode path=aot`. The reason is in `Machine.AOTFallback`.
- Then every package-level function, variable and type of an AOT package that the interpreted code uses must be in the AOT symbol table, or the load fails with the list of missing symbols. Before, a missing symbol panicked at its first call, during a request (`native func ... not in registry`).
- What a plugin gets in this fallback (measured on 23 October 2026 with `go run ./cmd/aotcheck -modes go,fallback`, and checked by `TestAOTFallback` in `cmd/aotcheck`): the `fallback` mode runs each program of `testdata/aot` in AOT mode with every package of the library's modules interpreted from source, the rest (standard library copies) in AOT mode. The "Interp" column of the table below interprets fmt, errors and sort too, so it predicts worse than this. Results: cast, go-cmp, goccy/go-yaml, json-iterator, sigs.k8s.io/yaml, BurntSushi/toml, yaml.v3, mitchellh/mapstructure and go-viper/mapstructure/v2 match `go run` (the two mapstructure decoded a struct into a map with nil values until the VM reflect boxed the values that `SetMapIndex` puts into a map of interfaces). pelletier/go-toml/v2 matches too since the copy is from v2.4.3 (November 6, 2026): v2.2.4 used `unsafe.Pointer` arithmetic (`internal/danger`) and stopped on a pointer that does not point to a VM value (UNSUP); a plugin that vendors a version older than v2.3 still gets that. One does not: validator needs `golang.org/x/sys/cpu`, which the plugin then also interprets, and which has assembly (UNSUP, a load error, unless `StubAssembly`). A plugin that vendors another version of it should vendor the version of yaegi v2's `go.mod` (see `Machine.AOTFallback` for the reason of a fallback).
- Limits: the other dependencies of a copy stay native, built from yaegi v2's `go.mod`; their version is not compared with the plugin's (as for the standard library). A registration without `Module` (a copy generated before this check) is never used for a non-standard package: regenerate it with `scripts/aotgen.sh`. The standard library copies are checked against the Go version of the host elsewhere (`Package.GoVersion`, `vm/aotversion.go`, see Versions below).

### Copies in the module graph of a host (Traefik)

The check above compares a copy with the plugin. A second risk is on the side of the host. A copy often keeps packages of its own module native (the copy of `github.com/pelletier/go-toml/v2` imports the native `.../unstable`, the copy of `github.com/goccy/go-yaml` imports `.../ast`). When a host links yaegi v2, Go's minimal version selection picks the highest version of each module in the whole graph. If the host requires a newer version of a copied module, these native packages are built at the host's version, and the copy stays at yaegi v2's: the copy can stop compiling, or compile and mix two versions of one library. This happened with Traefik: it requires go-toml v2.4.3, the copy was from v2.2.4, and `aot/all` did not build (`unstable.Unmarshaler` changed). The copy is from v2.4.3 since November 6, 2026.

`cmd/hostgraph` checks this against a snapshot of Traefik's `go.mod` (`testdata/hostgraph/traefik.go.mod.txt`, its commit in `testdata/hostgraph/SOURCE`):

```bash
go run ./cmd/hostgraph            # offline (also run by go test): fails when a copy keeps native packages
                                  # of its module and Traefik would select another version of that module
go run ./cmd/hostgraph -build     # network: builds aot/all in a throwaway module that requires yaegi v2
                                  # (this checkout) and the snapshot, then checks the real selected versions
go run ./cmd/hostgraph -update    # network: refreshes the snapshot from traefik/traefik master (read only)
```

A copy that is complete (no native package of its own module, for example validator) only gets a note when Traefik selects another version: it builds, and a plugin that vendors Traefik's version runs the library interpreted (the version check above). The other modules a copy uses natively (x/text for validator, for example) are only checked by `-build`, which compiles them at Traefik's versions. To fix a failure: `go get module@version` in yaegi v2, then `scripts/aotgen.sh` with each Go release of the window. The nightly CI job "AOT copies in Traefik's module graph" runs `-update` then `-build`, so a change in Traefik's master shows up before a release.

## What the generation does

1. **Loading** (`go/packages`) of the targets and their closure.
2. **Plan** (`plan.go`): each package is put in a category:
   - **aot**: it imports `reflect` or `internal/reflectlite` outside the "native floor" (runtime, sync, os, time, net/http, crypto, math, iter...); it is converted;
   - **copy**: an internal package needed by a copy; it is copied as is, with imports remapped. A native package is copied too when a copy uses its API with types of a copied internal package: the copy has its own version of these types (`encoding/json/jsontext`, used by `encoding/json/v2` with `encoding/json/internal/jsonopts` types, was one; with `encoding/json/v2` it is a target of `scripts/aotgen.sh` now: a program that imports them runs their copies, with a symbol table, see `stdAOTTriggers` in `vm/aot.go`);
   - **native**: the rest, unchanged.

   `internal/godebug` and `internal/stringslite` are linked to the runtime by linkname: they are replaced by hand-written shims (`cmd/aotgen/shims`). `log/internal` has a shim too: in the standard library, package `log` sets its `DefaultOutput` in an init function, but the native `log` never sets the copy, so the default handler of the `log/slog` copy called a nil function. The shim writes with the default `log.Logger` (`log.Output`).
3. **Rewrite** (`rewrite.go`). `reflect` becomes `github.com/traefik/yaegi/v2/vreflect`. Then, with the same semantics on interpreted values:
   - assertions and type switches to non-empty interfaces: `AssertI`, `TSG` with generated wrappers;
   - interface comparisons and `switch`: `EqualAny`, because a VM value cannot be compared by wrapper identity;
   - assertions and type switches to unnamed composites (`x.([]byte)`, `case map[string]any`): `Unbox`, new;
   - `//go:linkname` is neutralized;
   - build constraints keep their platform part (`constraints.go`). go/packages loads the targets for one build (the GOOS, GOARCH and GOEXPERIMENT of the generation), but a copy must have the same files as its source package in every build. The tags of the platform (GOOS, GOARCH and its levels such as `amd64.v3`, `unix`, `cgo`, `goexperiment.*`) stay in the `//go:build` line of the copy; the other tags (`go1.N`, the compiler, custom tags such as `purego`) are evaluated for the generation and removed, as before. The files that another build selects (variants) are copied too when they need no type information (no reflect import, type assertion, type switch, comparison, tag switch or linkname, and only imports of loaded packages): `encoding/binary` has `native_endian_little.go` and `native_endian_big.go`, so `binary.NativeEndian` is big-endian on s390x, ppc64 and mips, as in Go. A variant that cannot be copied makes the copy refuse the builds that select it: `aotgen_guard.go` has their constraint and does not compile there, with a name that says to generate the copy again for that build. With Go 1.27, `encoding/json` is built on `encoding/json/v2` by default (`goexperiment.jsonv2`): the copy is the v2 one (`v2_*.go`, with copies of `jsontext`, `json/v2` and their internal packages), so a build with `GOEXPERIMENT=nojsonv2`, where `encoding/json` is json v1, fails instead of running another json than the host. A package that only some builds have (`encoding/json/v2` exists only with jsonv2) gets the same constraint on its glue and on its import in `aot/go1_NN/all` (`all/constrained*.go`). aotgen stops when the glue names a symbol that a supported build does not declare (`checkGlueSymbols`, checked on the ports of `go tool dist list`). These checks give a value to a tag only while the constraint left still depends on it (`counterexample`): `internal/goexperiment`, copied for `log/slog` with Go 1.27, has 20 experiments and one file per value, and does not need 2^20 builds per port. New.
   - constructs that `go vet` accepts only on `reflect.Value`, or refuses since Go 1.24: `unsafe.Pointer(v.Pointer())` becomes `v.UnsafePointer()`, `unsafe.Pointer(v.UnsafeAddr())` becomes `vm.UnsafeAddrOf(v)`, and a non-constant format with no argument gets `nil...`. New.
   - in `errors`, the recursion of `is`, `as` and `asType` on `Unwrap() []error` starts with `defer vm.NestAOT().Done()`: an error tree with a cycle then stops with `ErrStackOverflow` (MaxNest) instead of a fatal Go stack overflow that kills the host. aotgen fails if it does not find the three places.
   - in every copied file, `os.Exit`, `log.Fatal`, `log.Fatalf`, `log.Fatalln` (calls and function values) become `aotvm.AOTExit`, `AOTFatal`, `AOTFatalf`, `AOTFatalln`, and the reads of `os.Args` become `aotvm.AOTArgs()` (`vm/exit.go`). They use the `os.Exit`, `log.Fatal` and `os.Args` of the machine of the goroutine: for a restricted plugin, `flag.ExitOnError` with a bad flag panics with `os.Exit(2)` like the plugin's own `os.Exit`, instead of ending the host. aotgen fails if a copy assigns `os.Args` or uses another function that ends the process (`(*log.Logger).Fatal*`, `syscall.Exit`). New.
   - in `encoding/json`, the key of the pointer cycle check of `ptrEncoder.encode` (`ptr := v.Interface()`, stored in `ptrSeen`) becomes `aotvm.PtrKey(v)`. The twin's `Interface` of an interpreted pointer is a new box at each call, so the check never found the cycle: a pointer that points back to itself grew the Go stack until gc killed the host. `PtrKey` gives the same key for the same pointer and type, and `json.Marshal` returns the `UnsupportedValueError` of `go run`. aotgen fails if `ptrEncoder.encode` or its key is not found, unless `encoding/json` is built on `encoding/json/v2` (Go 1.27, `GOEXPERIMENT=jsonv2`): then the next rule must apply.
   - in `encoding/json/v2`, the cycle check (`visitPointer`, `leavePointer`) keys on `v.UnsafePointer()`, the identity of the VM pointer, map or slice in the twin. But maps and slices reached it as a new native copy at each level, so a map or a slice that contains itself gave "exceeded max depth" instead of the `UnsupportedValueError` of `go run`. In `marshalValueAny`, `aotvm.Unbox(val)` of the type switch becomes `aotvm.NoUnbox(val)` (an interpreted `map[string]any` or `[]any` takes the reflect path), and in `marshalValueAny` and `makeInterfaceArshaler` the copy `v := newAddressableValue(T); v.Set(X)` becomes `v := addressableValue{aotvm.AddrCopy(X), true}`, which keeps the VM map or slice. aotgen fails if one of these places is not found, or if `encoding/json` uses `encoding/json/v2` and it is not an AOT copy.
4. **Glue** (`glue.go`, `zz_aotglue.go` per package):
   - symbol table (`vm.RegisterAOT`), with the module version and the source hashes of a non-standard copy (see the version check above);
   - native hooks per exported function and method (`vm.RegisterNative`, direct call without `reflect.Call`);
   - interface wrappers (`vm.RegisterWrapper`);
   - case tables of the type switches.
   - hooks of the generic functions (`vm.RegisterGeneric`, `zz_aotgeneric.go`). A copy has only the instances of its generic functions that its own code needs, and the program calls them with its own types (`json.MarshalFunc[celsius]`). The hook of a generic function is written by hand in `cmd/aotgen/generics/<path>/generic.go.in`: the function with its type arguments as twin types (`vm.GenericCall.Types`) and the function values of the program called through the VM (`GenericCall.Call`). aotgen copies the file into the copy (`AOTPATH` becomes the import path of the copies). Go 1.27: the generic functions of `encoding/json/v2` (`GetOption`, `MarshalFunc`, `MarshalToFunc`, `UnmarshalFunc`, `UnmarshalFromFunc`) and of `encoding/json/jsontext` (`AppendQuote`, `AppendUnquote`, `AppendFormat`). `TestAOTGenericHooks` fails when an exported generic function of a standard copy has no hook. New.

   `aot/go1_NN/all` imports everything; `aot/all` imports the `all` of the Go release of the build (`aot/all/go1_NN.go`, one per release of the window); `natsyms/all` does the same for the native tables.
5. **Build** of each copy. A copy that does not compile is moved to `aot/go1_NN/_failed/` (ignored by git) and its error goes into the report.
6. **Report** (`aot/go1_NN/REPORT.txt`), per package:
   - reflect API used (members × occurrences);
   - members missing or partial in the twin;
   - rewrites;
   - hooks without a direct path;
   - uses of `unsafe` and `go` statements;
   - blockers.

## Results per library [M]

Programs: `testdata/aot/*.go`. Each one is a plugin whose types have the methods that the library calls ("exercised" column). The criterion is an output **identical** to `go run`, stdout and stderr combined. `rawmsgconc.go` is not a library row: it is the regression program of the AOT shadows under concurrency (encoding/json, `vm/twin_value.go`, `shadowRef`). `gobcallback.go` is not one either: it is the regression program of a shadow that a callback of the program suspends and resumes in the middle of a decode (encoding/gob calls GobDecode for an element of a `[]any`). `pprofhandles.go` is not one either: it is the regression program of the opaque handles of equal values, which native code compares (runtime/pprof's Profile.Remove). `chanstored.go` is not one either: it is the regression program of an interpreted channel that AOT code stores in native memory with reflect (`Value.Set`, `Value.SetMapIndex`: the channel gets its stand-in, `natStored`).

Modes:

- **native**: the library is called natively, with the reflect wall;
- **interp**: the library, and fmt, errors, sort, strconv..., interpreted from source, with the VM's internal reflect;
- **AOT**: `yaegi -aot`.

"UNSUP" means that the VM refuses a construct; "DIFF" means that the output differs.

| Library | Program, exercised | Generation | Build | Native | Interp | AOT | Recommended mode |
|---|---|---|---|---|---|---|---|
| encoding/json | json: MarshalJSON, UnmarshalJSON, TextMarshaler, String | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| fmt | fmt: String, Error, Format, GoString | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| errors | errors: Is, As, Unwrap, Timeout | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| sort | sort: Len, Less, Swap | ok | ok | DIFF | MATCH | **MATCH** | AOT (interp is cheaper, see costs) |
| encoding/xml | xml: MarshalXML, UnmarshalXML, TextMarshaler | ok | ok | DIFF | MATCH | **MATCH** | AOT |
| encoding/gob | gob: GobEncode, GobDecode, registered interface | ok | ok | UNSUP | UNSUP | **MATCH** | AOT |
| encoding/binary | binary: plugin structs | ok | ok | UNSUP | UNSUP | **MATCH** | AOT |
| encoding/asn1 | asn1: plugin structs and tags | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| text/template | template: methods called from the template | ok | ok | UNSUP | UNSUP | **MATCH** | AOT |
| html/template | htmltemplate: methods, escaping | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| flag | flag: flag.Value, TextMarshaler | ok | ok | DIFF | MATCH | **MATCH** | AOT |
| net/rpc | rpc: plugin service (exported methods) | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| log/slog | slog: Handler, LogValuer, MarshalJSON | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| database/sql | sql: full plugin driver | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| gopkg.in/yaml.v3 | yamlv3: MarshalYAML, UnmarshalYAML | ok | ok | DIFF | DIFF | **MATCH** | AOT |
| sigs.k8s.io/yaml | k8syaml: MarshalJSON, UnmarshalJSON (through json) | ok | ok | DIFF | DIFF | **MATCH** | AOT |
| github.com/goccy/go-yaml | goccyyaml: MarshalYAML, UnmarshalYAML | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| github.com/BurntSushi/toml | toml: TextMarshaler | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| github.com/pelletier/go-toml/v2 | gotoml: TextMarshaler | ok | ok | DIFF | MATCH (v2.4.3; DIFF with v2.2.4) | **MATCH** | AOT |
| github.com/mitchellh/mapstructure | mapstructure: hooks `func(reflect.Type, reflect.Type, any)`, squash, remain | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| github.com/go-viper/mapstructure/v2 | mapstructure2: TextUnmarshaler, hooks | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| github.com/go-playground/validator/v10 | validator: custom validations (`FieldLevel`), struct-level | ok | ok | DIFF | UNSUP | **MATCH** | AOT |
| github.com/spf13/cast | cast: String, Error, named types | ok | ok | DIFF | DIFF | **MATCH** | AOT |
| github.com/google/go-cmp (cmp, cmpopts) | gocmp: Equal, Reporter, Comparer, Transformer, AllowUnexported | ok | ok | UNSUP | UNSUP | **MATCH** | AOT |
| github.com/json-iterator/go | jsoniter: MarshalJSON | ok (copy) | **FAIL** (reflect2) | DIFF (only `MarshalJSON` ignored) | MATCH (`useUnsafe`) | **MATCH** (falls back to interp) | interp (`useUnsafe`) |
| github.com/goccy/go-json | n/a | not evaluated: missing from the active cache | n/a | n/a | n/a | n/a | native (static diagnosis below) |

Why native mode fails everywhere: this is the reflect wall. Native code does not see the methods of interpreted types: it sees the underlying type, `vm.stringerW`, a wrong `%T`. It also cannot build a recursive type or an interpreted interface (`reflect cannot build recursive type`). The programs are chosen on purpose to cross this wall.

Why interpreted mode fails: the VM's *internal* reflect (`vm/vmreflect.go`) implements only the ~83 entry points that encoding/json, fmt, errors and sort use. This produces UNSUP results such as `reflect.Indirect`, `Value.Convert`, `Type.MethodByName`, `Value.Slice`. On top of that come native globals and types that are missing from the yaegi tables (`sync/atomic.Pointer`, `log/internal.DefaultOutput`, `internal/godebug.New`) and `unsafe.Pointer` to `uintptr` (fmt `%p`, `golang.org/x/sys/cpu`). These are limits of the interpreted mode, not of the twin.

## Cost per call [M]

**Method** (`go run ./cmd/aotcheck -bench`):

- each program has a `benchOp` (a typical plugin operation: marshal + unmarshal, a validation decision, a template render...), run `n` times in a loop in one process (`AOT_BENCH=n`), after a first call that is not timed;
- `n` is calibrated for a run of about 200 ms;
- 5 measured runs, the minimum is kept;
- 3 full passes, minimum of the passes. Raw data: `docs/aot/aotcheck-bench.tsv`.

**Stability:**

- the machine is shared, with another active session (load average 5 to 16);
- between passes 1 and 2, 54 pairs out of 63 agree within ±5 %;
- the other 9, with an intra-pass spread of 50 to 120 % in pass 2, were measured again in pass 3, which agrees within ±5 % with pass 1.

**Conditions:** Apple M3 Pro, Go 1.26.8, same `yaegi` binary for native, interp and AOT.

The "go" column is `go run`: the whole program compiled, the absolute floor. The "DIFF" numbers measure a run whose output is wrong: they are only indicative. "n/a": cannot be measured (the `benchOp` panics or the VM refuses).

| Program | go (compiled) | native (VM) | interp (VM) | AOT (VM) | AOT / go |
|---|---|---|---|---|---|
| asn1 | 2.9 µs | DIFF 21.4 µs | UNSUP | MATCH 17.2 µs | ×5.9 |
| binary | 335 ns | UNSUP | UNSUP | MATCH 5.2 µs | ×15.5 |
| cast | 255 ns | DIFF 2.8 µs | DIFF 6.0 µs | MATCH 2.7 µs | ×10.5 |
| errors | 72 ns | DIFF n/a | UNSUP | MATCH 1.3 µs | ×17.5 |
| flag | 423 ns | DIFF n/a | MATCH 5.9 µs | MATCH 3.5 µs | ×8.4 |
| fmt | 277 ns | DIFF 2.7 µs | UNSUP | MATCH 2.8 µs | ×10.3 |
| gob | 14.6 µs | UNSUP | UNSUP | MATCH 64.9 µs | ×4.4 |
| goccyyaml | 38.2 µs | DIFF n/a | UNSUP | MATCH 93.2 µs | ×2.4 |
| gocmp | 19.2 µs | UNSUP | UNSUP | MATCH 74.0 µs | ×3.9 |
| gotoml | 4.2 µs | DIFF n/a | DIFF n/a | MATCH 15.1 µs | ×3.6 |
| htmltemplate | 3.4 µs | DIFF 10.2 µs | UNSUP | MATCH 12.8 µs | ×3.8 |
| json | 5.3 µs | DIFF n/a | UNSUP | MATCH 20.3 µs | ×3.8 |
| jsoniter | 763 ns | DIFF 10.1 µs | MATCH 41.6 µs | MATCH 32.5 µs (interp) | ×43 |
| k8syaml | 33.0 µs | DIFF n/a | DIFF n/a | MATCH 61.4 µs | ×1.9 |
| mapstructure | 4.5 µs | DIFF 23.9 µs | UNSUP | MATCH 18.1 µs | ×4.1 |
| mapstructure2 | 6.6 µs | DIFF 30.2 µs | UNSUP | MATCH 22.2 µs | ×3.4 |
| rpc | 4.3 µs | DIFF n/a | UNSUP | MATCH 8.4 µs | ×1.9 |
| slog | 578 ns | DIFF 3.4 µs | UNSUP | MATCH 3.6 µs | ×6.3 |
| sort | 2.4 µs | DIFF 123.5 µs | MATCH 98.3 µs | MATCH 116.6 µs | ×49.6 |
| sql | 643 ns | DIFF n/a | UNSUP | MATCH 11.3 µs | ×17.5 |
| template | 1.6 µs | UNSUP | UNSUP | MATCH 10.1 µs | ×6.3 |
| toml | 33.2 µs | DIFF n/a | UNSUP | MATCH 82.0 µs | ×2.5 |
| validator | 1.6 µs | DIFF 17.9 µs | UNSUP | MATCH 11.5 µs | ×7.4 |
| xml | 8.1 µs | DIFF n/a | MATCH 634.0 µs | MATCH 37.5 µs | ×4.6 |
| yamlv3 | 18.5 µs | DIFF n/a | DIFF n/a | MATCH 45.2 µs | ×2.4 |

How to read it:

- **The AOT/go ratios mix two costs**: the cost of the interpreted plugin that builds the data (`benchOp` is plugin code) and the cost of the library.
  - When the library dominates (yaml, toml, k8syaml, rpc, gob, cmp), AOT is at 2 to 4× compiled code.
  - When the operation is tiny (errors 72 ns, cast, sql, binary), what dominates is the interpretation of the plugin and the boundary crossing, hence ×10 to 17.
- **Native vs AOT, when native mode can be measured**: equal on fmt and cast; AOT is cheaper on asn1, mapstructure, mapstructure2 and validator. In native mode, each call recursively converts VM values to native values and back (a copy). In AOT, the library reads the VM cells directly through the twin.
- **sort is the exception**: a `sort.Sort` on a plugin `sort.Interface` makes one AOT → VM round trip (`vmObj.call`) per `Less`/`Swap`. This costs more than staying in the VM (interp 98 µs, AOT 117 µs). For libraries dominated by callbacks into the plugin, interpreted mode can win [M]. A direct trampoline for these callbacks (with no `reflect.Value` for the arguments) would probably bring AOT below interpreted mode [E].
- Startup: the `init` of the 47 copies and of the native tables costs ≈ 0.8 ms (`GODEBUG=inittrace=1`); the `yaegi` binary is 44 MB [M].

## json-iterator, reflect2 and goccy/go-json: diagnosis

**modern-go/reflect2** (json-iterator depends on it):

- 21 files, 203 uses of `unsafe`, 12 `go:linkname` to the internals of `reflect`: `unsafe_New`, `typedmemmove`, `unsafe_NewArray`, `typedslicecopy`, `mapassign`, `mapaccess`, `mapiterinit`, `mapiternext`, `ifaceE2I`, `makemap`, `resolveTypeOff`, `typelinks`. [M]
- The copy does not compile: these functions without a body no longer have a target (`missing function body`). This is the symptom; the real cause is elsewhere. [M]
- reflect2 reads the type word of an interface (`unpackEFace`) to get the runtime `*rtype`. For a plugin value, this is the type of the opaque wrapper, not the interpreted type: interpreted types have no `*rtype`. [M]
- All the mechanics of json-iterator are address arithmetic on the Go memory layout: 48 files, 213 uses of `unsafe`. The encoders and decoders take an `unsafe.Pointer` to the value and read the fields at `StructField.Offset` (`UnsafeGet`, `UnsafeSet`). [M]
- An interpreted value is a `vm.Value{N int64; R any}`, and a struct is a `[]Value`: there is no Go memory with this layout. **No rewrite of reflect can make this correct.** The VM would need to store plugin values in the native layout (`reflect.StructOf`...), and this loses the methods: we fall back to native mode. [O]
- Tried or considered:
  - keep reflect2 native (`-keep`) under an AOT copy of jsoniter: it does not compile, because jsoniter passes `vreflect.Type` values where reflect2 expects `reflect.Type`, and the codecs would still work on VM memory;
  - emulate the linknames in the twin: useless as long as the codecs read raw memory.

  The tool detects and reports it (`BLOCKER: bodyless functions`).

**Interpreted mode works** (with `useUnsafe`): the VM follows the address arithmetic of the codecs on its cells (docs/UNSAFE.md), and `vm/reflect2.go` emulates the linknames of reflect2 on interpreted values (`mapiterinit`, `mapiternext`, `unsafe_New`, `unsafe_NewArray`, `typedmemmove`, `typedslicecopy`, `makemap`, `mapassign`, `mapaccess`, `ifaceE2I`). The output is the output of `go run`, `MarshalJSON` included, at 42 µs vs 0.8 µs compiled [M] (`testdata/jsoniter.go`, `testdata/aot/jsoniter.go`). Limits: the key and value pointers of a map iteration point to copies (a write through them does not reach the map), and a native `reflect.Type` is refused. Native mode is cheaper (10 µs) but ignores the plugin's methods (`"level":2` instead of `"L2"`), and the same goes for `UnmarshalJSON` and `TextMarshaler`. encoding/json in AOT stays the cheapest choice for a plugin [O].

**goccy/go-json**: missing from the active module cache. Only an extracted copy of v0.10.2 exists in `~/dev/go/pkg/mod`; its `.zip` is a broken link to `~/go/pkg/mod/cache`, which has only the `.mod`/`.info` files of v0.10.5. So it cannot be added to `go.mod` without a download. Static diagnosis of v0.10.2 [M]:

- 73 files, 47 of them use `unsafe`, 435 uses;
- 47 `go:linkname` to `reflect.(*rtype).*`, `reflect.typedmemmove`, `runtime.mapassign_faststr`, `runtime.mapiterinit`, etc.;
- its own `runtime.Type` modeled on `reflect.rtype`, and codecs with raw pointers.

Same family as reflect2, more deeply tied to the runtime: **not convertible, native fallback** [E].

## Gaps of the reflect twin (deliverable)

**API coverage** [M]: the 227 exported members of `reflect` (Go 1.26: functions, methods of `Value`, `Type`, `MapIter`, fields of `StructField` and `Method`) all exist in `vreflect`. Comparison script: `go/types` on `reflect` and on `github.com/traefik/yaegi/v2/vreflect`. So the gaps are **semantic**.

**Not supported on interpreted values** (`unsupported(...)`, the program stops with code 3):

| Member | Limit |
|---|---|
| `Value.Complex`, `Value.SetComplex` | the VM has no complex numbers |
| `NewAt` | native types only, except the unexported field idiom below |
| `SliceAt` | native types only |
| `StructOf` | fields of native types only |
| `Value.SetPointer` | native values only |
| `Value.InterfaceData` | not supported (deprecated in reflect) |
| `MapOf`, `FuncOf` with a native type that has no equivalent in the program | "native type has no interpreted counterpart" (native unnamed struct, native non-empty interface) |
| conversion of a VM value to a native type that is not handled | `vm: converting %T to native %s` (for example, interpreted channels to native) |

**Partial** (semantics differ from reflect):

- `Value.Bytes` on an interpreted `[]byte` stored as `[]Value`: a copy, with no aliasing (`SetBytes` and writes into the returned slice are not visible on the VM side). With the `bytes` optimization (the default), `[]byte` values that come from native code stay shared.
- `Value.UnsafePointer`, `Value.Pointer`, `Value.UnsafeAddr` on a VM value: identity only. It is the address of the VM cell, not of memory with the Go layout.
  - Identity comparisons, cache keys and cycle detection (go-cmp, json) work.
  - Reading raw memory does not: arbitrary arithmetic, `(*T)(ptr)`.
- `NewAt(f.Type, unsafe.Pointer(v.UnsafeAddr()+f.Offset))`, the unexported field idiom (go-cmp `AllowUnexported`, validator), is **emulated**, new: `UnsafeAddr` remembers the last 64 VM cells, and `NewAt` finds the cell itself (offset 0, the form validator uses) or the struct field by its offset. Any other arithmetic on these addresses reads arbitrary memory.
- `Type.Size`, `Align`, `FieldAlign`, `StructField.Offset` of interpreted types: gc sizes computed by `types.SizesFor`, consistent but with no real memory behind them.
- `Value.Interface()` of an interpreted value: a "boxed" value.
  - Predeclared types: native.
  - Pointers to a native type: a native pointer that aliases the cell (shadow, synced again at each return into the VM). A shadow belongs to the goroutine that made it: only the returns into the VM of that goroutine give it back, the cell is not changed while AOT code uses the shadow, and it is written back only if the value changed, so concurrent requests and read only calls on a shared config are safe (`vm/twin_value.go`, `shadowRef`).
  - Types with methods: a wrapper that implements the interfaces that the AOT code asserts.
  - The rest: an opaque handle.
  - Pointers to interpreted data with no native counterpart: a new box at each call, so two calls with the same pointer do not give equal values (a native pointer gives the same one). AOT code that keys a map with it must use `vm.PtrKey` instead (json, see the rewrites above).
  - An AOT assertion to an interface for which aotgen did not generate a wrapper panics (`no AOT wrapper for interface`).
  - An assertion to an unnamed composite (`x.([]byte)`, `case map[string]any`) gives a **copy** (`Unbox`). For a map or a slice, the copy is written back into the plugin value when the AOT call returns or calls back into interpreted code (`binary.Read(r, order, s)` fills `s`). Writes after a call back into interpreted code are lost.
- `Type.String()` of a native type that mentions the twin: `*vm.vtype`, `vm.RV`, `vm.StructField`... are rewritten to `reflect.Type`, `reflect.Value`... New. However, `%T` of a plugin `reflect.Type` gives `reflect.Type`, not `*reflect.rtype`.
- `Type.PkgPath()` of a program's main package: `main`, like reflect. go/packages says `command-line-arguments`.
- `Value.Method(i).Func` on an interpreted type: a VM closure, callable with `Call`. The `Method.Type` includes the receiver, like reflect.
- Boundary between the plugin reflect and the AOT reflect: new, see below. A native `reflect.Value` (not backed by the VM) that goes from AOT to the plugin arrives there **copied** (VM encoding `encV` on a new cell), with no aliasing. Values backed by the VM, the common case for hooks, share their cell.

## Limits

- **unsafe**: everything that assumes the Go memory layout fails on interpreted values. This is the case for reflect2, json-iterator, go-json, and in general for `(*T)(unsafe.Pointer(...))` on plugin data. The report counts the uses of `unsafe` per package (`risks:`): this is the first sorting criterion for a new library.
- **go:linkname**: neutralized in the copies. A function without a body becomes a blocker (reflect2). The two stdlib packages concerned have hand-written shims:
  - `internal/godebug` reads `$GODEBUG` once, with no metrics and no update through `os.Setenv`;
  - `internal/stringslite`.
- **init and global state**: the AOT copies are host code. Their `init` runs once when the host starts (≈ 0.8 ms for the 47 copies and the native tables), and **their globals are shared by all machines and all plugins**. In practice:
  - the `gob.Register` registry is shared;
  - the AOT `flag.CommandLine` is shared by the plugins that run flag in AOT mode (it is not the host's: in native mode, each restricted plugin has its own, `vm/exit.go`);
  - `slog.Default` too;
  - the per-type caches are keyed by `*vtype`, and interpreted types belong to one machine. Since task 5 (October 17, 2026), aotgen replaces package-level `sync.Map` values with `vm.TypeCache` (`vm/typecache.go`): the entries of interpreted types are stored in their descriptor and go away with the machine. This is the case for `encoderCache` and `fieldCache` (json), `tinfoMap` (xml), `structSize` (binary), `userTypeCache` and `concreteTypeToName` (gob). `TestAOTUnload` loads and drops a plugin that goes through json 20 times: no machine survives and the heap stays stable. `TestAOTStatePerPlugin` checks that two plugins with the same import path and the same type names do not see each other [E].
  - Ordinary package-level maps keyed by `reflect.Type` become `vm.TypeMap` (phase 2 bis), when all their uses are a read, a read with `ok`, a write, `delete`, a nil test or an initialization: `structMap` of yaml.v3 and of go.yaml.in/yaml/v2, the `fieldCache` of toml and sigs.k8s.io/yaml, the marshaler registries of goccy/go-yaml, `types` of gob [E].
  - gob remains: its `typeInfoMap` is a map copied on each write and stored in an `atomic.Value`, and its type identifiers are global. A plugin that goes through gob keeps its machine after it is dropped. The name registry of `gob.Register` is shared: two plugins that register the same name with different types get in each other's way [E]. For Traefik, the stakes are low: plugins are loaded once at startup (`pkg/plugins/builder.go`, `NewBuilder`) and the dynamic configuration reuses the interpreter; a machine is dropped only at shutdown.
- **Goroutines**: the copies that start goroutines (database/sql, net/rpc) call back into the VM from other Go goroutines: each callback creates an execution context. No problem observed, but no load test [M]/[E].
- **AOT → VM callbacks**: cost of one `reflect.Value` per argument (`vmObj.call` wrappers); see sort.
- **Versions**: the stdlib copies are the ones from the GOROOT used for generation (Go 1.27.4; the CI job "generated data is up to date" regenerates them with the `GO_VERSION` of the workflow and fails on any difference). An AOT copy of encoding/json follows this version, not the one the host is compiled with. They must be generated again at each Go upgrade, patch releases included, and for another GOEXPERIMENT that changes the files of a copied package (`jsonv2`: `aotgen_guard.go` makes the build fail until then). Two checks enforce the Go version:
  - **Build time**: `aot/all/release_guard.go` is only built by a Go release outside the window of the copies (`//go:build !go1.26 || go1.28`), and does not compile, with a message that says to regenerate (`sh scripts/aotgen.sh`). A host that imports `aot/all` and moves to Go 1.28 fails to build. The copies of each release of the window are in `aot/go1_NN/` (each file has the constraint of its release, `go1.27 && !go1.28`), and `aot/all/go1_NN.go` imports those of the release of the build. (`aot/all` used to panic in its `init`, before the `main` function of the host, even when the host did not use AOT mode.) Verified by hand with a host built with go1.27.2 and the copies of Go 1.26: the build stops on the guards of `stdtypes`, `stdsyms` and `aot/all`.
  - **Load time**: each glue file records the Go version of the copy (`Package.GoVersion`, for example `go1.27.4`). In AOT mode (`Options.AOT` or a `ModeAOT` path), `vm.Load` and `vm.RunMain` return an error when the copies are of another release (also for a host that imports single `aot/...` packages without `aot/all`), or when the host is built with a newer patch release than the copies whose sources of a copied package differ (or are not known): the manifest `aot/all/gosources.txt` (`internal/gosources`, written by aotgen and `cmd/aotgosrc -update`) has the hash of the GOROOT sources of each copied package for each Go patch release, and a newer patch release that changed none of them keeps the copies. Otherwise the host has security fixes (html/template, encoding/json, net/rpc...) that the copies miss, and govulncheck does not see the copies. `Options.AllowStaleAOT` (`yaegi -allow-stale-aot`) accepts the older copies: an explicit choice of availability over these fixes. `Options.StaleAOTFallback` (`yaegi -stale-aot-fallback`) loads the plugin in default mode instead of failing: no copy runs, so no fix is missed, but the plugin loses the behavior of AOT mode for this load (`Plugin.Notes` says so). `scripts/gopatch-diff.sh` lists the copied packages that a Go patch release changed (4 of the 8 patch releases from go1.26.5 to go1.27.3 changed none). `internal/pluginhost` turns the fallback on. Copies made with a newer patch release than the host are accepted. The policy is in `vm/aotversion.go`. The CI job "next Go" regenerates them with the latest Go release. golang.org/x/tools must be able to read the export data of that release (Go 1.27 needs v0.50.0 or later); otherwise aotgen stops with a load error on the twin.
- **Size**: 4.6 MB of generated sources for 47 packages, `yaegi` binary of 44 MB.
- **godebug**: the `GODEBUG` settings read by the copies (html/template: `jstmpllitinterp`, `htmlmetacontenturlescape`) only see the startup value, through the shim.

## VM side

The VM files that support the AOT copies:

- `vm/twin_bridge.go` (new): a bridge between two implementations of reflect.
  - On one side, the plugin reflect, as VM intrinsics: `reflect.Type` = interface `typeRT`+`*vtype`, `reflect.Value` = VM struct with 3 fields.
  - On the other side, the AOT twin: native `RT`, `RV`.
  - Functions: `twinFromVM`, `twinToVM`, `twinIfaceVM`, `twinNamed`, `reflectNamed`, `vmVT`.
  - Also: the `UnsafeAddr`/`NewAt` emulation of unexported fields (`rememberUnsafe`, `unsafeField`), and `natString`, which makes the twin types look like the reflect types.
- `vm/twin_value.go`:
  - calls to the bridge in `argNat`, `retVM`, `ifaceVM`, `natToVM`;
  - `UnsafeAddrOf` (new);
  - `UnsafeAddr` remembers the cell.
  - Taken from the interrupted session: `ifaceVM` keeps native the unnamed maps that come from native code, and `shadowRef.m`.
- `vm/twin_type.go`:
  - `cpart` maps `reflect.Type`/`reflect.Value` to the twin types;
  - `goTypeOf1` does the reverse;
  - `String()` of native types goes through `natString`.
- `vm/twin_func.go`: `NewAt` checks `unsafeField` first.
- `vm/native.go`:
  - bridge in `toNativeW` and `fromNative`;
  - `reflectType` maps `reflect.Type`/`Value` to the twin types when the VM reflect is active;
  - `box` makes the `*vtype` canonical (`cn()`) for a plugin `reflect.Type` and converts a plugin `reflect.Value` to `RV`.
- `vm/aot.go`: `Unbox` (new).
- `vm/compile.go`: `deepCopy` of a shadowed cell (interrupted session).

Conformance: none of these changes acts without `-opaque`/`-aot`. The bridge requires `m.vr != nil`.

## Non-regression [M]

| Measure | Before (aotgen not committed, 2026-10-16) | After |
|---|---|---|
| master `Output:` | 816 / 868 | 816 / 868 |
| global pass | 891 / 1013 | 891 / 1013 |
| programs that passed and now fail | n/a | 0 |

`go vet ./...` and `go test ./...` pass. `go test` includes `TestAOT` (`cmd/aotcheck`): the 24 AOT programs must give an output identical to `go run`; jsoniter is marked as a known failure, and the test fails if it starts to pass.

## Reproduce

```bash
export GOFLAGS=-mod=mod GOPROXY=off GOSUMDB=off GOCACHE=$TMPDIR/gocache
# if the module cache is not the GOPATH one: export GOMODCACHE=~/go/pkg/mod

sh scripts/aotgen.sh                         # regenerates aot/go1_NN/, natsyms/, aot/go1_NN/REPORT.txt for the release of the go command
go build ./... && go vet ./...
go test ./...                                # including TestAOT
go run ./cmd/aotcheck -v                     # MATCH/DIFF table of the 5 modes (go, native, interp, aot, fallback)
go run ./cmd/aotcheck -bench -reps 5 -modes go,native,interp,aot -tsv $TMPDIR/bench.tsv
go run ./cmd/aotcheck -run 'gocmp|validator' -modes go,aot -v   # a subset
go run ./cmd/runtests -j 8 -timeout 20s -out $TMPDIR/conf.tsv -summary $TMPDIR/conf.txt
```

Add a library:

1. have it in the module cache and in `go.mod`;
2. add its target to `scripts/aotgen.sh`;
3. write `testdata/aot/<name>.go`: a `package main` with `//aot:lib <import path>`, plugin types with the methods that the library calls, a `benchOp` and the `benchMain` of the other programs;
4. read its block in `aot/go1_NN/REPORT.txt`, starting with `risks:` and `BLOCKER:`.

## To do

The open AOT items (shared `gob.Register` and `flag.CommandLine`, AOT to VM callbacks without `reflect.Value`, one reflect for the plugin and the twin, aliasing of unnamed composites) are in `TODO.md`; the policy for Go patch releases (`vm.CheckAOT`, the manifest of the Go sources, `StaleAOTFallback`) is in the README. Also: evaluate goccy/go-json (static diagnosis: not convertible), and check the `internal/godebug`, `internal/stringslite` and `log/internal` shims at each Go release.
