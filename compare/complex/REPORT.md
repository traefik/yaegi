# Complex plugins: results

20 plugins of [README.md](README.md), run on darwin/arm64 (Go 1.26.8) with the
middleware scenario of the catalog harness, `useUnsafe: true` in every
manifest. The reference is a native build of each plugin, run twice. The
column v2 is an experimental second engine, measured on a work branch at
ee82dcf (loaded with `vm.Options.Engine = "v2"` by the vmhost of that
branch with an `-engine` flag). It is not part of this tree.

## Summary

| engine | identical to native | fails at load | fails in New or a request | other output |
|---|---|---|---|---|
| yaegi v0.16.1 | 0 | 20 | 0 | 0 |
| yaegi v2 (v1) | 10 | 10 | 0 | 0 |
| yaegi v2 (v1) -aot | 9 | 10 | 1 | 0 |
| yaegi v2 v2 | 10 | 8 | 2 | 0 |

yaegi loads none of these plugins. yaegi v2 runs protobuf (generated
messages, protojson, Any, dynamicpb), the OpenAI and Anthropic SDKs with
tool calls and streaming, the AWS SDK, two script engines (JavaScript with
goja, expressions with expr), GraphQL, HTML processing, JOSE and
CBOR/MessagePack, with the same output as the native build. v2 also runs
zap, but not aws.

## Per plugin

| plugin | yaegi | v1 | v1 -aot | v2 | cause |
|---|---|---|---|---|---|
| protobuf | load | ok | ok | ok | |
| openai | load | ok | ok | ok | |
| aws | load | ok | ok | load | v2: nil pointer dereference in the init of the plugin |
| expr | load | ok | ok | ok | |
| goja | load | ok | ok | ok | |
| graphql | load | ok | ok | ok | |
| html | load | ok | ok | ok | |
| jose | load | ok | ok | ok | |
| cbor | load | ok | run | ok | v1 -aot: fixed later (below) |
| anthropic | load | ok | ok | ok | |
| zap | load | load | load | ok | v1 on darwin: a function without a Go body (below) |
| cel | load | load | load | new | v1: fixed later (below); v2: `maps.clone` has no body (assembly) |
| grpc | load | load | load | new | v1 on darwin: a function without a Go body (below); v2: nil pointer dereference in New |
| wazero | load | load | load | load | v1: a function without a Go body (below); v2: `golang.org/x/sys/cpu.syscall_syscall6` has no body |
| k8s | load | load | load | load | rule: client-go imports `os/exec` (exec credential plugin); with `-vmexec mock`, v1 ok in both modes (below) |
| otel | load | load | load | load | rule: `go.opentelemetry.io/otel/sdk/resource` imports `os/exec`; with `-vmexec mock`, v1 ok in both modes (below) |
| opa | load | load | load | load | rule: the wasm compiler of OPA imports `os/exec`; with `-vmexec mock`, v1 ok in both modes (below) |
| lua | load | load | load | load | rule: gopher-lua imports `os/exec` (Lua's `os` library) |
| redis | load | load | load | load | rule: miniredis embeds gopher-lua, so `os/exec` |
| prometheus | load | load | load | load | rule: client_golang imports `expvar` |

"rule" is a restriction of yaegi v2 (Traefik's plugin rules), the same for
both engines: the import is refused at load time even when the plugin
never calls it. Six plugins out of 20 stop there. A host could accept
these imports and refuse the calls at run time instead (`os/exec` with no
symbols, as yaegi does); this is a decision to take, not a fix.

Without `useUnsafe`, 17 plugins are refused by yaegi v2 v1 at load time:
protobuf (`protoreflect`, `internal/strs`), `golang.org/x/sys/unix`,
xxhash, gjson, reflect2, goldmark, the JSON packages of the OpenAI and
Anthropic SDKs, smithy-go, msgpack, regexp2, wazero (`syscall`) all import
`unsafe` or `syscall`. Only expr, graphql and jose load without it.

yaegi refuses every plugin at load time. Its errors are not compared here.

## Past the os/exec rule

With `-vmexec mock` (the yaegi v2 hosts load the plugins with
`Options.Exec = vm.ExecMock`: os/exec can be imported, no program runs),
k8s, opa and otel give the output of the native build with yaegi v2 v1, in
the default mode and with `-aot` (darwin/arm64, Go 1.26.8, October 22,
2026). They needed these fixes:

- k8s: the init of go-spew
  read the flag field of reflect.Value with unsafe and panicked. The flag
  word of the VM now has the bits of gc, and `Type.FieldByName` on
  reflect.Value finds reflect's field. Then `slices.SortFunc` (in
  sigs.k8s.io/json) failed on the unexported helpers of pdqsort: the
  unexported functions of the standard library packages built from their
  embedded source now run in the VM.
- opa: `//go:embed` into an
  `embed.FS`, with the rules of the go command.
- otel: the metrics were
  missing in the default mode. A func field of a struct kept in an
  `atomic.Value` (`CompareAndSwap`) came back as a native func, so the
  call converted its pointer argument, and the values of generic types
  written through it lost their type. Such a func is now the closure
  again.

## Bugs found by these plugins

Fixed, each with a regression test:

- The native host of the
  catalog harness did not compile, so every native build failed.
- `reflect.NewAt` of
  `unsafe.Pointer(v.UnsafeAddr())` in interpreted code.
- `Value.SetIterKey` and
  `SetIterValue`.
- `reflect.Method.IsExported`.
- `flagEmbedRO` does not stick.
- Writes to unexported fields
  of a plugin struct that native code handed back.
- `json.Decode` into a `**T`
  whose `*T` has `UnmarshalJSON` (its test does not fail without the fix:
  the trigger is not isolated yet).
- A pointer method of a
  native type called through an interface on an interpreted variable
  (`json.RawMessage.UnmarshalJSON`) lost its writes.

Fixed later (the table above is the run before these fixes):

- cbor in AOT mode gives the
  output of the native build.
- cel gives the output of the
  native build, in both modes.
- zap, grpc and wazero reach
  functions without a Go body (a system call through the C library on
  darwin, the assembly of the wazero compiler). They are still refused by
  default; with the new `Options.StubAssembly`, zap (restricted, with
  `useUnsafe`) and grpc and wazero (with `Unrestricted` and `RealProcess`:
  they need the whole `syscall` package) give the output of the native
  build, in both modes. Since then, the catalog harness sets the option
  (`internal/pluginhost`), the `syscall` subset of `useUnsafe` has the
  `RawConn` and `Conn` types and `Read` of a descriptor lent by a `RawConn`
  callback, and the assembly system calls of `x/sys/unix` return `ENOSYS`
  on linux. As restricted plugins (`useUnsafe`, no `Unrestricted`), zap
  gives the output of the native build on darwin/arm64 and on linux (arm64
  and amd64, Go 1.26.8 in docker), and grpc on darwin/arm64, in both modes.
  On linux, grpc loaded but each call failed with `DeadlineExceeded`: its
  server sets `TCP_USER_TIMEOUT` with `syscall.SetsockoptInt` on each
  connection, which was outside the subset. Since `SetsockoptInt` and
  `GetsockoptInt` take the socket that a `RawConn` lends, for a short list
  of options (`docs/SANDBOX.md`), grpc as a restricted plugin with
  `useUnsafe` gives the output of the native build on linux/arm64 too (Go
  1.26.10 in docker), in both modes. Its dialer still sets `SO_KEEPALIVE`
  with `golang.org/x/sys/unix`, which returns `ENOSYS` and is ignored.
- `golang.org/x/sys/unix` now
  comes from the host (`vm/xsys.go`, `docs/SANDBOX.md`): its read-only
  subset really calls the system in a restricted plugin with `useUnsafe`.
  The dialer of grpc sets `SO_KEEPALIVE` for real (`unix.SetsockoptInt` on
  the lent socket), zap's go-isatty asks the real terminal state of
  `os.Stdout` (`unix.IoctlGetTermios`), and otel describes the host with
  `unix.Uname`. The plugins use older versions of x/sys than yaegi v2's
  v0.48.0 (zap v0.29.0, grpc v0.47.0, wazero v0.44.0, otel v0.48.0); the API
  they use is the same, so all four get the host copy. On linux/arm64,
  wazero did not load as a restricted plugin: `golang.org/x/sys/cpu` (run
  from source) calls `syscall.Uname`, which is now in the `syscall` subset
  too. Results on October 23, 2026, middleware scenario, `useUnsafe`,
  otel with `-vmexec mock`:

  | plugin | darwin/arm64 (Go 1.26.8) v1 | v1 -aot | linux/arm64 (Go 1.26.10, docker) v1 | v1 -aot |
  |---|---|---|---|---|
  | zap | ok | ok | ok | ok |
  | grpc | ok | ok | ok | ok |
  | wazero | ok | ok | ok (was load) | ok (was load) |
  | otel | ok | ok | ok | ok |
