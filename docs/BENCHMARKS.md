<!-- These measurements were made in October 2026 on an early version of the VM, in another tree
(`.triage/spikes/bench-c/`, module `benchc`). In this repository the host and the script are in
compare/bench/ (module compare/go.mod, compare/bench/run.sh), the plugins in plugins/ and the VM
in vm/. The raw results (results/final/... below) are not kept in the repository: run.sh writes
them to compare/bench/results/. yaegi comes from the module version in compare/go.mod (the
measurements used yaegi master at the time, d690a67f). The "vjson/" copy of encoding/json used here was later replaced by the
AOT copies of cmd/aotgen (aot/encoding/json). -->

# Benchmarks: per-request cost of plugins

This page explains, with numbers, why yaegi v2 calls the standard library
natively or through AOT copies, and does not interpret it from source.
The README has the current summary table; this page has the method and the
details.

Legend: **[M]** measured, **[E]** estimated, **[O]** opinion.

## Summary

**Interpreting the standard library from source is too slow.** [M]+[E]

- On W2 (JSON), configurations 4 (vm-src) and 5 (vm-mixed) give the correct output (MATCH, `MarshalJSON` is respected). yaegi does not.
- But they cost 530 µs and 637 µs per request, against 22 µs for yaegi: **24× and 29× yaegi**.
- On the first, unoptimized version, it was 686 µs and 800 µs. Six optimizations gained only 1.3×.
- Estimated ceiling of a well-built bytecode VM that interprets encoding/json: **4 to 8× yaegi** [E]. So this cost does not come from a weak implementation. Interpreting json costs about 40× the native code, and no `switch` VM in Go goes down to 2×.

**The VM + the reflect twin + reflect libraries compiled ahead of time (AOT) against the VM value model is fast enough.** This is the design yaegi v2 uses (the "AOT" mode of the README; called C′ below). [M]

| Workload | Measurement |
|---|---|
| W2 JSON, correct output | 25.2 µs against 22.3 µs for yaegi (wrong output), so **1.13×** (CPU 0.90×) |
| W1 headers | 5.5 µs against 12.1 µs for yaegi, so **0.45×** |
| Loading | cold 85–92 ms (at that time including ~80 ms of `go list`); with cache 0.3–0.7 ms |

- encoding/json is generated mechanically (`reflect.X` replaced by `vm.X`) and compiled into the host. It walks the interpreted values through the VM reflect, in native Go code.
- At the time of this measure only json was converted to AOT. fmt stayed native, so it still hit the reflect wall for `%T`, `%d` on a named type, and `%v` on an interpreted struct. Today `cmd/aotgen` makes AOT copies of 24 libraries, fmt included (`docs/AOTGEN.md`).

## Current numbers (October 23, 2026) [M]

The tables further down are the measurements of the first version. On the
branch of TODO.md section 5 (fewer allocations per operation, more
generated trampolines, callbacks without `reflect.MakeFunc`), against main
at f45579f, both built and run in turn on the same machine:

- Apple M3 Pro (12 cores), Go 1.26.8, `GOMAXPROCS=4`. The machine was
  **shared with other agents** (load average 7 to 26 during the run), so
  the times are less reliable than the allocations; the native and yaegi
  columns, which did not change, moved by less than 1 %.
- `compare/bench -cfg native,yaegi,vm-native,vm-aot -count 3 -benchtime 300ms`,
  5 rounds per binary, base and new in turn: median of 15 runs.

| Workload | config | wall µs (main → branch) | CPU µs | allocs/op | B/op |
|---|---|---|---|---|---|
| W1 headers | native | 2.12 → 2.11 | 3.7 | 31 | 2 712 |
| W1 headers | yaegi | 12.3 → 12.2 | 17.4 | 255 | 8 933 |
| W1 headers | vm-native | 5.11 → **4.67** (-8.6 %) | 7.7 → 6.7 | 88 → **75** | 4 481 → 3 616 |
| W1 headers | vm-aot | 5.16 → **4.69** (-9.2 %) | 7.8 → 6.7 | 88 → **75** | 4 483 → 3 617 |
| W2 json | native | 12.9 | 17.8 | 104 | 8 370 |
| W2 json | yaegi (DIFF) | 23.1 | 32.7 | 337 | 16 339 |
| W2 json | vm-native | 31.1 → 31.1 | 42.4 → 42.6 | 375 → **367** | 19 973 → 19 469 |
| W2 json | vm-aot | 30.7 → **29.9** (-2.5 %) | 41.7 → 41.1 | 370 → **362** | 19 792 → 19 298 |
| W3 regexp | native | 1.23 | 2.3 | 22 | 2 064 |
| W3 regexp | yaegi | 6.15 → 6.11 | 9.8 | 149 | 6 088 |
| W3 regexp | vm-native | 3.22 → **3.15** (-2.1 %) | 5.3 → 5.2 | 70 → **65** | 3 511 → 3 391 |
| W3 regexp | vm-aot | 3.21 → **3.16** (-1.7 %) | 5.3 → 5.2 | 70 → **65** | 3 511 → 3 391 |

W2 is now correct in both modes (vm-native goes to the AOT copy of json
for interpreted types, `vm/aotroute.go`). The focused benchmark of each
change is `go test ./vm -run '^$' -bench BenchmarkPerfOps -benchmem`
(`vm/perfops_test.go`), with numbers in the commit messages. The bench
binary is 1.0 MB larger (about 1 000 generated trampolines instead of
500). Loading traefikoidc, the largest catalog plugin (`cmd/loadtime`,
warm, median of 3): 226 ms in VM mode (parse and type check 109 ms, SSA
41 ms, load-time check 52 ms), 194 ms in AOT mode.

## Machine and conditions [M]

- Apple M3 Pro (6 P cores + 6 E cores), 18 GB, macOS 25.6 (Darwin arm64), Go 1.26.8, yaegi `d690a67f`.
- The machine is shared with other agents. Load during the final run: load average 3.8 to 8.4. During development, it went up to 99.
- To reduce noise:
  - all configurations of one W run in the same process, measured in turn;
  - each configuration does 10 repetitions of about 300 ms, after a warm-up of 20 requests and a calibration;
  - we report the median, the min and the max.
- Two time measurements:
  - **wall** (latency);
  - **CPU** (user+sys of the process divided by the number of requests). CPU includes the GC that runs on other cores: this is the real cost in throughput.

## What was built

Everything is under `.triage/spikes/bench-c/`. Nothing was changed in `spikes/vm/` (copied into `bench-c/vm/`), nothing in the tracked files, nothing on GitHub.

**Plugins written like real plugins** (`CreateConfig` + `New(ctx, next, *Config, name)`), in `plugins/`:
- W1 `w1headers`: Get/Set/Del of headers, `range` over `req.Header`, `strings.ToLower/HasPrefix/Contains`, `strconv.Itoa`, `http.Error`.
- W2 `w2json`: decodes a 1 KiB body into `Payload` (with `Item`, `map`, `*string`), changes it, encodes it again.
  - `Stamp` has a `MarshalJSON` method and is nested in `Payload`;
  - `Level` is a named `int` with `String()`, used through `fmt.Sprintf("%s/%d items")`.
- W3 `w3route`: precompiled `regexp` on the path, `strconv.Atoi/FormatInt/Itoa`, `fmt.Sprintf("%02d")`.

**Host** (`compare/bench`), in the Traefik style:
- `httptest.NewRecorder` and a native request copied at each iteration;
- a minimal `next` during the measurement, and a `next` that "dumps" the request to check the output;
- the output (status, sorted headers, body, what `next` receives) is compared with the native run compiled into the same binary;
- yaegi is loaded exactly as in Traefik: `interp.New` + `Use(stdlib.Symbols)` + `Eval import` + `Eval New` + `reflect.Call`.

**Configurations:**

| # | name | content |
|---|---|---|
| 1 | native | plugin compiled into the host |
| 2 | yaegi | yaegi master |
| 3 | vm-native | VM, fully native stdlib (reflect.Call at the boundary) |
| 4 | vm-src | VM, stdlib of the plugin interpreted from source (W1: strings, strconv, fmt, errors, sort, slices…; W2: + encoding/json, bytes, base64, utf16; W3: + regexp, regexp/syntax, unicode, bytes) |
| 5 | vm-mixed | VM, only encoding/json, fmt, errors, sort, slices, cmp, internal/fmtsort are interpreted; strings, strconv, regexp, bytes, net/http stay native |
| C′ | vm-aot | VM, native stdlib, except encoding/json replaced by `vjson/`: json v1 from GOROOT, rewritten mechanically by `scripts/mkvjson.py` against `vm.RV` / `vm.RT` |

**Additions to the spike VM** (about 2 300 lines; what serves 4 and 5 before optimization):
- `vm/vmreflect.go`: reflect inside the VM, that is the 83 reflect/reflectlite entry points used by json, fmt, errors, sort and fmtsort (listed by `cmd/reflscan`).
- `vm/intrinsics.go`: `internal/bytealg` (assembly), `abi.NoEscape`, `unsafe.String/SliceData/StringData/Slice`.
- `sync.OnceValue[T]` as an intrinsic (generic from a native package).
- Lazy compile errors: an unsupported construct fails only if it is executed.
- Opaque values for `any` passed to native code (sync.Pool, sync.Map).
- `vm/plugin.go`: API `Load`, `Plugin.Call`, `Object`, and `LoadCached`.
- **Note [F]**: the announced `VM_REFLECTLITE` option does not exist in the code of `spikes/vm` (grep finds nothing). The internal reflect was written here.

## Final measurements (all optimizations on) [M]

Files `results/final/bench-w*.tsv`. Time in µs per request (median, with [min–max] of CPU). The "host cost" (request copy, recorder, minimal next) is 0.6 / 0.4 / 0.2 µs (wall) for W1 / W2 / W3. The ratios do not subtract it.

### W1 headers

| config | output | wall | CPU [min–max] | allocs/op | B/op | wall ratio / yaegi |
|---|---|---|---|---|---|---|
| native | MATCH | 1.90 | 3.9 [3.9–4.0] | 31 | 2 712 | 0.16 |
| yaegi | MATCH | 12.1 | 17.1 [16.9–17.3] | 278 | 9 277 | 1 |
| vm-native | MATCH | 5.50 | 9.8 [9.8–10.1] | 164 | 6 583 | **0.45** |
| vm-src | MATCH | 33.6 | 48.1 [47.5–51.1] | 521 | 19 910 | 2.8 |
| vm-mixed | MATCH | 5.54 | 9.9 [9.7–10.0] | 164 | 6 583 | **0.46** |
| vm-aot | MATCH | 5.51 | 9.9 [9.7–10.0] | 164 | 6 583 | 0.45 |

### W2 json

| config | output | wall | CPU [min–max] | allocs/op | B/op | wall ratio / yaegi |
|---|---|---|---|---|---|---|
| native | MATCH | 12.3 | 19.7 [19.4–19.8] | 104 | 8 369 | 0.55 |
| yaegi | **DIFF** | 22.3 | 38.4 [34.3–39.0] | 348 | 16 514 | 1 |
| vm-native | **DIFF** | 55.6 | 89.6 [87.8–91.1] | 642 | 50 995 | 2.5 |
| vm-src | MATCH | 529.8 | 706 [687–709] | 5 781 | 212 760 | **23.7** |
| vm-mixed | MATCH | 636.7 | 843 [839–878] | 9 106 | 278 014 | **28.5** |
| vm-aot | MATCH | 25.2 | 34.6 [34.5–35.0] | 457 | 18 418 | **1.13** |

- **yaegi** is wrong: `MarshalJSON` is ignored and `"when"` comes out as `{"Sec":1700000000,"Zone":"UTC"}` instead of `"1700000000@UTC"`. This is upstream-issue 1486 / 1490, still present on master.
- **vm-native** is wrong in the same way: this is the reflect wall. The struct is copied into a `reflect.StructOf` type without methods.
- For vm-native, the copy back on return had to be fixed first: it was not done through an `any` parameter. Without this fix, `json.Unmarshal` filled nothing (prototype: empty output).
- **Update (2026-10-21)**: in default mode (vm-native), `json.Marshal`, `MarshalIndent` and `Unmarshal` now go to the AOT copy of json when the value holds interpreted types (`vm/aotroute.go`). W2 vm-native is now **MATCH**, at 33.7 µs wall (47.6 µs CPU, 377 allocs/op, 19 889 B/op) against 77.3 µs (DIFF, 632 allocs/op, 56 680 B/op) before, on the same machine (native: 13.8 µs). `./bench -w w2 -cfg native,vm-native -count 10 -benchtime 300ms`.

### W3 regexp/strconv

| config | output | wall | CPU [min–max] | allocs/op | B/op | wall ratio / yaegi |
|---|---|---|---|---|---|---|
| native | MATCH | 1.15 | 2.7 [2.6–2.8] | 22 | 2 064 | 0.20 |
| yaegi | MATCH | 5.89 | 8.5 [8.2–10.9] | 149 | 6 088 | 1 |
| vm-native | MATCH | 4.11 | 6.9 [6.8–7.0] | 102 | 4 099 | 0.70 |
| vm-src | MATCH | 31.9 | 32.2 [31.9–33.4] | 205 | 10 302 | 5.4 |
| vm-mixed | MATCH | 13.4 | 18.0 [17.6–18.4] | 163 | 7 494 | 2.3 |
| vm-aot | MATCH | 4.23 | 6.7 [5.6–7.1] | 102 | 4 098 | 0.72 |

### Loading and memory

Files `results/final/load.tsv` and `rss.tsv`: a new process per configuration, 3 repetitions.

| config | cold (source → handler ready) | with cache (`LoadCached`) | heap kept after loading | max RSS |
|---|---|---|---|---|
| native | 0 | - | - | 25 MB |
| yaegi | 1.7–2.8 ms | 1.9–2.1 ms | 1.5 MB | 29–30 MB |
| vm-native | 89–199 ms (median ~90) | 0.3–0.6 ms | 3.3–4.0 MB | 35–37 MB |
| vm-src | 225–315 ms | 2.5–3.1 ms (W3: 22 ms) | 47–91 MB | 237–320 MB |
| vm-mixed | 181–309 ms | 0.5–3.3 ms | 24–36 MB | 172–195 MB |
| vm-aot | 79–122 ms (median ~88) | 0.3–0.7 ms | 2.6–4.0 MB | 35–38 MB |

- **Cold**, almost all the time goes to `go list` and to the go/packages type checking. SSA and init take less than 50 ms; only vm-src W3 has 25 ms of init (interpreted unicode/regexp tables).
- Traefik does not ship the Go toolchain. A real product would ship the stdlib types, plus the SSA or the bytecode of the interpreted packages.
- **`LoadCached`** measures this case: the plugin is type checked against packages already loaded and added to the existing SSA program. The cost per plugin is 0.3 to 3 ms, at the level of yaegi.
  - The W3 vm-src case at 22 ms replays the init of the interpreted packages for each new machine; this could be avoided.
- **Memory**: vm-src and vm-mixed keep the go/packages ASTs and `types.Info` in memory (+150 to 290 MB of RSS). This cost is not measured in production [E]. The type headers and the bytecode alone would fit in a few MB.

### Cost of the budget per request

`BenchmarkServeBudget` (`vm/budget_bench_test.go`): a trivial request of a plugin, without a budget and with `Budget` 1e8 (one account per request, `vm/budget.go`), on one goroutine and on 12 (`RunParallel`). **Update (2026-10-21)**: the account of a call from the host is now found in a lock-free table per goroutine, not in a map under a process-wide lock taken three times per request. Median of 8 runs, before and after binaries run in turn on the same busy machine (12 cores): with a budget, in parallel, 1653 → 1270 ns per request, now at the level of no budget (1324 ns); serial unchanged (about 3.0 µs with or without a budget). `go test ./vm -run '^$' -bench BenchmarkServeBudget -benchtime 200000x`.

## Where the time goes (W2 profiles, configurations 4 and 5) [M]

Sources:
- pprof profiles: `results/final/w2-vm-src.pprof` and `w2-vm-mixed.pprof` (summarized in `*.top.txt`);
- VM instruction profile: `results/final/w2-*.vmprof.txt`, with `VM_PROFILE=1 ./bench -w w2 -cfg vm-src -vmprof 200`.

After optimization, vm-src runs **72 800 VM instructions per request**, against 99 600 before. vm-mixed runs 57 400, plus 749 native calls.

The 4 json functions that dominate: `rescanLiteral`, `checkValid`, `unquoteBytes`, `decodeState.object`. With the state function called through a closure for each byte, they make up about 40 % of the instructions.

| # | Item | vm-src | vm-mixed |
|---|---|---|---|
| 1 | Dispatch loop (flat `exec`: fetch, `switch`, type assertions on `in.X`) | 31 % of samples; 73 k instructions × ~3 ns | 17 % |
| 2 | Allocations + GC: `mallocgc`, scan, `madvise`; `convTslice` (each slice put into `Value.R any` allocates a 24 B header), `deepCopy` of structs (7 %), strings | ~35 % | ~40 % |
| 3 | Native boundary through `reflect.Call` (bytes.Buffer, strconv, utf8 called for each token) | ~0 | `reflect.Value.call` 11.5 % → vm-mixed is **slower** than vm-src |
| 4 | Calls / returns / frames: Call+Ret+CallClosure ≈ 11 % of instructions; the json scanner calls `step` through a closure at each byte | 11 % instr. | 9 % instr. |
| 5 | Generic memory access (FieldLoad/IndexLoad/FieldStore ≈ 17 % of instructions, each with a type switch on `R`) and phi copies (`Mov` 11 %, `Jmp` 13 %: unoptimized SSA, no inlining) | 41 % instr. | 38 % instr. |

The VM reflect intrinsic (`Func`) counts for only 2 to 3 %: the internal reflect is not the bottleneck.

## Optimizations tried and their effect [M]

File `results/final/ablation.tsv`. The optimizations are turned on one by one, cumulatively, through `VM_OPT`. Wall median over 5 repetitions, in µs.

| step | W1 vm-native | W2 vm-src | W2 vm-mixed | W2 vm-aot | W3 vm-native | W3 vm-mixed |
|---|---|---|---|---|---|---|
| prototype (`VM_OPT=none`) | 21.0 | 686 | 800 | 94.7 | 12.6 | 25.9 |
| + 1. pool of execution contexts (96 KiB stack allocated at each native → VM call) | 15.9 | 669 | 787 | 79.6 | 7.0 | 19.5 |
| + 2. multiple results in consecutive registers (no more allocated tuples) | 15.7 | 619 | 788 | 78.6 | 7.0 | 17.3 |
| + 3. specialized native trampolines (direct Go call per signature, without reflect.Call) | **5.7** | 590 | 753 | 48.3 | **4.1** | 13.9 |
| + 4. memoization of the types derived from the internal reflect | 5.6 | 587 | 752 | 30.4 | 4.1 | 13.8 |
| + 5. zero-copy native `[]byte` (instead of `[]Value`, 24 B per byte) | 5.6 | 629* | 688 | **26.4** | 4.3 | 14.3 |
| + 6. superinstructions (FieldAddr/IndexAddr+Load/Store, compare+jump, fall-through) | 6.0 | **519** | **632** | 25.8 | 4.1 | 12.9 |

\* noise: the other measurements of this step give ~560 µs.

- Total gain on the VM **interpreting** json: **1.3×** (686 → 519 µs). The dispatch and the value model `{N int64; R any}` are the remaining limits.
- Total gain on the boundary (W1, W3): **3.7× and 3×**. The VM goes below yaegi, because native calls no longer use `reflect.Call`, while yaegi still uses it.
- Total gain on vm-aot: **3.7×**.
- The trampolines of step 3 are written by hand for about 15 signatures. In production, they would be generated for the whole stdlib, as yaegi's `extract` does [E].
- One regression was found and fixed along the way: the O(n²) fusion that searched for the next instruction made the W3 vm-src init go from 25 ms to 860 ms.

## Reachable ceiling for a well-built VM that interprets the stdlib [E]

- **Today** [M]: 519 µs for 72.8 k instructions, so about 7 ns per instruction all included (of which ~3 ns of dispatch and ~4 ns of allocations and GC). Interpreting json costs about **43× native json** (530 against 12.3 µs).
- **Number of instructions**: can be divided by 1.5 to 2 with inlining of small functions (`isSpace`, `stateX`, `tryGrowByReslice`) and a register allocation that removes the phi `Mov`s. go/ssa does none of these optimizations. We would get about 36 to 48 k instructions.
- **Cost per instruction**: a `switch` loop in Go has no computed goto and no guaranteed tail call. With typed registers without boxing (no `any` for slices and strings), a good VM is around **1.5 to 3 ns per instruction**, allocations included.
- **Result**: 55 to 145 µs for W2, so **2.5 to 6.5× yaegi**, more likely 4 to 8× when counting the remaining boundary.
- **To go down to 2× yaegi** (45 µs), we would need about 1 ns per instruction all included over 36 k instructions. No known `switch` interpreter written in Go reaches that: it would need a JIT, which is out of scope in pure Go.
- **Points of comparison (from memory, not checked)**:
  - a "classic" bytecode interpreter is about 10 to 50× slower than native code on byte-oriented code: Lua 5.4 against C about 10 to 30×; CPython about 30 to 100×;
  - the interpreter mode of wazero is about 10 to 20× slower than its compiled mode;
  - goja and otto (JS in Go) are tens of times slower than the V8 JIT;
  - measurements of this spike [M]: fib(30) VM = 24× native, yaegi = 280× native; interpreted json = 43× native.
- **Reachable VM/yaegi ratio, by plugin type**:
  - plugin that mostly makes stdlib calls on basic types (W1, W3): **0.3 to 0.7×** (already measured 0.45 and 0.70);
  - plugin that makes the VM interpret a heavy part of the stdlib (json, fmt, template): **4 to 8×**, against 24× today.

This last point is the reason for the no-go of C "interpreted stdlib". It is **structural**: it comes from interpretation itself, not from the prototype. The remaining margin on the prototype is 3 to 6×, not enough for a factor of 12 to 14.

## Variant C′ (AOT) and its limits [M]/[O]

**Principle.** The stdlib packages that must see the plugin types (json, fmt, errors, sort, text/template…) are fixed when Traefik is compiled. Instead of interpreting them, we **compile them ahead of time** against a VM reflect API (`vm.RV`, `vm.RT`: `vm/vreflect.go`, ~650 lines).

**How vjson is made**:
- `scripts/mkvjson.py` copies the 8 v1 files of `encoding/json` from the local GOROOT and replaces 11 `reflect.X` identifiers with `vm.X`;
- **no other change**; the code compiles on the first try;
- 20 lines of glue (`vjson/vmglue.go`) route the plugin's `json.Marshal` and `json.Unmarshal` to vjson.

**Why the reflect wall falls.**
- The only interface assertions that json makes on values go through `reflect.TypeAssert[Marshaler / Unmarshaler / TextMarshaler / TextUnmarshaler]`. This is a **finite set, known at generation time**.
- So the needed native wrappers are finite in number: 4 wrappers, instead of the 2^n of the general case.
- The plugin types keep their identity and their methods: the interpreted `Stamp.MarshalJSON` is really called.

**W2 result.** 25.2 µs (wall) and 34.6 µs (CPU), against 22.3 and 38.4 µs for yaegi, with a correct output that yaegi does not get.

**Measured or known limits:**
1. `json.Number` declared in the plugin types is not recognized. vjson compares `v.Type() == numberType` with a type of native origin, and the pointer equality fails. The fix is to generate these variables lazily, which needs a small change to the generator.
2. Only json was moved to AOT. Native fmt keeps the defects of the wall. Here `%s` of a `Stringer` works thanks to the wrapper, but `%T`, `%d` on `Level` and `%v` of an interpreted struct would be wrong. fmt (and errors, sort, text/template) would need to go through the same generator [E: 1 to 2 days per package, plus the oracle of their stdlib tests].
3. The generator must be maintained for each Go version: json v1 and v2 live side by side behind `goexperiment.jsonv2`.
4. The internal reflect implements only what json, fmt, errors and sort use: 83 entry points, without `Complex`, `Convert`, `Method` or `Call`.

## What blocked interpretation from source, and the effort it took (configuration 4) [M]

| Package | Blocker | Fix | Agent effort |
|---|---|---|---|
| errors, fmt, sort, encoding/json, internal/fmtsort | `internal/reflectlite.TypeOf` at init, then all of reflect | reflect inside the VM (`vmreflect.go`, 83 entry points) | ~1 h |
| encoding/json | `sync.OnceValue[T]`: generic from a native package, with no native instantiation | Go intrinsic (VM closure) | 10 min |
| fmt | `complex64` not supported in a branch that never runs | lazy compile errors | 10 min |
| fmt, json (sync.Pool, sync.Map) | interpreted values copied into `StructOf` | opaque handles for `any` | 15 min |
| strings, bytes | `internal/bytealg` (arm64 assembly), `abi.NoEscape`, `unsafe.String/SliceData` (strings.Builder) | 10 Go intrinsics; `unsafe.Slice` trick on a contiguous `[]Value`, exact aliasing | 20 min |
| strconv, internal/strconv, unicode, utf8, regexp, regexp/syntax, slices, cmp, base64, utf16 | none, once the fixes above are in place | - | - |
| net/http | not tried: the host provides native `*http.Request` and `ResponseWriter`; 183 packages, including unsafe and syscall (see stdscan) | - | - |

## Reproduce

```bash
compare/bench/run.sh final 10 300ms   # ≈ 8 min: the whole report -> compare/bench/results/final/
#   1. bench-w{1,2,3}.tsv : cost/request + correctness, all configs, round-robin
#   2. load.tsv, rss.tsv  : cold / cached loading, RSS (new process ×3)
#   3. w2-vm-{src,mixed}.pprof/.top.txt : W2 CPU profiles
#   4. ablation.tsv       : cumulative effect of the optimizations (VM_OPT)
# One at a time, from the repository root (bench loads plugins/ from its working directory):
(cd compare && go build -o ../bench ./bench)
./bench -w w2 -cfg native,yaegi,vm-src,vm-aot -count 10 -benchtime 300ms -show
VM_OPT=none ./bench -w w2 -cfg vm-src              # prototype without optimization
VM_PROFILE=1 ./bench -w w2 -cfg vm-src -vmprof 200 # VM instruction profile
./bench -w w2 -cfg vm-mixed -cpuprofile w2.pprof && go tool pprof -top bench w2.pprof
VM_NOCACHE=1 ./bench -w w1 -cfg vm-native          # "warm" loading without cache
go run ./cmd/reflscan encoding/json fmt errors sort internal/fmtsort  # required reflect API
```

**Columns of the bench TSVs**: workload, config, verdict, wall median, wall min, CPU median, CPU min, CPU max (ns), allocs/op, B/op, cold loading (ms), cached loading (ms), heap after loading (KiB), n, detail.

**Useful options**:
- `VM_TRACE=1` prints the Go stack of a VM panic;
- `VM_OPT=-x,-y` turns off some optimizations (`pool`, `tuple`, `fastnative`, `memo`, `bytes`, `super`, `appendv`, `agglit`, `rospill`, `deadload`, `shallow`, `phi`, `escape`; [vm/opts.go](../vm/opts.go) has the full list, with one line on each); `VM_OPT=none` turns them all off;
- `VM_LIMITS=1` loads the vm configurations with the limits of the catalog host (`internal/pluginhost`: `Budget`, `BackgroundRate`, `MaxGoroutines`, `PanicOnFatal`, `MaxMemory`), to measure their cost; `VM_BUDGET=n` sets the budget alone.

The prototype measurements made before any change to the harness (`results/proto/`) are consistent with the `none` line of the ablation: W2 vm-src 680 µs, W1 vm-native 23 µs.

## Files

- `compare/bench/run.sh` (writes `compare/bench/results/{label}/`)
- `plugins/w1headers`, `plugins/w2json`, `plugins/w3route`; for yaegi, the host links `plugins/` into a temporary GOPATH.
- `compare/bench` (host), `cmd/reflscan` (required reflect API)
- `vm/`: the VM
