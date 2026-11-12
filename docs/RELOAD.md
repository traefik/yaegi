# Reloading plugins (host pattern)

Traefik calls the `New` function of a middleware plugin at each reload of
its dynamic configuration. A plugin often starts goroutines in `New`: a
cache janitor with a `time.Ticker`, a refresh loop. Go has no way to stop
them from outside, and the plugin has no hook to stop them either. Without
help from the host they run for ever, and they count in
`Options.MaxGoroutines` for the whole life of the plugin: a plugin that
leaves 2 goroutines per `New` (darkweak/souin) fails at every `New` after
about 500 reloads with the `MaxGoroutines` of 1000 of `internal/pluginhost`
(`TestReloadGoroutineCap`).

yaegi v2 cannot stop the goroutines of one `New` only: the goroutines of the
instances share the variables of the package. The unit of life is the
`*vm.Plugin` (one `vm.Machine`). The lifetime is per
plugin, with a drain.

## The pattern

At each reload of the configuration:

1. For each plugin, load it again. `base.LoadCached(dir, opts)` reuses the
   program of a first load (`base`, never closed): about 2 ms for a small
   plugin (`TestReloadByLoad`); a full `Load` of a large plugin takes
   seconds (TODO.md, section 5, "Load time").
2. Call `CreateConfig` and `New` on the new `Plugin` for each middleware of
   the new configuration, and make the new router live.
3. Call `CloseContext(ctx)` on the old `Plugin`, in the background, with a
   deadline (the drain delay):
   - new calls into the old plugin fail at once (`ErrClosed`; a
     `ServeHTTP` answers 503). After step 2 the old handlers should not
     get new requests; a nested call (the same plugin twice in a chain:
     the first instance calls the second through `next`) is not new and is
     still accepted;
   - the requests that the old handlers serve go on until they end or the
     deadline passes;
   - then `Close` stops every goroutine of the old plugin with `ErrClosed`:
     the background goroutines, and the requests still running at the
     deadline (net/http gets `http.ErrAbortHandler`). `CloseContext`
     returns `ctx.Err()` when requests were stopped this way.

```go
newP, err := base.LoadCached(dir, opts)
// ... CreateConfig and New on newP, swap the router ...
go func(old *vm.Plugin) {
	ctx, cancel := context.WithTimeout(context.Background(), drainDelay)
	defer cancel()
	if err := old.CloseContext(ctx); err != nil {
		log.Printf("plugin %s: requests stopped at the end of the drain: %v", name, err)
	}
}(oldP)
```

## The drain delay

Use the time that Traefik already gives the requests of an old
configuration to end: the `transport.lifeCycle.graceTimeOut` of the entry
points, 10 s by default. A plugin whose requests
stream for longer (WebSocket, server-sent events) needs a longer delay, or
its long requests are cut at the deadline, as they are when Traefik stops.

## What Close does not stop

Native code that blocks (a network read, a `sync.Mutex` wait), and native
resources of the plugin (a listener of an `http.Server`, a file). A
`time.Ticker` or `time.Timer` that no live goroutine uses is collected by
the Go runtime; a `time.AfterFunc` still fires, and its callback stops at
once. A loop without blocking points in a background goroutine is only
stopped when the machine has a `Budget` or a `BackgroundRate`
(`internal/pluginhost` sets both). See `vm/close.go`.

## Open questions

- A plugin whose configuration did not change could keep its `Plugin` (and
  its goroutines) across a reload: the adapter would need to compare the
  configurations of the plugin. Not done: each reload loads every plugin
  again.
- `Close` and `CloseContext` do not wait for the goroutines to end: they
  stop at their next blocking point or step refill. `Machine.Goroutines`
  reports the ones that did not stop yet.
