# yaegi vs yaegi v2: what Traefik plugins can do, measured

This is an experimental study from October 17, 2026, with 79 programs. Each program runs under `go run`, under yaegi v0.16.1 configured as in Traefik, and under yaegi v2 (commit 67381ed). Each table row links to a program in `progs/`. The outputs of the last run are not under version control. The summary of the study is in `results/STATUS-2026-10-17.txt`.

## Update: standard library as in Go (October 19, 2026, linux/amd64)

`yaegi` without `-restricted` and `-restrictedstd` gives programs the standard library as in Go: the real `os.Exit` and `log.Fatal`, and the symbols of `os/exec` and `syscall` (`stdsyms.Unrestricted`). `run.sh` runs yaegi v2 this way. yaegi keeps its restricted stdlib, as in Traefik. We ran again only the programs that use these functions:

| program | yaegi v2 before | yaegi v2 now | yaegi v2 -aot before | yaegi v2 -aot now |
|---|---|---|---|---|
| s01 (`os/exec`) | unsupported | identical | unsupported | identical |
| s03 (`syscall`) | unsupported | identical | unsupported | identical |
| s04 (`os.Exit(7)` after a `defer`) | panic | exit code 7, no deferred call | panic | exit code 7, no deferred call |

s04 now behaves like the compiled program: it prints `before`, skips the deferred call and ends with code 7. `run.sh` still reports `EXIT7`, because `go run` itself ends with code 1 and prints `exit status 7`. s02 and s08 do not change (identical). With `-restricted`, s01 and s08 are still refused at load time, and with `-restricted` or `-restrictedstd`, `os.Exit` still panics, as in yaegi (`TestRestrictedPrograms`).

So, out of the 82 programs of the phase 2 bis table, two more are identical to `go run` in each yaegi v2 mode (s01 and s03).

## Update: phase 2 bis (October 17, 2026, linux/amd64)

After the second round of fixes, "phase 2 bis" (native registry for the host's Go version, method pointers through a native `any`, per-machine entries in the AOT maps), we ran all programs again: `results/STATUS-2026-10-17-linux-phase2bis.txt`.

Out of the 82 programs that are in all summaries, the output is identical to `go run` for:

| engine | first measurement | phase 2 | phase 2 bis |
|---|---|---|---|
| yaegi | 36 | 36 | 36 |
| yaegi v2 | 25 | 33 | 37 |
| yaegi v2 -aot | 37 | 52 | 55 |

Out of the 87 current programs: yaegi 39, yaegi v2 39, yaegi v2 `-aot` 57.

Statuses changed by phase 2 bis:

| program | yaegi v2 | yaegi v2 -aot |
|---|---|---|
| g02 (range-over-func) | unsupported → identical | unsupported → identical |
| r08 (`sort`), r08b (generic `slices` from the stdlib) | unsupported → identical | unsupported → identical |
| r12 (plugin values in native interfaces) | panic → identical | identical |

## Update: phase 2 (October 17, 2026, linux/amd64)

After the first round of fixes, "phase 2" (atomics, context values, import restrictions, AOT state per plugin, loading without the toolchain, reflect on plugin values, caches), we ran all programs again on linux/amd64 (Go 1.26.0): `results/STATUS-2026-10-17-linux-phase2.txt`. The tables below are still those of the first measurement (darwin/arm64, commit 67381ed).

Out of the 82 programs that are in both summaries, the output is identical to `go run` for:

| engine | first measurement | phase 2 |
|---|---|---|
| yaegi | 36 | 36 |
| yaegi v2 | 25 | 33 |
| yaegi v2 -aot | 37 | 52 |

Statuses changed by phase 2:

| program | yaegi v2 | yaegi v2 -aot | task |
|---|---|---|---|
| u08d (`atomic.AddInt64`, 200 goroutines) | wrong output → identical | wrong output → identical | 2 |
| u08c (`StorePointer`/`LoadPointer`) | panic → identical | panic → identical | 2 |
| u08b (`atomic.Value` + `atomic.Int64`) | panic → identical | wrong output → identical | 2, 8 |
| r12a (`context.WithValue`) | panic → identical | panic → identical | 3 |
| r12d (named string context key) | identical | panic → identical | 3 |
| p01, p02, p03, r05, r06, r07 (reflect in the plugin) | unchanged | unsupported → identical | 7 |
| l11 (fatih/structs), l12 (reflect in minilib) | unchanged | unsupported → identical | 7 |
| r12 (plugin values in context, sync.Map and sync.Pool) | unchanged | panic → identical | 3, 8 |
| r12b (`sync.Map`), u08e (`atomic.Value`) | panic → identical | identical | 8 |
| l17 (golang-lru) | wrong output → identical | identical | 8 |
| l15 (go-cache) | panic → identical | panic → identical | 8 |

yaegi v2 with `-restricted` refuses programs s01, s02 and s08 at load time (task 4, `TestRestrictedPrograms`). `run.sh` runs yaegi v2 without restrictions, as before (since October 19, also with the standard library as in Go, see above).

Some differences come from the platform, not from phase 2. x03 no longer compiles under `go run` (the asmlib assembly only targets arm64). l06 and l10 panic under yaegi instead of being refused (linux files of x/sys/unix). s05 (infinite loop) hits the timeout everywhere. Before the fix of `timed` (`env.sh`), it blocked the run.

## Reproduce

```bash
sh ../../scripts/setup.sh           # once: modules (network) and hooks
./build.sh                          # bin/govm and bin/yrun
for f in progs/*.go; do ./run.sh "$f"; done > out/STATUS.txt
./status.sh                         # recomputes the statuses from out/
./runtags.sh l01_xxhash l07_easyjson   # yaegi v2 with the purego,appengine tags
./timing.sh hello.go                   # startup time (paths relative to progs/)
```

`run.sh prog.go` runs the program under `go run`, `bin/yrun` (yaegi), `bin/govm` and `bin/govm -aot`. The outputs go to `out/<prog>.<mode>.txt`, and the exit codes go to `.rc`. The script prints one status line.

## Method

- **Machine**: Apple M3 Pro, darwin/arm64, Go 1.26.8. A benchmark was running at the same time, so we ran the programs one by one.
- **yaegi**: v0.16.1, the version in Traefik's go.mod. `yrun/main.go` reproduces `pkg/plugins/middlewareyaegi.go`:
  - `stdlib.Symbols` in restricted mode;
  - `unsafe.Symbols` and `syscall.Symbols` added, which is the same as `useUnsafe: true`; with `-unsafe=false`, it is a plugin without `useUnsafe`;
  - the dependencies are served by a GOPATH of links to the module cache.
- **yaegi v2**: `-aot` uses the AOT copies and implies `-opaque`.
- **Oracle**: `go run`.
- **Statuses**:

| Status | Meaning |
|---|---|
| identical | same output, exit code 0 |
| wrong output | exit code 0 but different output |
| error | refused at load time (yaegi) |
| panic | panic at run time |
| unsupported | yaegi v2 exits with code 3 |

The raw status files in `compare/yaegi/results/` use the codes OK, DIFF, ERR, PANIC and UNSUP for these five statuses.

Limit: we only tested darwin/arm64. The results that depend on build tags (`x/sys/unix`, the xxhash assembly) can change on linux/amd64.

## Summary

- **The reflect wall.** yaegi breaks as soon as json, fmt, template or errors handle plugin types. yaegi v2 `-aot` gives the right output, but only for the libs in its AOT list.
- **Reflect used by the plugin itself.** Both sides handle this badly.
  - Default yaegi v2 loses `Value.Set*` writes.
  - With `-aot`, the VM's internal reflect lacks `FieldByName`, `MethodByName`, `Method`, `DeepEqual` and `NewAt`.
- **unsafe.**
  - yaegi v2 handles `unsafe.String`, `StringData`, `SliceData` and `Add`. yaegi 0.16.1 does not have them.
  - But yaegi v2 refuses, by design, any reinterpretation of bytes. xxhash, gjson, easyjson, protobuf and reflect2 depend on it.
- **Recent language features.** yaegi v2 wins.
- **Serious yaegi v2 regressions**, in both modes:
  - `atomic.AddInt64` is not atomic;
  - `context.WithValue(ctx, key{}, v)` panics;
  - go-cache panics.
- **Runtime constraints.** yaegi v2 needs the Go toolchain, loads 4 to 15 times slower than yaegi, and has no import allow list.

## 1. unsafe in the plugin code

| Case | yaegi | yaegi v2 | yaegi v2 -aot | Comment |
|---|---|---|---|---|
| [unsafe.String/StringData/SliceData/Slice](progs/u01_unsafe_string.go) | error | identical | identical | yaegi 0.16.1 does not have these symbols |
| [`*(*string)(unsafe.Pointer(&b))` and s2b](progs/u02_unsafe_oldidiom.go) | identical | unsupported | unsupported | "needs byte-level memory" |
| [s2b through `struct{string; Cap int}`](progs/u02b_unsafe_s2b_struct.go) | identical | unsupported | unsupported | fasthttp/gjson idiom |
| [zero-copy aliasing](progs/u03_unsafe_alias.go) | error | wrong output | wrong output | yaegi v2 copies |
| [`(*[4]byte)(unsafe.Pointer(&u32))`](progs/u04_unsafe_bytes_of_u32.go) | identical | unsupported | unsupported | structural |
| [struct→struct cast, `[16]byte`→struct](progs/u05_unsafe_struct_cast.go) | identical | unsupported | unsupported | bytes→struct refused |
| [`unsafe.Add` + `Offsetof` (uintptr)](progs/u06a_unsafe_add.go) | error | identical | identical | yaegi typing bug |
| [`unsafe.Add` (int)](progs/u06d_unsafe_add_int.go) | identical | identical | identical | |
| [`uintptr+Offsetof` then back to a pointer](progs/u06b_unsafe_uintptr.go) | identical | identical | identical | |
| [address identity](progs/u06c_unsafe_addr_identity.go) | identical | wrong output | wrong output | one cell per value |
| [Sizeof/Alignof/Offsetof](progs/u07_unsafe_sizeof.go) | identical | identical | identical | |
| [`atomic.Pointer[T]`](progs/u08a_atomic_pointer_generic.go) | error | unsupported | unsupported | native generic type |
| [`atomic.Value` + `atomic.Int64`](progs/u08b_atomic_value_int.go) | identical | panic | wrong output | |
| [`atomic.AddInt64`, 200 goroutines](progs/u08d_atomic_addint64_race.go) | identical | wrong output | wrong output | **not atomic** |
| [`atomic.Value` alone](progs/u08e_atomic_value_only.go) | identical | panic | identical | |
| [`atomic.StorePointer`/`LoadPointer`](progs/u08c_atomic_unsafe_pointer.go) | identical | panic | panic | |
| [`reflect.NewAt`](progs/u09a_reflect_newat.go) | identical | wrong output | unsupported | |
| [`reflect.SliceHeader`/`StringHeader`](progs/u09b_sliceheader.go) | identical | unsupported | unsupported | structural |
| [`unsafe.Pointer` to native](progs/u09c_unsafe_to_native.go) | error | unsupported | unsupported | |

### Real libraries with unsafe

The "+ tags" column runs yaegi v2 with `-tags purego,appengine,nounsafe,safe` (`runtags.sh`). yaegi already takes these fallbacks on its own, because its build context is not `gc`. `vm.Load` (plugins, toolchain-free loader) now sets `purego` by itself; `govm` and this study still use `go list`, without tags.

| Case | yaegi | yaegi v2 | yaegi v2 -aot | yaegi v2 + tags |
|---|---|---|---|---|
| [cespare/xxhash v2.3.0](progs/l01_xxhash.go) | identical | unsupported | unsupported | **identical** |
| [tidwall/gjson v1.17.0](progs/l02_gjson.go) | error | unsupported | unsupported | unsupported |
| [json-iterator](progs/l03_jsoniter.go) | error | unsupported | unsupported | unsupported |
| [protobuf v1.36.11](progs/l04_protobuf.go) | panic | unsupported | unsupported | unsupported |
| [klauspost/compress](progs/l05_klauspost.go) | error | unsupported | unsupported | unsupported |
| [x/sys/unix](progs/l06_xsys_unix.go) | error | unsupported | unsupported | unsupported |
| [easyjson](progs/l07_easyjson.go) | identical | unsupported | unsupported | **identical** |

## 2. Native reflect (stdlib) on plugin types

| Case | yaegi | yaegi v2 | yaegi v2 -aot |
|---|---|---|---|
| [json: tags, embedding, pointers](progs/r01_json_tags.go) | wrong output | panic | identical |
| [json: MarshalJSON/UnmarshalJSON/TextMarshaler](progs/r02_json_methods.go) | wrong output | wrong output | identical |
| [json: interface field](progs/r11_json_interface_field.go) | wrong output | wrong output | identical |
| [fmt, Stringer, %w](progs/r03_fmt_stringer.go) | wrong output | wrong output | identical |
| [text/template with methods](progs/r04_template.go) | panic | panic | identical |
| [reflect.DeepEqual](progs/r05_deepequal.go) | identical | identical | unsupported |
| [TypeOf Name/PkgPath/MethodByName](progs/r06_typeof_methods.go) | panic | panic | unsupported |
| [reflect.New/Zero/StructOf/MakeMap](progs/r07_new_zero_structof.go) | wrong output | panic | unsupported |
| [sort.Sort/Slice/Stable](progs/r08a_sort_interface.go) | identical | identical | identical |
| [slices.SortFunc, maps.Keys](progs/r08b_slices_generic_std.go) | error | unsupported | unsupported |
| [errors.As/Is/Join](progs/r09_errors.go) | panic | panic | identical |
| [plugin http.Handler](progs/r10_http_handler.go) | identical | identical | identical |
| [context.WithValue, `struct{}` key](progs/r12a_context_value.go) | identical | **panic** | **panic** |
| [context.WithValue, named string key](progs/r12d_context_string_key.go) | identical | identical | panic |
| [sync.Map](progs/r12b_sync_map.go) | identical | panic | identical |
| [sync.Pool](progs/r12c_sync_pool.go) | identical | identical | identical |
| [Mutex/RWMutex/Once](progs/c01_mutex_counter.go) | identical | identical | identical |

`r08b` passes in yaegi v2 with `-src slices,maps,iter`.

## 3. Reflect in the plugin code

| Case | yaegi | yaegi v2 | yaegi v2 -aot | Comment |
|---|---|---|---|---|
| [fields, tags, Set*](progs/p01_reflect_fields_tags.go) | wrong output | wrong output | unsupported | writes lost; `FieldByName` missing |
| [method by name](progs/p02_reflect_call_by_name.go) | panic | panic | unsupported | `Type.Method` |
| [Value.Set through a pointer](progs/p03_reflect_set_pointer.go) | identical | panic | unsupported | `Value.FieldByName` |
| [walk by Kind](progs/p04_reflect_typeswitch_kinds.go) | wrong output | wrong output | identical | |
| [identity of native types](progs/p05_reflect_vs_native_type_identity.go) | identical | identical | identical | |

## 4. Third-party libraries

| Case | In the AOT list? | yaegi | yaegi v2 | yaegi v2 -aot |
|---|---|---|---|---|
| [local reflect mini-lib](progs/l12_minilib_reflect.go) | no | wrong output | wrong output | unsupported |
| [fatih/structs](progs/l11_fatih_structs.go) | no | wrong output | wrong output | unsupported |
| [mapstructure with hook](progs/l13_mapstructure.go) | yes | wrong output | wrong output | identical |
| [validator v10](progs/l14_validator.go) | yes | error | unsupported | identical |
| [spf13/cast](progs/l18_cast.go) | yes | error | wrong output | identical |
| [golang-jwt v5](progs/l08_jwt.go) | no | wrong output | wrong output | identical |
| [google/uuid](progs/l09_uuid.go) | no | **wrong UUID** | identical | identical |
| [logrus](progs/l10_logrus.go) | no | error | unsupported | unsupported |
| [go-cache](progs/l15_gocache.go) | no | identical | panic | panic |
| [golang-lru](progs/l17_lru.go) | no | identical | wrong output | identical |
| [semver](progs/l16_semver.go) | no | identical | identical | identical |

logrus is identical under yaegi v2 with `-tags appengine`. We did not test gorilla/mux, fastjson and go-json.

## 5. Recent language features

| Case | yaegi | yaegi v2 |
|---|---|---|
| [generics](progs/g01_generics.go) | error | identical |
| [constraints with methods](progs/g04_generic_constraint_methods.go) | identical | identical |
| [range-over-func](progs/g02a_range_func_only.go) | panic | identical |
| [iter.Pull, slices.Collect](progs/g02_range_func.go) | error | unsupported |
| [range over int](progs/g03a_range_int.go) | panic | identical |
| [Go 1.22 loop variable](progs/g03b_loopvar.go) | **wrong output** | identical |
| [min/max/clear](progs/g03c_min_max_clear.go) | error | identical |

## 6. linkname, cgo, assembly

[go:linkname](progs/x01_linkname.go), [cgo](progs/x02_cgo.go) and [assembly without a fallback](progs/x03_asm.go) break on both sides. The pure Go fallbacks work.

## 7. Runtime constraints

| Test | yaegi | yaegi v2 |
|---|---|---|
| PATH without `go` | identical | **failure**: "go command required" |
| `GOROOT=/nonexistent` | identical | **failure** |
| Hello plugin, startup | 18 ms | 83 ms |
| HTTP handler plugin | 19 ms | 118 ms |
| Plugin with a dependency (semver) | 27 ms | 410 ms |

These are "wall" times, median of 5, measured while a benchmark was running. We did not test `vm.LoadCached`.

## 8. Sandbox

| Test | yaegi (Traefik) | yaegi v2 |
|---|---|---|
| `import "unsafe"` without `useUnsafe` | refused at load time | runs: no option |
| `os/exec` called | refused at load time | unsupported at run time |
| `os/exec` imported, never called | refused | loaded |
| write into a host object through unsafe | possible with `useUnsafe` | impossible |
| infinite loop | no budget | `-budget`, `MaxDepth` |

## Possible with yaegi, broken with yaegi v2

| Case | Cause | Fixable? |
|---|---|---|
| Byte reinterpretation (u02, u02b, u04, u05, u09b) | value model without byte-level memory | structural |
| xxhash, easyjson | same | yes with the purego tags (verified) |
| `atomic.AddInt64`, `Store/LoadPointer` (u08d, u08c) | pointer copied, then copied back | atomic intrinsics, 1 to 3 days |
| `context.WithValue` (r12a, r12d) | native boundary, cause not found | a few days, **blocking** |
| go-cache (l15) | StructOf, SetFinalizer | a few days |
| reflect `Set`, `DeepEqual` in the plugin (p03, r05) | limited internal reflect in the VM | route to the twin, 1 to 2 weeks |
| `reflect.NewAt` (u09a) | no typed memory | structural |
| `syscall` | with `useUnsafe`, only a read-only subset (`docs/SANDBOX.md`); the rest is missing from the registry | trivial, it is a sandbox decision |

## Possible with yaegi v2, broken with yaegi 0.16.1

- json, fmt, template, errors and slog on plugin types, with `-aot`.
- mapstructure, validator, cast and jwt, with `-aot`.
- google/uuid: yaegi produces a wrong value.
- `unsafe.String`/`StringData`/`SliceData`/`Add`.
- Generics, range-over-func, range over int.
- Go 1.22 loop variable: yaegi is wrong without an error.
- `min`/`max`/`clear`.
- Execution budget.

## Not tested

- linux/amd64.
- `vm.Load`/`LoadCached`.
- A vendored or GOPATH plugin.
- aotgen on a lib chosen by the plugin.
- gorilla/mux, fastjson, go-json.
