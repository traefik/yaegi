# Reloading plugins (host pattern)

Decisions:

1. yaegi v2 follows the reload model of yaegi in Traefik. The host loads each
   plugin **once** and keeps that one `*vm.Plugin` (one `vm.Machine`) for
   the life of the process. At each change of the dynamic configuration,
   it only calls `CreateConfig` and `New` again. It calls `Close` only at
   shutdown.
2. `MaxGoroutines` bounds the goroutines of each `New` (each `vm.Instance`),
   and there is no total cap per plugin. The Traefik adapter logs a warning
   for the goroutines that a `New` left (`host.Leaks`, below).
3. The Traefik adapter never calls `Instance.Close` on the instances of an
   old configuration (yaegi behaviour): they live until `Plugin.Close` at
   shutdown. `Instance.Close` stays in the API for other hosts.

## What Traefik does (read in the Traefik source, master, November 2026)

- `pkg/plugins/builder.go`, `NewBuilder`: the plugins are downloaded and
  loaded at startup, one yaegi interpreter per plugin. The plugin code is
  never loaded again.
- `pkg/plugins/middlewareyaegi.go`: for each middleware of the dynamic
  configuration, `createConfig` calls `CreateConfig` and decodes the
  settings, and `NewHandler(ctx, next)` calls `New` of the same
  interpreter. A middleware used by several routers gets one `New` per
  router.
- `pkg/server/routerfactory.go`, `CreateRouters`: at each new
  configuration, the context of the previous one is canceled first
  (`cancelPrevState`), and the new routers get a new context. This is the
  `ctx` that `New` gets.
- At shutdown, `Server.Stop` stops the entry points, which serve the
  pending requests for `transport.lifeCycle.graceTimeOut` (10 s by default,
  `DefaultGraceTimeout` in `pkg/config/static/static_config.go`).

The model that a previous version of this file described (load the plugin
again with `LoadCached` at each reload, then `CloseContext` on the old
`Plugin`) is rejected: Traefik has no hook after the switch of the routers,
the grace timeout is only used at shutdown (long requests of the old
plugin would be cut 10 s after each reload), the package state of the
plugin would be lost at each reload (the cache and stream startup of
crowdsec-bouncer, the global cache of traefikoidc), and `LoadCached` keeps
its new packages in the SSA program and the `token.FileSet` of the first
load, never freed (see below).

## The pattern

```go
// Startup: one Plugin per plugin, for the life of the process.
// host.LoadKind also checks CreateConfig and New (host.CheckEntryPoints),
// so a broken plugin fails here, as under yaegi.
p, err := host.LoadKind(ctx, host.Options(), pkgPath, host.Middleware)

// Each new dynamic configuration, for each middleware that uses p:
cfg, err := host.CreateConfig(p)
// ... decode the settings of the middleware into cfg (cfg.Decode) ...
h, inst, err := host.NewInstanceHandler(ctx, p, next, cfg, name)
// ctx: the context of the new configuration (canceled at the next one).
// inst: the vm.Instance of this New call (see below). The host may drop it.

// Shutdown, after the entry points stopped (Traefik: after Server.Stop):
p.Close()
```

`host.NewHandler` is the same without the instance in the results, and
`host.NewProvider` and `host.CallMethod` do the same for a provider
(`internal/pluginhost` gives the same names to the tests). A host that does
not use package `host` does the same with the `vm` API:

```go
inst := p.NewInstance()
res, err := p.Call(inst.Context(ctx), "New", ctx, next, cfg, name)
```

The context of `Plugin.Call` carries the instance; the plugin gets `ctx`
itself as its argument, and cannot see the instance.

## Goroutines that New leaves

Many plugins start goroutines in `New` that never end: a cache janitor with
a `time.Ticker`, a refresh loop (darkweak/souin leaves 2 per `New`, under
yaegi too). A plugin that stops them with the `ctx` of `New` is fine: the
context of the old configuration is canceled at the next reload. The
others pile up, as under yaegi: about 4 KB of heap per goroutine (and its
Go stack).

Before instances, they all counted in `Options.MaxGoroutines` (a live count
of the machine, 1000 in `internal/pluginhost`), so after about 500 reloads
of souin every `New` failed with `ErrTooManyGoroutines`.

Each `New` now runs in its own `vm.Instance` (`vm/instance.go`). The
goroutines that the call starts (go statements, `iter.Pull` sequences, the
callbacks that native code runs on a goroutine of its own such as
`time.AfterFunc`, also when a goroutine of `New` gives it later), the
finalizers and cleanups that the code of the instance sets, and the
goroutines that these start in turn, belong to the instance, also after
`New` returned:

- `MaxGoroutines` bounds the live goroutines **of each instance**. The
  goroutines that old instances left do not make a new `New` fail. The
  goroutines that belong to no instance (the requests that `ServeHTTP`
  serves, a call without an instance) keep one cap together, as before.
- `inst.Goroutines()` reports the live goroutines of the instance:
  the host can log a plugin that leaks goroutines at each reload.
  `p.M.Goroutines()` counts all of them.
- `inst.Close()` stops the goroutines of that instance only, with
  `ErrClosed`, as `Plugin.Close` does for the whole machine (next blocking
  point or step refill, deferred calls run). The other instances, the
  handlers and the package variables are not touched. It is for hosts
  that know when an instance is gone: a handler of the instance may need a
  goroutine of the instance. **The Traefik adapter does not call it**
  (decision 3): Traefik has no point after the switch of the routers where
  the old handlers are sure to be unused, and yaegi never stops them
  either. The old instances live until `Plugin.Close` at shutdown.

A goroutine belongs to the instance that started it, also when it serves
every instance. A shared service that the first `New` starts once
(with `sync.Once`, for a cache in a package variable)
belongs to the first instance: `host.Leaks` reports it after the first
reload (a false positive: it does not grow with the reloads), and
`inst.Close()` on that instance stops the service for all the later
instances.

There is no total cap of goroutines per plugin (decision 2): a plugin that
leaks at each `New` grows without bound, as under yaegi, but only with the
number of reloads, which the plugin does not control. A total cap would
make `New` fail again after many reloads.

## The leak warning (recommended check)

The goroutines of an instance that still run after its configuration was
replaced are goroutines that `New` left: a well behaved plugin stops them
with the `ctx` of `New`, which Traefik cancels at the next reload. So the
check is on the old instances, with a threshold of **0** goroutines, after
a short delay (`host.LeakDelay`, 5 s: a goroutine that ends with the
context stops about 1 ms after the cancel). `host.Leaks` does the
bookkeeping and does not log:

```go
var leaks host.Leaks // one per plugin

// Each New (host.NewInstanceHandler, or p.Call with inst.Context):
leaks.Add(ctx, inst, middlewareName)

// After each reload (for example at the next one, or on a timer):
for _, l := range leaks.Check(0, host.LeakDelay) {
	log.Warn().Msgf("plugin %s, middleware %s: %d goroutines started by New still run after its configuration was replaced; they live until Traefik stops",
		pluginName, l.Name, l.Goroutines)
}
```

Each instance is reported once. `Leaks` keeps no instance alive (weak
pointers) and forgets an instance once it is reported, once it has no
goroutine left after the delay, or once nothing uses it, so it stays small
over many reloads. A host that wants less noise raises the threshold.
`p.M.Goroutines()` gives the total of the plugin, for a metric
(`p.M.Stats()` gives it with the other counters of the plugin).

The requests that a handler of the instance serves are **not** part of the
instance: `ServeHTTP` runs with the context of the request.

Tests: `TestReloadLeakedGoroutines` (2000 `New` on one `Plugin`, one leaked
goroutine each, with the cap of 1000; the package state is kept; the
first handler still serves), `TestInstanceClose`, `TestReloadHeap` (10000
reloads with a new context each, the old one canceled: the heap stays
flat), all in `internal/pluginhost/reload_test.go`; `TestLeaks` in
`host`.

## Shutdown

At shutdown, after the entry points stopped serving (Traefik: after
`Server.Stop`, which waits for the grace timeout), the host calls
`p.Close()` for each `Plugin`. `Close` stops every goroutine of the machine
(all the instances) and makes later calls fail with `ErrClosed` (a
`ServeHTTP` answers 503). It does not wait for the goroutines:
`p.M.Goroutines()` reports the ones that did not stop yet.
`p.CloseContext(ctx)` first lets the calls that run end, up to the deadline
of `ctx`; use it if requests may still run at that point.

What `Close` does not stop: native code that blocks (a network read, a
`sync.Mutex` wait), and native resources of the plugin (a listener of an
`http.Server`, a file). A `time.AfterFunc` still fires, and its callback
stops at once. A loop without blocking points in a background goroutine is
only stopped when the machine has a `Budget` or a `BackgroundRate`
(`internal/pluginhost` sets both). See `vm/close.go`.

## LoadCached

`Plugin.LoadCached` loads a plugin again against the program of a first
load. It is a measure of the per-plugin cost of a host that ships no Go
toolchain (`compare/bench`), not a reload tool: each call adds the packages of
the plugin to the SSA program and the `token.FileSet` of the first load,
and they stay there as long as that first `Plugin` lives, also after the new
`Plugin` is closed. The cost grows with the code of the plugin: about
64 KB per call for the small plugin of `TestLoadCachedKeepsProgram` (in
`vm`); the second review measured about 3 MB per call for a larger plugin.
Not fixed: an SSA program cannot drop a package.
