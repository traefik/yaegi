# What plugins do: yaegi v0 against yaegi v2

84 small programs that copy what Traefik plugins do (unsafe, reflect,
third-party libraries, recent language features). Each program runs under
`go run`, under yaegi v0.16.1 configured as in Traefik, and under yaegi v2
in VM mode and in AOT mode. A program passes when its output and its exit
code are the ones of `go run`.

## Results (17 November 2026)

darwin/arm64, Go 1.26.10. Programs with the same output as `go run`:

| Area | Programs | yaegi v0 | yaegi v2 (VM mode) | yaegi v2 (AOT mode) |
|---|---|---|---|---|
| `u`: unsafe and atomics in the plugin code | 22 | 15 | 20 | 20 |
| `r`, `c`: standard library reflect on plugin types (json, fmt, template, errors, sort, context, sync) | 19 | 9 | 19 | 19 |
| `p`: reflect in the plugin code | 5 | 2 | 5 | 5 |
| `l`: third-party libraries | 18 | 5 | 18 | 18 |
| `g`: recent language features | 8 | 1 | 8 | 8 |
| `x`: linkname, cgo, assembly | 3 | 0 | 1 | 1 |
| `s`: access to the host process | 9 | 5 | 5 | 5 |
| Total | 84 | 37 | 76 | 76 |

The status of each program is in
[results/STATUS-2026-11-17.txt](results/STATUS-2026-11-17.txt).

The programs that yaegi v2 does not pass, the same in both modes:

| Program | yaegi v0 | yaegi v2 | Comment |
|---|---|---|---|
| [u06_unsafe_arith](progs/u06_unsafe_arith.go) | error | wrong output | pointer arithmetic inside a struct |
| [u06c_unsafe_addr_identity](progs/u06c_unsafe_addr_identity.go) | identical | wrong output | address identity: yaegi v2 has one cell per value |
| [x02_cgo](progs/x02_cgo.go) | load error | load error | cgo; `go run` fails too with `CGO_ENABLED=0` |
| [x03_asm](progs/x03_asm.go) | error | unsupported | assembly without a Go fallback |
| [s04_os_exit](progs/s04_os_exit.go) | panic | exit code 7 | `os.Exit(7)`: yaegi v2 ends like the compiled program; `go run` itself ends with code 1 |
| [s05_infinite_loop](progs/s05_infinite_loop.go) | timeout | timeout | an infinite loop, killed by the timeout in every engine |
| [s07_reflect_unexported_write](progs/s07_reflect_unexported_write.go) | identical | unsupported | a write into an unexported field of a host type through reflect and unsafe |
| [s09_unsafe_write_native_struct](progs/s09_unsafe_write_native_struct.go) | identical | unsupported | a write into a host object through unsafe |

The failures of yaegi v0 are mostly the reflect wall (json, fmt, template
and errors do not see the methods of plugin types), missing symbols
(`unsafe.String`, generic packages of the standard library), generics,
range-over-func, range over int, and `min`/`max`/`clear`.

## Method

- **yaegi v0**: v0.16.1, the version in Traefik's go.mod. `yrun/main.go`
  reproduces `pkg/plugins/middlewareyaegi.go`: `stdlib.Symbols` in
  restricted mode, plus `unsafe.Symbols` and `syscall.Symbols` (the same as
  `useUnsafe: true`; `-unsafe=false` is a plugin without `useUnsafe`). The
  dependencies are served by a GOPATH of links to the module cache.
- **yaegi v2**: `cmd/yaegi` of this repository, without restrictions (the
  standard library as in Go). `TestRestrictedPrograms` checks that
  `-restricted` refuses s01, s02 and s08 at load time.
- **Oracle**: `go run`.

The status files use these codes:

| Code | Meaning |
|---|---|
| OK | same output, same exit code |
| DIFF | exit code 0 but different output |
| ERR | refused at load time (yaegi v0) |
| LOADERR | load or type error |
| PANIC | panic at run time |
| UNSUP | yaegi v2 exits with code 3 (construct not supported) |
| TIMEOUT | killed by the timeout |
| EXITn | another exit code |

The scripts and the status files name yaegi v2 `govm`, its former name.

## Run

```bash
sh ../../scripts/setup.sh           # once: modules (network)
./build.sh                          # bin/govm (yaegi v2) and bin/yrun (yaegi v0)
for f in progs/*.go; do TO=30 ./run.sh "$f"; done
./status.sh                         # the status of every program, from out/
./runtags.sh l01_xxhash l07_easyjson   # yaegi v2 with the purego,appengine tags
./timing.sh hello.go                   # startup time (paths relative to progs/)
```

`run.sh prog.go` runs the program under `go run`, `bin/yrun`, `bin/govm`
and `bin/govm -aot`. The outputs go to `out/<prog>.<mode>.txt`, and the
exit codes to `.rc`. `TO` is the timeout in seconds (default 180).
