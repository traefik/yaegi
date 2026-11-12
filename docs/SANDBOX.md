# Plugin isolation in yaegi v2: guard rails, not a sandbox

This document explains what yaegi v2 does to protect a host program (such as
Traefik) from the plugins it runs, and what it does not do. The open work is in
`TODO.md`.

## In short

- **yaegi v2 is not a sandbox, and nothing in it is a security boundary.** The
  plugin runs in the host process and shares its memory, its heap and its
  global variables. No Go interpreter that runs inside the host process can be
  a boundary. The yaegi maintainers say the same: "Complete sandboxing seems to
  be out of reach" and "The only fully safe option would be to isolate the
  untrusted code in a separate process" (upstream-issue: 486).
- **A plugin written to harm the host can do it.** Go data races can corrupt
  memory without `unsafe`, native code reached by the plugin is not stopped and
  may be misused, a blocking native call cannot be interrupted, and a fatal
  runtime error ends the process.
- **What yaegi v2 offers is guard rails.** Restriction levels, a CPU budget, a
  goroutine cap, a memory quota, an environment and process-wide registries of
  its own for each plugin, panics kept away from the host, and checks of the
  loaded code. They stop a buggy or clumsy plugin, and they make obvious
  misuse harder. They do not stop a determined attacker.
- **`Options.Sandbox` is a best effort against accidents, not a boundary.** Its
  capability checks cover a subset of the `os` and `net` functions. Do not use
  it to run untrusted code.
- **Untrusted code needs process isolation**: a separate process with its own
  limits (seccomp, Landlock, namespaces, cgroups), or a runtime with its own
  memory space. For Traefik, the boundary for untrusted code is the Wasm
  plugins, or a separate Traefik instance for the routes that use such plugins.

## Restriction levels

`vm.Load` runs a plugin at one of these levels:

- **Restricted** (the default). The plugin gets a restricted standard library:
  - imports that act on the whole process are refused at load time: `os/exec`,
    `plugin`, `unsafe` and `syscall` (without `Options.UseUnsafe`), the stdlib
    `internal/` packages, `go/build`, `go/importer`, `go/types`,
    `net/http/pprof`, the test driver of `testing`, and a few others;
  - functions that change the process are refused at load time: starting a
    program (`os.StartProcess`, `net/http/cgi`), signals to other processes,
    the current directory, runtime settings (`debug.Set*`), the signal
    handling of the host, the CPU profile and trace of the process;
  - methods that are refused can also be reached without naming them (reflect,
    interfaces, templates): they are refused at run time too
    (`vm/refusedmeth.go`);
  - `os.Exit`, `log.Fatal` and the `Fatal` methods of loggers panic instead of
    ending the process, also when they are reached through standard library
    code (`flag`, `log`, templates; `vm/exit.go`);
  - `runtime.GOMAXPROCS(n)` with `n > 0` changes nothing;
  - writes to the variables of the standard library and to the objects they
    reach are refused (see "Host memory").
- **`Options.UseUnsafe`**: `unsafe` is emulated on the values of the VM
  (`docs/UNSAFE.md`), and `syscall` and `golang.org/x/sys/unix` give a
  read-only subset (see "The `syscall` subset").
- **`Options.Unrestricted`** lifts the load-time refusals and gives the plugin
  the environment of the process. `os.Exit` and `log.Fatal` still panic.
- **`Options.RealProcess`**, with `Unrestricted`, gives the standard library as
  in Go: `os.Exit` ends the process, `os/exec` and `syscall` work. For trusted
  code only.
- **`Options.Exec`** gives `os/exec` alone: `ExecMock` lets the plugin load but
  starts no process, `ExecReal` runs real programs (trusted code only).

A `Sandbox` always keeps the restrictions, whatever `Unrestricted` and
`RealProcess` say.

`Options.StubAssembly` removes no protection: a plugin that reaches a function
without a Go body (assembly, `//go:linkname`) loads, and a call of such a
function panics or returns `ENOSYS`. The VM never runs assembly.

## Safe defaults

The zero `Options` set no limit, as Go has none: no `Budget`, no
`BackgroundRate`, no `MaxGoroutines`, no `MaxMemory`, no `LoadTimeout`, and
`PanicOnFatal` is off. Only `MaxDepth` (100000 frames) and `MaxStack` (1 GiB per
goroutine) have a default.

A host that loads plugins it does not trust should use package `host`:
`host.Options()` sets the limits, and `host.Check` (called by `host.Load`)
refuses an unsafe combination (`Unrestricted`, `RealProcess`, `ExecReal`, a
zero limit, a nil `Env`, `PanicOnFatal` off...). The values are: AOT mode,
`Budget` 1e8, `BackgroundRate` 5e7, `MaxGoroutines` 1000, `MaxMemory` 1 GiB,
`MaxStack` 256 MiB, `PanicOnFatal`, `LoadTimeout` 30 s, and an empty
environment. On the plugins of the Traefik catalog, these limits give the same
output as the zero `Options` (`docs/CATALOG.md`).

`Options.RecoverHandler` makes `ServeHTTP` recover a panic of the plugin: the
panic goes to `Options.GoPanic` and the response gets the status 500.

## The environment of a plugin

Each plugin has its own copy of the environment (`vm/env.go`). `os.Getenv`,
`LookupEnv`, `Environ`, `ExpandEnv`, `Setenv`, `Unsetenv` and `Clearenv` work on
that copy, so a plugin cannot change the environment of the host or of another
plugin.

- A nil `Options.Env` is a copy of the whole host environment; an empty slice
  is no variable.
- `internal/pluginhost` sets an empty environment by default. A host passes
  the environment of a plugin itself: `pluginhost.HostEnv()` for the whole host
  environment (what Traefik gives a yaegi plugin), or
  `pluginhost.EnvFilter{Names, Prefixes}.Env()` for only the listed variables,
  as `settings.envs` of the Traefik Wasm plugins.
- The copy has a lock, `os.Setenv` refuses a key with `=` or a NUL byte as Go
  does, and `os.Environ` is sorted.

The copy protects the host environment from changes. It does not hide
secrets: the plugin shares the process of the host, and some native code of
the standard library reads the environment of the process itself
(`http.ProxyFromEnvironment`, `os.UserHomeDir`, `os.TempDir`...). Do not put
secrets in the environment of a host that runs plugins you do not trust.

## Process-wide registries

In Go, some functions of the standard library write to a table of the process.
Each machine without `RealProcess` has its own copy of these tables, made at the
first use (`vm/hostregs.go`, `TestPluginHostRegistries`):

| Registry | Per machine |
|---|---|
| `net/http` | `DefaultServeMux`, `DefaultClient`, `DefaultTransport` (a clone of the host's), and the functions that use them; a nil handler means the plugin's mux |
| `expvar` | the table of variables; it starts empty for a restricted plugin |
| `database/sql` | the drivers |
| `mime` | the extension types |
| `image` | the formats |
| `net/rpc` | `DefaultServer` |
| `crypto` | `RegisterHash` |
| `crypto/x509` | `SetFallbackRoots` |
| `archive/zip` | the compressors and decompressors |
| `log/slog` | the default logger and its level |
| `flag`, `os` | `flag.CommandLine`, `flag.Usage`, `os.Args` |
| `net` | `DefaultResolver` (restricted machines) |

So a plugin cannot add routes to the host's `http.DefaultServeMux`, change the
TLS settings of the host's default transport, or replace a hash function of
the host.

Limits: native code that the plugin calls uses the host's tables, the names of
`gob.Register` are shared by the process (`vm/gob.go`), and `RealProcess`
machines keep the host's tables.

A host that imports `stdsyms/sideeffect` (`cmd/yaegi` does) links
`net/http/pprof` and `expvar`, which register `/debug/pprof/` and `/debug/vars`
on the host's `http.DefaultServeMux`. A host that imports only `vm` does not get
them (`TestHostImport`).

## Host memory

The variables of the standard library run natively: they and the objects they
point to belong to the host. A restricted machine checks its writes to native
memory against the set of these objects (`vm/hostmem.go`, `TestHostWrites`,
`TestHostMem`):

- at load time, a write that names a variable of a package not run from
  source (assignment, `&v`, `delete`, `clear`...) is refused with its position,
  except the variables the plugin has its own copy of (see above);
- at run time, a store through a pointer, a field or an element of native
  memory, the writes into a host `[]byte`, atomic stores, map updates and the
  `Set` methods of reflect are checked. A refused write panics with a
  `*vm.RefusedError`, which the plugin can recover.

The identity of the objects does not change: `err == io.EOF` still works.

This check covers the writes of the VM only. Native code writes where it wants,
and with `UseUnsafe` the unsafe views are not checked. It is a guard rail
against accidents, not a memory boundary.

## File descriptors and standard streams

In a restricted plugin:

- `os.NewFile` panics with a `*vm.RefusedError` (`vm/newfile.go`), and
  `net.FileConn`, `net.FileListener` and `net.FilePacketConn` return an error:
  a plugin cannot turn a number into a file of the host;
- the functions that open a path walk it one element at a time and refuse the
  entries of the file system that would reopen a descriptor of the host
  (`vm/hostfd.go`, `TestHostFD`). `/dev/stdout` and `/dev/stderr` open the
  plugin's own streams;
- `os.Stdin`, `os.Stdout` and `os.Stderr` are the plugin's own duplicates of
  the host descriptors, or pipes to `Options.Stdout` and `Options.Stderr` when
  the host sets them (`vm/stdio.go`): closing them does not close the host's.

## `Options.Sandbox`

`Options.Sandbox` (`vm/sandbox.go`) adds capability checks on top of the
restricted level:

- `Env`: the environment of the plugin;
- `ReadPaths`, `WritePaths`: the paths that the checked file functions accept;
- `DialHosts`: the hosts that the checked network functions accept;
- the budget, the goroutine cap and the panic handling.

A refused access returns the sandbox error. The checks cover a subset of the
`os` and `net` functions (the common ones that open, create, remove or rename
files, and `net.Dial`, `net.DialTimeout`). Other functions of the standard
library stay native and are not checked, and the path check is lexical.
`Options.Sandbox` is useful against a plugin that reads a file or dials a host
by mistake. It is not a way to run untrusted code.

## The `syscall` subset

Without `UseUnsafe`, the import of `syscall` is refused. With `UseUnsafe`, a
restricted plugin gets a **read-only subset** of `syscall`
(`vm/unsafesyscall.go`, `TestUnsafeSyscall`). Only `Unrestricted` with
`RealProcess` gives the whole package.

| Name | Why it is allowed |
|---|---|
| `Getpid`, `Getppid`, `Getuid`, `Geteuid`, `Getgid`, `Getegid`, `Getpagesize` | `os` gives the same values |
| `Gettimeofday`, `Uname`, `Statfs` | read the time, the system name and file system statistics; they write only to the value the plugin gives (`Statfs` follows the `Sandbox` read paths) |
| `Getenv`, `Environ` | read the plugin's own environment copy, never the process environment |
| types `Signal`, `Errno`, `Timeval`, `Timespec`, `Utsname`, `Statfs_t`, `RawConn`, `Conn`, and pure conversions | plain data, so that `errors.Is(err, syscall.ECONNREFUSED)` and similar code work |
| `Read`, `Fstatfs`, `Recvfrom` (`MSG_PEEK` only), `SetsockoptInt`, `GetsockoptInt` (a short list of TCP and buffer options) | only on the descriptor that a `RawConn` of the plugin's own connection lends to its callback, on that goroutine and while the callback runs (`vm/rawconn.go`, `TestRawConnSubset`) |

Everything else is refused at load time, with the position and the message
"it is not in the read-only syscall subset of a plugin with
Options.UseUnsafe": what opens, writes, signals, starts processes, maps memory,
makes sockets or raw system calls, or changes the process, and every `syscall`
variable.

**`golang.org/x/sys/unix` from the host.** Plugins import it through gRPC, zap,
wazero and the OpenTelemetry SDK. Run from source, its system calls go through
assembly or the C library, which the VM cannot run. The VM calls the host's
copy instead (`vm/xsys.go`), when the plugin's version is not newer than the
host's and every object it uses has the same type in the host copy (otherwise
the package runs from source, and `Machine.NativeFallback` says why). A
restricted plugin with `UseUnsafe` gets a read-only subset with the same rules
as the `syscall` subset (`vm/xsysunix.go`, `TestXSysSubset`): process and
time information, the environment copy, pure functions, and the socket options
and terminal queries on lent descriptors.

## CPU, goroutines and cancellation

Design of `vm/budget.go`:

- **One budget per call from the host** (init, each `Plugin.Call`, each
  `ServeHTTP`, a callback that native code runs on a goroutine of its own). The
  call and every goroutine it starts while it runs take steps from one account
  (`Options.Budget`). Steps are the calls and backward jumps of the VM. When
  the account is empty, all its goroutines stop with `ErrBudget`.
- **Cancellation**: off by default, as in Go. With `Options.Cancel` (and always
  for the context of `LoadContext` during init), a done context stops the call
  and its goroutines with `ErrCanceled` at their next step, channel wait,
  select or `time.Sleep`. Deferred calls still run. `ServeHTTP` then panics
  with `http.ErrAbortHandler`.
- **Background goroutines**: a goroutine that outlives its call may run
  `Budget` steps between two blocking points. `Options.BackgroundRate` bounds
  the steps per second of all background goroutines of a plugin together.
- **Goroutine cap**: `Options.MaxGoroutines` bounds the live goroutines of the
  plugin. Past it, the `go` statement panics with `ErrTooManyGoroutines`
  (recoverable).
- **Stack**: `Options.MaxStack` bounds the stack of each goroutine, and
  `MaxDepth` its depth. Recursion through native callbacks is bounded by
  `MaxNest`. Past these limits, the host gets `ErrStackOverflow`.
- **Load and init**: `vm.LoadContext` and `Options.LoadTimeout` bound the whole
  load. `Options.LoadLimits` refuses source that is too large, and the loader
  refuses known shapes of source whose types, constants or generic instances
  grow exponentially (`vm/loadbomb.go`, `vm/loadinst.go`). Only one load at a
  time runs per plugin.
- **Close**: `Plugin.Close` stops every goroutine of the plugin at its next
  blocking point (`vm/close.go`, `docs/RELOAD.md`).

Native code is never stopped in the middle: Go cannot preempt a goroutine from
the outside. The budget counts VM steps, not native work.

## Memory

`Options.MaxMemory` (off by default; 1 GiB in `host.Options()`) is an
approximate quota per plugin (`vm/memquota.go`, `TestMaxMemory`). The VM counts
its large allocations while they are alive, the package-level variables of the
plugin, and the stacks of its goroutines. Native calls whose result size is
known from their arguments (`strings.Repeat`, `bytes.Repeat`, `Grow`,
`io.ReadAll`...) are checked before the call (`vm/nativemem.go`). Past the
quota, the allocation panics with `ErrMemory`, which the plugin can recover.

It is a weak guarantee: small allocations and most native allocations are not
counted, and memory is shared by the whole process. The real guard is a cgroup
or `GOMEMLIMIT` for the whole host.

## Fatal runtime errors

The Go runtime ends the process with a "fatal error" that `recover` cannot
stop: concurrent map writes, unlocking an unlocked mutex, out of memory, a
stack overflow in native code, a deadlock of all goroutines. Compiled Go and
yaegi v0 end the same way.

With `Options.PanicOnFatal` (off by default; set by `host.Options()`), the VM
detects some of these errors on memory of the plugin before the runtime does,
and panics with a `*vm.FatalError` that the plugin can recover (`vm/fatal.go`,
`TestPanicOnFatal`). The same errors in native code, or on host objects, still
end the process. A host should run with a restart policy.

## What other engines do

| Engine | Side effects | CPU | Memory | Concurrency |
|---|---|---|---|---|
| Starlark-go | none by default, only what the host predeclares | `SetMaxExecutionSteps` (calls and backward jumps, like yaegi v2), `Thread.Cancel` | no limit | no goroutines in the language |
| goja (JavaScript) | what the host exposes | `Runtime.Interrupt`, no effect on native functions | none | one `Runtime` per goroutine |
| gopher-lua | chosen libraries (`SkipOpenLibs`) | `SetContext` (timeout) | stack and registry size | `LState` not shared |
| Tengo | chosen imports | none | `SetMaxAllocs` (number of objects) | no |
| CEL, expr-lang | none, languages that are not Turing-complete | cost bounded by design | | |
| Deno | no input or output by default, permissions per target | | | recommends seccomp, cgroups, gVisor or Firecracker for untrusted code |
| wazero (Wasm) | nothing by default (`ModuleConfig`) | `WithCloseOnContextDone` | `WithMemoryLimitPages`, isolated linear memory | one instance per module |
| HashiCorp go-plugin, Kong, Envoy `ext_proc` | separate process | the OS's | the OS's | the OS's |

The engines that give a real guarantee do it either by restricting the language
(CEL, Starlark) or by changing the memory space (Wasm, separate process). An
interpreter for full Go, which shares the host runtime, can only offer partial
protection. yaegi uses the same words ("partial sandboxing", upstream-issue:
486).

## Layers of guard rails

None of these layers makes yaegi v2 a security boundary.

1. **Static refusals and host protection** (done): refused imports and
   functions, copied environment, own registries, host memory checks, panics of
   goroutines and native callbacks sent to `GoPanic`, file paths of the loader
   kept inside the module.
2. **Capabilities declared per plugin** (partial): `Options.Sandbox` checks
   the environment, some file functions and `net.Dial`. To do: checks on every
   function that takes a path (with `os.Root`), and a filter of hosts for the
   network, including `net/http`. This needs the declaration in the host (for
   Traefik, in the manifest or the configuration).
3. **CPU** (done): the budget, the background rate and cancellation.
4. **Goroutines** (done): `Options.MaxGoroutines`.
5. **Approximate memory** (done, opt-in): `Options.MaxMemory`.
6. **Out-of-process mode**: not for middlewares (next section). Possible for
   providers, whose interface can be serialized.

The guard rails are checked on each test run by the hostile corpus of the
catalog harness (`compare/catalog/vmhost/testdata/hostile`, `go test
./catalog/vmhost -run TestHostileCorpus` in `compare/`): one plugin per case,
each in a child process, in the default and AOT modes
(`compare/catalog/README.md`).

## No out-of-process mode for middlewares

A middleware plugin is not a protocol: it gets `next http.Handler` and returns
an `http.Handler`, so it can do what Go code can do with `net/http`, and
plugins do it:

- wrap the `http.ResponseWriter` (rewrite-body, caches, compression), on which
  the host and the other middlewares then make type assertions (`http.Flusher`,
  `http.Hijacker`, `http.ResponseController`);
- call `next` zero, one or several times, from another goroutine, or after a
  write;
- keep references: change the request after `next`, store it, read part of the
  body and let `next` read the rest;
- read what the host puts in the request (`req.TLS`, the values of the
  context, `RemoteAddr`), hijack the connection (WebSocket), set trailers, see
  the cancellation of the request.

Across processes, each `Header().Set`, `Write` or `Flush` becomes a remote call
or is batched (the order and the timing of the writes change), and the shared
pointers no longer exist. Running the existing plugins out of process needs a
layer that imitates `net/http` across the boundary: an approximation, with
differences of its own. A phase-based interface (request, then response) can
be serialized, but it is another plugin interface: it exists, and it is the
Wasm plugins (http-wasm), isolated by design.

So yaegi v2 keeps the middlewares in the host process, with guard rails, and
does not promise a boundary. For untrusted code: Wasm plugins, or a separate
host instance for the routes that use such plugins.

## Sources

- upstream-issue: 486, https://github.com/traefik/yaegi/issues/486
- upstream-issue: 901, https://github.com/traefik/yaegi/issues/901
- https://github.com/google/starlark-go
- https://github.com/dop251/goja
- https://github.com/yuin/gopher-lua
- https://github.com/d5/tengo
- https://github.com/google/cel-go
- https://github.com/expr-lang/expr
- https://docs.deno.com/runtime/fundamentals/security/
- https://github.com/tetratelabs/wazero
- https://go.dev/ref/mem
- https://docs.kernel.org/userspace-api/seccomp_filter.html
- https://docs.kernel.org/userspace-api/landlock.html
- https://github.com/hashicorp/go-plugin
