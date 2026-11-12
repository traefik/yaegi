# Architecture

yaegi v2 is a Go interpreter built as a "state machine": the Go toolchain's own
front-end produces SSA, the VM compiles each function to a register bytecode
and runs it with explicit register and frame stacks. This page describes the
parts file by file; the [README](../README.md) gives the overview and
explains the execution modes. yaegi v2 is still a prototype: see
[TODO.md](../TODO.md) for what is missing.

```
source ──go/packages──▶ go/types ──go/ssa──▶ SSA ──vm.compile──▶ []Instr ──ctx.exec──▶ effects
                                                    (lazy, per function)      │
                                                                              └─ native calls: reflect / trampolines
```

## Front-end (`vm/run.go`, `vm/plugin.go`, `vm/srcload.go`)

- Plugins (`Load`, `srcload.go`) load without the toolchain: the packages of
  the plugin module and of its `vendor/` directory are parsed from source
  (`go/build` file selection for the platform, no cgo, build tag `purego`
  only where needed, see below)
  and type-checked
  against the standard library types of `stdtypes`. `stdtypes` embeds, per
  platform, the gc export data of every standard library package (written
  by `cmd/stdexport`, about 0.9 MB compressed) and the sources of `cmp`,
  `iter`, `slices` and `maps`, which it type-checks first so that their
  objects are shared with the export data: the VM interprets the instances
  of their generic functions. The data is decoded once per process (about
  85 ms) and shared by all plugins. SSA packages are created for the
  standard library packages the plugin reaches, from their types only (their
  functions are called natively). The go directive of the module (and of
  each vendored module, from `vendor/modules.txt`) sets the language
  version; a directive newer than the release the VM was built with
  refuses the plugin, as the go command refuses to build with an older
  release than the one asked for, unless `Options.AllowNewerGo` lowers it
  to the release of the VM (the patch level is not compared). A `go.mod`
  without a go directive is a go 1.16 module, as for the go command
  (`goModVersion`). `go.mod` is parsed as the go.mod of a main module
  (`modfile.Parse`: an unknown directive is an error, except in a go.mod
  that asks for a newer release than the VM, read then with
  `modfile.ParseLax` as the go command reads it), and `vendor/modules.txt`
  must agree with it, as go build requires ("inconsistent vendoring", the
  same checks as cmd/go: `checkVendor`), unless
  `Options.IgnoreInconsistentVendoring` reads the vendor directory as it
  is. A plugin refused by one of these checks, or by a type error that an
  option of `Options` accepts, names the option in its error. `Options.GoList`
  (implied by `Options.Src`) keeps the `go list` path below. `Load` never
  takes that path on its own: on a platform without embedded types
  (`stdtypes.Platforms`), it returns an error unless the host sets
  `Options.GoList` (go list runs the Go toolchain on the plugin sources
  before the load-time checks).
- Nested plugin modules: when the plugin package is in a directory below
  `Options.Dir` that has its own `go.mod` (with the module path of that
  directory, as `plugins/traefik` of darkweak/souin), both loaders use that
  module as the go command does when it builds there: its go directive,
  its `vendor/` directory (not the one of the root), and `go list` runs in
  it. A package of the main module path that sits in a deeper nested module
  is not part of the main module either (vendor first). A package of the
  root module that the nested module does not vendor is refused, as the go
  command refuses that layout; with `Options.ImportFromArchive`, the
  toolchain-free loader reads it from its directory in the archive, as
  yaegi does. Apart from the `go.mod` of the module
  that holds `Options.Dir` (found as the go command finds it, also in a
  parent directory or through a symbolic link), the toolchain-free loader
  reads nothing outside that module. It follows symbolic links as Go does
  (a Kubernetes ConfigMap volume makes every file a link:
  `main.go -> ..data/main.go`, `..data -> ..2026_10_22_xxx/`), but only when
  the final target stays inside the module: a Go file, a nested `go.mod` or
  a `vendor/modules.txt` whose target leaves the module, is missing (a
  dangling link, a loop) or is not a regular file (a FIFO, a device), and a
  package or module directory that leaves the module through a symbolic
  link are refused. The files are opened through an `os.Root` at the module
  root, on the resolved path, so a link changed after the check cannot
  leave the module. It does not read `replace`
  directives: the vendor copy wins, as with `-mod=vendor`. The go list
  loader (`GoList`) uses the go command, which follows symbolic links and
  (with `-mod=mod`) `replace` directives, also outside the archive.
- The generated data (`stdtypes`, `stdsyms`, `aot/`, `vm/zz_natsigs_*.go`)
  exists once per Go release of the window, N-1 and N (`internal/gorelease`):
  the build constraint of each file keeps the data of the release of the
  build, and a `release_guard.go` per package stops the build of a host on a
  release outside the window. In AOT mode, `Load` and
  `RunMain` also compare the patch level of the AOT copies with the host
  (`vm/aotversion.go`).
- Couplings with the Go runtime that no API promises are checked when
  package `vm` starts, and the feature is off or refused when the check
  fails (fail closed): the layout of `runtime.funcinl` (`funcpc.go`), of
  `runtime.Frames` (`callers.go`), of `embed.FS` (`embed.go`), the stack
  bounds of the goroutine structure (`stackguard_g.go`), and the way
  `runtime.AddCleanup` treats globals and string constants (`inHeap`,
  `unsafe.go`), and the malloc header of a large `[]Value` that
  `runtime.findObject` shows (`cellStartOK`, `fatal.go`: with
  `PanicOnFatal`, `runtime.SetFinalizer` on a pointer inside an object is a
  panic of the plugin). `TestRuntimeCouplings` fails when one of these
  checks fails, so a new Go release shows it at once. The `//go:linkname`
  pulls of `runtime.nanotime`, `runtime.getAuxv` (`linkname.go`) and
  `runtime.findObject` (`fatal.go`) fail at link time.
- Build tags of the toolchain-free loader: the VM cannot run assembly, so
  when the files that go run selects for a package have assembly (`.s`
  files) or functions without body, the loader selects the files of that
  package again with `purego`, the tag that selects the pure Go version of a
  package that also has one in assembly (golang.org/x/crypto, x/sys/cpu,
  cespare/xxhash, klauspost/compress, protobuf; the standard library uses
  it the same way). Without it, the file with the bodyless declaration was
  taken and the first call panicked. The other packages keep the files of
  go run, also when they have `purego` variants, so both loaders and the AOT
  copies run the same code. yaegi v0.16.1 often gets the same files
  another way: it only reads `// +build` lines (`interp/build.go`, not
  `//go:build`) and does not know the `gc` tag, so the `!gc` fallbacks of
  files that still have `// +build` lines are true for it. gc-compiled Traefik does not
  matter here: it never compiles the plugin. The `go list` loaders (`yaegi`,
  `RunMain`, `Options.GoList`, `Options.Src`) do the same (`goasm.go`), but
  go list has one set of tags for all the packages and its overlays cannot
  change the module cache: when a package run from source has assembly that
  `purego` and `noasm` remove, they load again with these tags for all the
  packages. Other packages with such variants then take them too (they do
  the same work without assembly or unsafe). A standard library package
  that the VM interprets (`-src`) and that the tags would change keeps the
  files of go run instead (its native code is compiled without the tags),
  and the assembly is refused as before. The `// yaegi:tags t1 t2` comment
  lines of the plugin package (a directive of yaegi) add build tags only
  with `Options.YaegiTags` (and `RunOptions.YaegiTags`, `yaegi -yaegi-tags`,
  which the conformance runner sets): go build ignores them, and so do
  both loaders by default.
- Otherwise `golang.org/x/tools/go/packages` loads the program (`go list`
  underneath, so the Go toolchain must be installed), `go/types` type-checks it, and
  `go/ssa` builds SSA with `ssa.InstantiateGenerics` (generic functions are
  monomorphized by ssa, the VM never sees type parameters).
- The VM gets the whole type-checker for free: compile errors are go/types
  (gc) messages, not yaegi's.
- `RunMain` (used by `cmd/yaegi` and the conformance runner) loads a main
  package. The set of packages run *in the VM* is: the main package, every
  dependency outside the standard library (they have no native registry
  entry; this forces a full-syntax reload), and the packages listed in
  `RunOptions.Src` (`"all"`: every package outside the native floor,
  `IsFloor`). Everything else is called natively.
- `Load` / `LoadCached` (`plugin.go`) load a plugin package. `Plugin.Call`
  calls its exported functions by name and returns `Object`s: the host
  passes them back as arguments, converts them to native values
  (`Object.Interface`; an `http.Handler` gets one budget per ServeHTTP),
  calls their methods (`Object.Call`), or lets a native decoder
  (mapstructure) fill them (`Object.Decode`). `internal/pluginhost` calls
  the plugins of the catalog with it (`CreateConfig`, `New`). `LoadCached` type-checks a
  plugin against already loaded packages and adds it to the existing SSA
  program.
- `Plugin.Close` (`close.go`) stops every goroutine of a plugin with
  `ErrClosed` at its next blocking point or step refill, and makes later
  calls fail; `Plugin.CloseContext` lets the calls that run end first, up
  to a deadline. A host reloads a plugin by loading it again and draining
  the old one (`docs/RELOAD.md`): Go cannot stop the goroutines of one
  `New` only.
- What a load keeps of the front end (`vm/shrink.go`): ssa keeps the
  syntax and the `types.Info` of every function built from source, to build
  instances of generic functions on demand. After the SSA build and the
  load-time checks, `Load`, `LoadCached` and `RunMain` keep in `types.Info`
  only the entries of generic functions, drop the statements of the
  non-generic function bodies (except a body with a `for range` loop over
  an array, which `unevaluatedRange` reads), and drop the name resolution
  of `go/parser`. The SSA, the types and the position tables stay.
  `cmd/loadmem` measures the heap and RSS after a load.
- Load-time denial of service (`plugin.go`, `srcload.go`, `loaddos.go`):
  parse, type check and SSA build run on untrusted source before the VM
  budget exists. `LoadContext(ctx, ...)` and `Options.LoadTimeout` bound the
  whole load, so a plugin cannot block a config reload for ever (only when
  the host sets a deadline: the default `LoadTimeout` is 0, no deadline);
  the context is checked again before the preflight and before the init, so
  an abandoned load does not start them after the deadline (an init that is
  running when the deadline passes is canceled like a call whose context is
  done, see `budget.go`: the `*LoadError` then also wraps `ErrCanceled`).
  `Options.LoadLimits` bounds the
  source read (size, number of files and packages) of the default loader;
  with `GoList` or `Src`, `go list` reads the source and it does not apply.
  `Machine.zero`, `OpAlloc` and `OpMakeSlice` refuse a value of more than
  `maxZeroElems` Value entries (`zeroEntries` counts arrays and structs at
  any depth, the count is kept per type in `Machine.zeroCounts`) with a
  `*LoadError` instead of a fatal out-of-memory. The conversions of a native
  value to VM values (`fromNativeS`, `fromNativeDeepC`, `natToVMs`, `retVM`:
  a native array loaded through a pointer, slices and maps converted element
  by element) count their Value entries (`natSeen.grow`) and refuse more
  than `maxZeroElems` in one conversion. The bound is per value, not per
  plugin: many values just under it still exhaust memory. The
  `go/types` phase has no cancellation, so a small but
  crafted package (nested generic instantiations, doubled string constants)
  is bounded only in the time `Load` waits, not in the memory and CPU of the
  abandoned type check (see `docs/SANDBOX.md`, "CPU, goroutines and cancellation"). A plugin is
  loaded once at a time (`loadSlots`): a new load of the same plugin waits
  for an abandoned one instead of adding one more in the background.

## Values (`vm/value.go`)

One uniform `Value{N int64; R any}` for every interpreted value, no
`reflect.Value` for interpreted data:

| Go value | representation |
|---|---|
| ints, bools, runes, floats | `N` (floats as IEEE bits); integers narrower than 64 bits are kept sign- or zero-extended and cut back after each operation (`OpTrunc`), `int`, `uint` and `uintptr` too on a 32-bit platform (`vm/intsize.go`) |
| string | `R = string` |
| slice, array, struct | `R = []Value` (arrays/structs copied on load/store); a `[]byte` from native code, `make` or `[]byte(s)` is a Go `[]byte` (`bytes.go`); another slice of scalars from native code (`[]string` of an `http.Header`) is a `reflect.Value` view of the native array (`natslice.go`) |
| pointer | `R = *Value` (a cell, or an element of a `[]Value`) |
| interface | `R = *Iface{T *RType, V Value}` |
| func value | `R = *Closure{Fn, Env}` (or a Go intrinsic) |
| map | `R = *MapV` keyed by a comparable Go key derived from the value (`keyOf`) |
| chan | native `chan Value` |
| native value at the boundary | `R = reflect.Value` |

`RType` is the dynamic type descriptor stored in interfaces: the go/types type,
its method set (compiled `*Func`s), and a cached `implements` table.
`Machine.rtypeOf` builds the method set before it publishes the `RType`, so
other goroutines read `Methods` with no lock.

## Compiler (`vm/compile.go`, `vm/features.go`)

- `Func.ensure` compiles a function the first time it is called.
- Each SSA value gets a register; constants are pre-loaded in a register
  template copied into the frame on call. Phi nodes become moves on the
  incoming edges.
- `Instr{Op, T, A, B, C, X}`: 112 opcodes, superinstructions included (arithmetic per kind, memory, calls,
  interfaces, maps, channels, select, defer/recover, range). Rare operations
  are `OpFunc`, a Go closure over the register file.
- Unsupported constructs are compiled lazily into an instruction that raises a
  `CompileError` only if executed (needed to interpret stdlib packages with
  unsupported branches, e.g. complex numbers in fmt). The function keeps the
  list of these instructions (`Func.problems`).
- Load-time check of plugins (`vm/preflight.go`): `vm.Load` compiles, on a
  machine of its own that it then drops, every function the plugin can reach
  from the members of its package (calls, function values and closures in
  every branch, except under a constant false condition; the methods of the
  types converted to an interface). A problem in the plugin's own code (its
  module and vendored modules) fails the load with a `*PreflightError` that
  lists all of them with their positions: a native symbol missing from the
  registry, a function written in assembly, `syscall` outside the read-only
  subset of a restricted plugin with `useUnsafe`,
  an unsupported construct. Problems in standard library code run in the VM
  are not reported (the plugin author cannot fix them). `Options.NoPreflight`
  turns the check off. The machine of the plugin still compiles each
  function at its first call.
- Optimizations behind `VM_OPT` (`vm/opts.go`; `VM_OPT=none` disables all,
  `VM_OPT=-name,...` some): `pool` (pooled execution contexts), `tuple`
  (multi-value results in consecutive registers), `fastnative` (direct native
  call trampolines, `natfast.go` and `zz_natsigs_go1_NN.go`), `memo` (memoized derived VM-reflect types),
  `bytes` (zero-copy `[]byte`), `super` (superinstructions: address+load/store,
  compare+branch, fall-through), `switch` (a chain of `x == constant` tests
  as one jump table, and the last phi move of an edge with its jump,
  `switches.go`), `boxes` (one shared box per type for
  small integers, bools and the empty string in an interface,
  `boxes.go`), `iterpool` (the iterators of range loops over maps and
  strings go back to a pool at the end of the loop), `escape` (a local
  whose address cannot outlive the run of its function keeps one cell per
  frame, `escape.go`).

## Execution (`vm/exec.go`)

- `ctx` is one thread of interpreted execution: a register stack, an explicit
  frame stack, defers. Interpreted calls push a frame; there is no Go recursion
  for interpreted→interpreted calls.
- A new `ctx` (a call from the host, a callback of native code, a goroutine)
  has no register stack: its first run makes one from the frame of the entry
  function (at least 64 Values), and a call that needs more doubles it
  (`grow`). Frames keep offsets into the stack, never pointers or slices: the
  main loop slices its registers again after a call or a return. The ctx
  pool keeps stacks of 4096 Values at most.
- The main loop is a `switch` on `Op`. A `recover` in `protected` turns Go
  panics (interpreted `panic`, runtime errors, native panics) into an unwind
  loop over the explicit frames, running defers; `recoverPC` resumes a frame
  whose deferred call recovered. As in gc, `recover` stops a panic only in
  a function called directly by a deferred call: method wrappers made by
  ssa (bound methods, thunks) do not count as callers, `defer recover()`
  works in such a function, and reflect method values and `MakeFunc`
  functions get the panic when they are deferred (`Closure.Direct`,
  `Closure.GoP`).
- Frames keep their registers until they return, and returned frames stay
  on the register stack until a call writes over them. An interpreted
  `runtime.GC()` (`OpGC`, `gclive.go`) first clears, in the frames of its
  goroutine, the registers whose SSA values are dead at that point (liveness
  computed from SSA on the first GC), and the stack above the top frame, so
  that finalizers of unreachable objects run. A frame that waits for an
  interpreted call does not keep the arguments of that call: the callee
  owns them. When the GC runs inside a deferred call, the frames of the
  ctx that runs the defers are cleared too (a deferred call runs on its own
  ctx, linked by `ctx.parent`); a panicking frame keeps only the values of
  its Recover block. `runtime.KeepAlive` compiles to no code (the SSA
  liveness already keeps its argument), and arrays of plain values have
  no zero template, so a large array is not kept by its allocation site.
  Normal execution pays nothing.
- Goroutines run on new `ctx`s in Go goroutines; channels are native channels
  of `Value`; `select` uses `reflect.Select`.
- Nested runs (`nest.go`): a call from native code back into the VM
  (`Machine.Call`) and a deferred call start a new `ctx` under Go frames.
  The VM keeps the innermost `ctx` of each goroutine (a table keyed by the
  runtime goroutine pointer, `getg_*.s`): the new `ctx` continues its depth
  (`MaxDepth`) and its budget, and `Machine.MaxNest` bounds the number of
  nested runs, so a recursion through callbacks stops with
  `ErrStackOverflow` before the 1 GB Go stack limit.
- Sandbox hooks: `Machine.Budget` (calls + backward jumps per call from
  the host, shared with the goroutines of the call, `ErrBudget`;
  `Options.Budget` for plugins; cancellation by the context of the call
  (`Options.Cancel`, off by default; always for init), `ErrCanceled`; `Machine.MaxGoroutines`, `ErrTooManyGoroutines`; `Machine.BackgroundRate` (a token bucket for the background goroutines);
  `budget.go`), `Machine.MaxDepth`
  and `Machine.MaxStack` (`ErrStackOverflow`; 100000 frames for plugins,
  2M frames and 256 MB of registers for `RunMain` programs), `Machine.GoPanic` (panics of the program's goroutines
  instead of exiting, and of the callbacks that native code runs on its own
  goroutines, `nativego.go`), `Machine.SetEnv` (own copy of the environment,
  `env.go`) and `Machine.SetStdio` (own standard output and error,
  `stdio.go`: `fmt.Print*` (also the hooks of the AOT copy of fmt,
  `Machine.hookOverrides`), the plugin's `os.Stdin`, `os.Stdout` and
  `os.Stderr`, the default logger of `log`, `print` and `println`;
  `Options.Stdout` and `Options.Stderr` for plugins, nil means the host
  streams; every plugin machine gets it, so a plugin never changes these
  host globals). Load-time restrictions: `restrict.go`; the refused methods
  that a plugin calls without naming them (reflect, interfaces, templates)
  are refused at run time (`refusedmeth.go`); per-plugin registries, HTTP
  defaults and `net.DefaultResolver`: `hostregs.go`; the writes of a
  restricted plugin to host objects that stdlib variables reach are refused
  at run time (`hostmem.go`); `Options.PanicOnFatal` turns the fatal
  errors that the VM sees first into panics (`fatal.go`), and
  `Options.MaxMemory` counts the large allocations of the VM
  (`memquota.go`); study: `docs/SANDBOX.md`.
- A panic that leaves the VM for Go code keeps its `*VMPanic` wrapper,
  except for the native sentinels that host code compares by identity
  (`hostPanic` in `panics.go`: `http.ErrAbortHandler`), which go on as
  themselves. net/http then aborts the response without a log.
- `VM_TRACE=1` prints the Go stack of VM panics; `VM_PROFILE=1` counts
  executed instructions per opcode and per function (`prof.go`).

## Stack introspection (`vm/callers.go`)

`runtime.Caller`, `runtime.Callers`, `runtime.CallersFrames`,
`runtime.Stack`, `runtime/debug.Stack` and `debug.PrintStack`, called by
interpreted code, see the interpreted frames with the file:line of gc
(`//line` directives included), not the Go frames of the VM. The
traceback of an unrecovered panic (`PanicError.Trace`, printed by `yaegi`)
uses the same walk: the first context of the goroutine keeps the stack of
the last panic (`ctx.ptrace`, taken when a context catches a new panic;
a panic that leaves a nested context to the one below keeps its stack),
so a panic that native code recovered (fmt and a `String` method) does
not leak into a later traceback (`TestTraceback`, `testdata/traceback`).
Function names follow gc: `init.0` for the first init function (ssa's
`init#1`), and the package initializer is not shown below them;
`runtime.main` is the PC where it calls `main.main` (taken when
`RunMain` starts under the host's `main.main`).

- Positions: the compiler records the source position of each SSA
  instruction in `Func.pos`, one entry per change of position (a load or
  store fused with its address takes the position of the address, the
  deferred calls of a return take the position of the `return` or of the
  closing brace). The cost is paid at compile time only.
- Frames: the walk reads the current frame of a context (`ctx.fn`,
  `ctx.at`, the instruction it runs) and the caller frames (`frame.pc`, the
  return index). A context links to the context below it on the goroutine
  (`ctx.parent`): the function that deferred its call, or the interpreted
  caller of the native code that calls back into the VM (the per-goroutine
  record of `nest.go`); the native frames between them are not shown. During a panic, the
  frames that the panic has left stay visible (`ctx.unwound`) with a
  `runtime.gopanic` frame above them, as in gc, until a recover. The main
  goroutine of `RunMain` ends with `runtime.main` and `runtime.goexit`, a
  goroutine with `runtime.goexit`. Method wrappers made by ssa are not
  shown, like gc's.
- Program counters: an interpreted frame gets a made-up PC (a tag bit, a
  number per function, the instruction index), not an address: it cannot
  be turned into a pointer. `runtime.FuncForPC` decodes it and returns a
  stand-in `*runtime.Func` per line (the funcinl layout of `funcpc.go`,
  whose `FileLine` ignores its argument). `runtime.CallersFrames` returns a
  real `*runtime.Frames` whose frames are already expanded (a layout
  checked at start-up, like funcinl): its own `Next` returns them. The
  runtime frames that programs look for (`runtime.Callers`,
  `runtime.gopanic`, `runtime.main`, `runtime.goexit`) are real PCs of the
  runtime.
- The position of a panic: `ctx.at` must follow each instruction, so that a
  panic knows the instruction that panicked. `exec` stores it at each step
  only when the machine tracks (`Machine.track`): when the program prints
  its tracebacks (`RunMain`) or when an interpreted package imports
  `runtime` or `runtime/debug`. Other plugins pay one predicted branch per
  step (W1 about 1% slower, within the noise of W2 and W3). Without
  tracking, `ctx.at` is still right at the introspection calls and at the
  deferred calls of a return. (A generic `exec` compiled twice, with and
  without the store, measured slower: 5% on W1.)
- Limits: the walk stops at a call from the host (a plugin's `ServeHTTP`).
  Calls through a function value (`f := runtime.Caller`) and native callers
  of these functions (`log` with `Lshortfile`, called natively) still see
  the VM's Go frames. `runtime.Stack(buf, true)` shows the current
  goroutine only; frames print `(...)` for their arguments, and goroutines
  have no "created by" line.

## Native boundary (`vm/native.go`, `vm/natfast.go`)

- Native functions, types and globals come from a small hand-written registry
  plus `stdsyms`, the restricted symbol table of the standard library
  generated for the host Go release by `cmd/stdsyms` with yaegi v2's copy of
  yaegi's extractor (`internal/extract`).
  The packages whose init changes the global state of the host (expvar,
  net/http/pprof, testing/quick) are in `stdsyms/sideeffect`, which only
  hosts that want them import (`cmd/yaegi` does): importing `vm` registers
  nothing on `http.DefaultServeMux` or `flag.CommandLine`.
- Calls go through `reflect.Value.Call` with per-argument conversion
  (`toNative` / `fromNative`), except the signatures that have a
  trampoline: a direct Go call that reads the VM registers without reflect.
  `zz_natsigs_go1_NN.go` (one per Go release of the window) is generated by `natsigs_gen.go` (`go generate ./vm`, about
  1000 signatures): the functions of the standard library with at most 8
  parameters of basic types, `[]byte` (shared), `[]string` and `...string`
  (copied, and the strings that native code changed are written back, as
  `sort.Strings` needs), or `error`, `io.Reader`, `io.Writer` and
  `context.Context` holding a value of a native type; their results may be
  of any type (converted by `fromNative`, as on the general path). In the
  packages plugins use most (strings, bytes, regexp, time, net/url,
  net/http...) also the methods and the named types (`time.Duration`,
  `time.Time`, `*url.URL`). A trampoline takes the general path when an
  argument is not a native value (an interpreted form of the type, an
  interpreted implementation of the interface, a nil pointer), and the call
  sites that the general path treats in a special way (decoders, printers,
  `sync.Pool.Put`) have none. `natfast.go` hand-writes a few more (`http.Error`,
  `io.ReadAll`, `fmt.Sprintf` with arguments of basic types, `http.Header`
  and `sync` methods).
- A call of a method through an interface (`ctx.Err()`) has a generated
  direct call too: `natsigs_gen.go` writes `genFastInvoke`, one case for
  each method of the exported interfaces of the standard library (and
  `error`) whose signature it supports, 145 methods. The case asserts the
  value to an interface with only that method, then calls it: no reflect.
  The call site finds its case once, by the name and the signature of the
  method (`invokeKey`). Before the direct call, `natInvokeOK` checks the
  dynamic type: a restricted machine that replaces the method (refused,
  host or restricted methods: `(*os.File).Chdir`, `(*http.Client).Do`,
  `(*log.Logger).Fatal`) and the methods that the general path handles in
  a special way (channel readers, print and decode methods) take the
  general path. The answer is kept for the first native dynamic type of
  the site. `TestNatInvokeOK` and `TestGenInvokeReplaced` check this.
  - When it applies: only when the interface value holds a native type
    descriptor (`Iface.T.Native`), that is, when native code made the
    value: a result, a field or a parameter of interface type
    (`req.Context()`, `req.Body`), or a type assertion from such a value.
    A native value that the plugin puts in an interface itself
    (`var l interface{ Len() int } = strings.NewReader("abc")`, or
    `var w io.Writer = &buf`) gets the interpreted descriptor of its static
    type: the call takes `callBodyless`, then reflect, as before.
- Pointers to interpreted memory are copied in and copied back after the call
  (breaks if native code retains them); the conversions of one call share a
  map of converted pointers, so cycles map to cycles and the write-back keeps
  pointer identity. Slices and maps passed with their static type are copied
  back too. Maps and slices keep their identity: the native copy of an
  interpreted map is written back into the same map (every alias sees what
  native code put in it, `json.Unmarshal(data, &m)` on a non nil map), and
  the native copy of a slice has the capacity of the interpreted one and
  converts back to its array (an append in place reaches it). This holds for
  maps and slices reached through pointers, struct fields and elements, for
  the AOT shadows of cells (`shadowRef`) and for `Unbox` in AOT code. Structs and maps of native named types declared in interpreted
  code live in native memory (`strings.Builder`, `http.Header{}`). A slice
  of scalars (strings, numbers, bools) read from native memory is not
  copied: it stays a view of the native array (`natslice.go`), so
  `req.Header[k][i] = v` and `for _, vs := range req.Header { vs[i] = ... }`
  write into the header, as in Go. The other way is still a copy: a slice
  made by interpreted code and stored into native memory
  (`h["A"] = vs`) is a new native slice, with len = cap; later writes
  through `vs` do not reach it (writes through `h["A"]` do). A pointer
  stored by native code through `any` (container/list, context) comes back as
  the original interpreted pointer. A value that is not a pointer (a struct,
  array, slice, map or basic value of a plugin type: a generic
  instantiation, a local or unexported type) comes back as the original
  interpreted value too, with its type (`sync.Pool{New: func() any { return
  Box[int]{} }}`): its native copy is put in an interface whose data word is
  remembered (`rememberValue`, found again by `ifaceVMs`); Go never changes
  the value inside an interface. All zero-size values share one word: a
  value of a zero-size plugin type goes to native code as an opaque
  handle, one per type and machine (`zeroSizeHandle`), which comes back as
  the plugin value with its type. An unnamed struct from native code takes
  the program's type of the same shape.
  Pointer-shaped structs and arrays are their own word and are not
  remembered; a native map that the plugin holds in a map type of its own
  is shared with native code and is not remembered either (it comes back
  with the unnamed type: Go cannot make a second type for the same map).
- An interpreted channel (a `chan Value`) that native code gets in an
  interface, directly or in a field, element or map value
  (`json.Marshal(ch)`, `fmt.Sprint(s)`, a context value) is a native
  stand-in (`chanstandin.go`): a closed native channel of the same type
  and capacity, one per channel and type. Native code sees the right kind
  (json, xml and gob report their own error, `json:"-"` skips the field),
  and a stand-in that it gives back converts back to the interpreted
  channel. A native send or receive on the stand-in does not reach the
  interpreted channel (a send panics, a receive gets the zero value), so a
  channel passed as a typed parameter (`signal.Notify(c, ...)`) is still
  refused, and so is a channel in the data of text/template and
  html/template (`{{range .C}}`, `{{len .C}}` read the channel: an empty
  stand-in would give a wrong result with no error). While their Execute
  runs on a goroutine, no channel converted on that goroutine gets a
  stand-in: the data, and what a FuncMap func or `{{call .F}}` returns.
  Execute through the plugin's reflect (`MethodByName`, `Type.Method`) is
  a marked func, and its callers set the rule before they convert the
  data.
  The cleanups of the stand-ins never reach the Machine: a cleanup is a GC
  root while its object lives, and a plugin global channel lives as long
  as the Machine. They hold the tables and the channel type only weakly.
- Native types are built lazily, at the boundary: interface types with
  methods become `any`, and in fields of recursive structs the references
  back to the struct are cut (`[]*T` becomes `[]any`, `*T` becomes `any`).
  A recursive named type that is not a struct is cut the same way, in the
  parts of its underlying type that refer back to it (`cutNamed`):
  `type M map[string]M` becomes `map[string]any`, `type T []T` `[]any`,
  `type A [1]*A` `[1]any`. Native values that contain themselves keep their
  cycle when they come back into the VM (`natSeen`).
  A native decoder fills such an `any` with its own value (`map[string]any`):
  `json.Unmarshal` (default mode) and `Object.Decode` therefore run the
  decoder again on a type unrolled as deep as the data, until no cut is
  filled (`recursive.go`). reflect never frees these types: they depend only
  on the static type and the depth (uniform types), or, when those are too
  large, on the fields the data uses, under one budget for the process. Other
  native decoders (`json.Decoder.Decode`, yaml) still see the cut type, and
  the conversion back fails with a clear message.
- The reflect wall: native code cannot see interpreted methods. An
  interpreted value passed as a native interface is boxed in yaegi's
  generated wrapper for that interface (`wrappers.go`, e.g. `_flag_Value`,
  func fields built with `reflect.MakeFunc`); passed as `any`, it is boxed in
  a hand-written wrapper chosen by method set (`stringerW`, `errorW`,
  `errStringerW`, `handlerW`, `jsonMarshalerW`). These implement
  `fmt.Formatter` and `json.Marshaler`, so native fmt and encoding/json print
  the interpreted value, not the wrapper. Go cannot create named types at
  run time, so `%T`, `%#v` and nested `String` methods cannot be right in
  native fmt: when the AOT copies are compiled into the host (`aot/all`),
  a call of a fmt print function (also `Errorf`: the error of the copy wraps
  the native errors of `%w` as they are), of `json.Marshal`,
  `MarshalIndent`, `Unmarshal`, `Encoder.Encode`, `Decoder.Decode`
  (`aotroutejson.go`), or of `xml.Marshal`, `MarshalIndent` or `Unmarshal`
  whose `any` arguments hold an interpreted type goes to the AOT copy,
  which sees the interpreted types through the twin reflect
  (`aotroute.go`). Other calls stay native, and so do calls whose values
  hold a type of the package itself (`xml.Name`, `xml.Attr`): the AOT copy
  has its own copies of these types. `json.RawMessage`, `json.Number` and
  `json.Delim` do not count: the copy calls the native methods of
  RawMessage, and takes the host's Number and Delim as its own
  (`sameValType`, in `EqualAny`). A call of json or xml that cannot go to
  the AOT copy (no copy in the host, the package forced to native mode, a
  value that holds `xml.Name`, the xml `Encoder` and `Decoder`) while
  native code would not call a method of an interpreted type inside the
  value (`UnmarshalJSON`, `MarshalText`, `UnmarshalXML`... of a field, an
  element, a map key) is never silently wrong (`lostmethods.go`): when the
  type is known at compile time, it is a problem of the load-time check;
  otherwise the call returns a `*vm.LostMethodError`. The routes need the
  twin, but they do not turn on the VM reflect for the program's own
  reflect calls (`progVR`): a plugin (`Load`) without `OpaqueAny` whose
  own packages do not import `reflect` keeps native reflect, with its
  checks on host objects (`TestPluginAOTRoute`). A value of a native type
  that holds interfaces (`[]any`, `map[string]any`, `*any`) goes to the copy
  when it holds an interpreted value, as a native copy whose interfaces hold
  handles (`convGraph.aot`), as in AOT mode. The print functions of `log`
  format their message with the copy of fmt, then give it to the native
  function as one string (`aotroutelog.go`).
  A native interface with unexported methods (`ast.Expr`, `ast.Decl`,
  `constraint.Expr`, the options of `encoding/json/v2`) cannot hold a value
  of the program, even one that embeds a native node and so has the
  promoted method: Go cannot make a native type with that method at run
  time. The conversion stops with a clear `unsupported` error that names
  the interface and the method (`unexportedIfaceError`); go/ast, go/printer and go/doc see only native nodes.
  The standard library looks for methods beyond the interface of a value
  (`x.(io.WriterTo)`, `IsBoolFlag`, `Unwrap`, `Timeout`, `crypto.Signer`):
  docs/BOUNDARY.md lists every such place. A plugin value given as a native
  interface, or as `any` when no hand-written wrapper fits, gets the wrapper
  of a family of `optwrap.go` (io.Reader, io.Writer, error, flag.Value,
  rand.Source, fs.FS, fs.File, context.Context, http.RoundTripper,
  image.Image, crypto.Signer, crypto.Decrypter, net.Conn): one static type
  per combination of its optional methods (`zz_optcombos.go`, made by
  `go generate ./vm`), so the wrapper has exactly the optional methods of
  the type, as in Go. The wrapper compares as the plugin value with `==`
  (a non-comparable type panics, `optKey`). Other values that combine
  methods beyond one wrapper lose the extra methods,
  except `http.ResponseWriter`
  (`rwwriter.go`): a plugin type passed as a ResponseWriter is boxed in a
  static wrapper type that has exactly its optional methods among Flush,
  Hijack, ReadFrom, Push, CloseNotify, Unwrap and the methods of
  `http.ResponseController` (one type per combination, `zz_rwcombos.go`,
  made by `go generate ./vm`), so `w.(http.Flusher)` and
  `w.(http.Hijacker)` work in native code. To keep the binary small, a type
  with ReadFrom, Push or CloseNotify and only some of the four
  `http.ResponseController` methods gets all four (`rwComboMask`): the
  result through `http.ResponseController` is still the one of Go. Closures go through
  `reflect.MakeFunc`; such a func that comes back to the VM, even long after
  (a func field of a struct kept in an `atomic.Value`), is the closure again,
  when the func type is the same (`noteClosureFunc`, `closureOfFunc`).
- `unsafe` is emulated on the value model (`unsafe.go`, `docs/UNSAFE.md`); string and slice
  headers are views of the variable (`unsafehdr.go`).
- `OpaqueAny`: interpreted values passed to native `any` parameters become
  opaque handles that round-trip unchanged (sync.Pool, sync.Map, context).
  The native printers (`printFuncs` in `native.go`: the fmt print functions,
  `Errorf`, the log functions and `*log.Logger` methods) only read their
  arguments: a value without methods gets a native copy, as without
  `OpaqueAny`, because fmt prints `%T` with the type of the handle. This copy is bounded (`printCopyMax` values): a larger
  value keeps its handle (`%T` then prints `vm.vmOpaque`), so that a plugin
  cannot make the host copy a large value for each `Printf("%T", v)`.
- Interfaces nested in interfaces are copied for native code on the host
  stack: deeper than `maxBoxDepth` (100000, and 25000 on 32-bit platforms,
  whose goroutine stacks are 4 times smaller), box gives an opaque handle
  instead of a fatal Go stack overflow of the host. Native code may walk
  `deepSegs` such bounds in one value (200000 levels, 125000 on 32-bit).
- AOT copies walk a value through the twin reflect, once per level on the
  host stack. fmt and json are bounded before the walk (`vm/deepprint.go`);
  for the others (yaml, toml, asn1, go-cmp, templates...), `RV.Elem`
  panics when the goroutine stack is almost full (`vm/stackguard.go`, 508
  MiB of the 512 MiB that gc allows, 127 of 128 MiB on 32-bit platforms;
  the limit of `runtime/debug.SetMaxStack` set by the host is read again at
  each `NewMachine`): the plugin gets a panic it can recover instead of a
  fatal stack overflow of the host. The guard reads the stack bounds from
  the goroutine structure; a start-up check turns it off if they look
  wrong, and a read whose bounds do not hold the stack pointer counts as no
  use.

## VM-internal reflect (`vm/vmreflect.go`, `vm/vreflect.go`, `vm/intrinsics.go`)

- When `reflect`-using stdlib packages are interpreted from source (`-src`),
  their calls to `reflect` / `internal/reflectlite` are routed to intrinsics
  that operate on VM values and go/types types, so interpreted types keep
  their identity and methods. Only the ~83 entry points used by
  encoding/json, fmt, errors, sort and internal/fmtsort exist (listed by
  `cmd/reflscan`); enabled with `OpaqueAny`, and by `RunMain` and `Load`
  when one of the program's own packages (main or the plugin, and the
  non-standard packages) imports `reflect`, even if it is not the package
  that defines the types (native reflect cannot see interpreted methods:
  `Type.Method`, `MethodByName`, `MakeFunc` need the VM reflect; and it
  cannot make named types: a result of `Value.Call` would come back as
  `func() int` instead of `main.Builder`). The twin also exists when calls can go to AOT copies
  (`aotroute.go`), without changing the program's own reflect. Native
  values (host objects such as a `*http.Request`) keep their native data
  and a native descriptor, not a copy, and the calls that get one, or a
  value whose cell holds native data, go to the twin
  (`vmreflect_nat.go`): writes reach the host object, and native reflect
  keeps its checks (`Set` of an unexported field panics). A `reflect.Type`
  or `reflect.Value` passed to a native function becomes a real one.
  `runtime.FuncForPC` of the `Pointer` of an interpreted function gives a
  stand-in `*runtime.Func` with the runtime's layout for inlined functions,
  so its methods give the gc-style name from any caller (`funcpc.go`).
- `intrinsics.go` implements bodyless functions of internal packages
  (`internal/bytealg`, `abi.NoEscape`).
- Generic functions and types of native standard library packages have no
  native instance for the types of a plugin. The VM interprets the
  embedded sources of `cmp`, `iter`, `slices` and `maps`; `generics.go`
  implements the others, with every loader and whatever the other imports
  are (their source needs runtime internals): `sync/atomic.Pointer[T]` (the
  unsafe.Pointer hooks of `atomic.go` on the field cell: same pointer
  identity, real atomicity), `math/rand/v2.N`, `weak.Make` and
  `weak.Pointer.Value` (a native weak pointer to the VM cell),
  `runtime.AddCleanup` (the cleanup runs as a goroutine of the program:
  its panics, ErrBudget and ErrStackOverflow go to GoPanic),
  `hash/maphash.Comparable` and `WriteComparable`
  (the hash of the VM map key), `crypto/hkdf` and `crypto/pbkdf2` (the
  native instance for `hash.Hash`), `database/sql.Null[T]` (native
  instances for the basic types, `time.Time`, `[]byte` and `any`, and the
  named types without methods over them), `iter.Pull`, `unique.Make`,
  `sync.OnceValue(s)` (a panic of f is raised again on every call, as in
  Go) and `errors.AsType`. `reflect.TypeFor` and
  `reflect.TypeAssert` are in the VM reflect. Still unsupported:
  `sql.Null[T]` for other T (a struct, a type with `Scan` or `Value`
  methods), and an `atomic.Pointer` that lives in native memory.
- The reflect twin (`twin_type.go`, `twin_value.go`, `twin_func.go`,
  `twin_iter.go`, facade package `vreflect/`) exports the same model as
  the API of `reflect` (`vm.RV`, `vm.RT`...), over both VM and native values.
  `cmd/aotgen` copies reflect-using packages (stdlib and third-party) under
  `aot/` with reflect replaced by the facade and generates their glue; they
  are compiled into the host and run in `ModeAOT` (`vm/aot.go`: per import
  path modes native / aot / interp, `yaegi -aot`, `yaegi -mode`). This is the
  "C′" configuration of `docs/BENCHMARKS.md`; see `docs/AOTGEN.md`.
  `twin_bridge.go` converts the interpreted program's own reflect values
  (VM intrinsics above) to the twin's and back at that boundary.

## Tools

- `cmd/yaegi`: run a program. `cmd/runtests`: conformance runner over
  `tests/yaegi/_test`. `cmd/aotgen`: AOT generator; `cmd/aotcheck`: AOT
  programs of `testdata/aot` in every mode against `go run`. `compare/bench` + `plugins/` + `compare/bench/run.sh`: W1/W2/W3 plugin
  benchmarks against native and yaegi. `cmd/stdscan`: what interpreting the
  stdlib from source requires. `cmd/reflscan`: reflect API used by packages.
