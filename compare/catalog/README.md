# Traefik plugin catalog harness

Measures how the public Traefik plugins behave under yaegi v0.16.1 (configured
as Traefik configures it) and under yaegi v2, against a native build of each
plugin. The report is [docs/CATALOG.md](../../docs/CATALOG.md).

The Go commands run from `compare/`, the module of the comparison tools
(`compare/go.mod`, it requires yaegi v0); the shell scripts run from the
repository root.

```bash
sh compare/catalog/fetch.sh          # clone plugins.txt into src/ (network)
cd compare
go run ./catalog/run -j 6            # build the hosts, run everything (network for the native builds)
go run ./catalog/run -reuse          # re-classify out/ without running anything
go run ./catalog/run -reuse -soak 10m  # run only the soak (several plugins per process)
cd ..
sh compare/catalog/deps.sh           # reflect/unsafe packages imported by the plugins -> results/deps.tsv
```

## What is run

`plugins.txt` lists the 100 most starred repositories of the GitHub topic
`traefik-plugin` (plugins.traefik.io is not reachable from the measuring
host). `fetch.sh` clones each at its highest semver tag, since the catalog
serves tagged versions. Repositories whose `.traefik.yml` declares a provider
or a wasm runtime, or that have no manifest, are out of scope of the
middleware scenario; the providers run in the provider scenario (see "Other
scenarios").

For each yaegi middleware, the same scenario (`vmhost/driver.go`) runs in four
hosts:

| host | how the plugin is loaded |
|---|---|
| native | `run` copies the module, adds a `main` package calling `CreateConfig`/`New` directly, and builds it with the module proxy; run twice |
| yaegi | `yhost`: a GOPATH holding the module under `src/<module>`, `stdlib.Symbols`, unsafe and syscall only when the manifest sets `useUnsafe`, `Eval` of the import, then `<basePkg>.New` and `.CreateConfig` (Traefik's `pkg/plugins/middlewareyaegi.go`) |
| govm (yaegi v2, the engine key of the results) | `vmhost`: `vm.Load` with `pluginhost.Options()` (budget, goroutine cap, load timeout, empty environment), then `CreateConfig`, `Object.Decode`, `New` through `Plugin.Call` (`internal/pluginhost`) |
| aot | `vmhost -aot`: the same with the packages of `aot/` in AOT mode |

In every host the manifest `testData` is decoded into the result of
`CreateConfig` with mapstructure and Traefik's options (weakly typed input,
comma-separated strings into slices). Then seven requests go through the
handler (paths and queries, client addresses and forwarding headers,
credentials, a JSON body, an HTML response, a CORS preflight, another host, a
repeated request). The transcript records the stage reached (load, config,
New), and for each request the status, the response headers and body, and
what reached the next handler. Load errors are recorded without their text,
which belongs to the engine; errors returned by `New` keep theirs.

The hosts only see the plugin directory. yaegi v2's default loader reads the
plugin module and its `vendor/` directly and type-checks against the embedded
standard library types (`stdtypes/`): the yaegi v2 hosts run with
`PATH=/usr/bin:/bin` (no `go` command) and no `GOROOT`, so a plugin that does
not vendor its dependencies fails to load under yaegi v2 as it does under yaegi.
For the `go list` loader (`vm.Options.GoList`), the hosts also get
`GOPROXY=off`, `GOWORK=off` and a module cache that holds nothing but Go
toolchains; a toolchain newer than the local one, required by a `go.mod`, is
downloaded into the real cache first.

## Extra scenarios

The base scenario sends seven sequential requests into an
`httptest.ResponseRecorder`, through one handler, one plugin per process.
That hides what a proxy does with a plugin. `vmhost/driver_extra.go` (shared
by the three hosts like `driver.go`, selected with `-scenario`, the same flag
as the scenarios of the next section) adds:

| scenario | what it does | what it shows |
|---|---|---|
| `headers` | 5 requests with several values per header (`X-Forwarded-For`, `Cookie`, `Accept`, `Authorization`...); next echoes the request headers in its body and sets response headers with several values (`Set-Cookie`, `Vary`...) | plugins that read or rewrite request or response headers in place |
| `stream` | a real `httptest.Server`; `/events`: next writes a chunk, flushes, waits (2 s at most) until the client has read it, writes again (SSE); `/ws`: a raw HTTP/1.1 Upgrade handshake, next hijacks the connection, answers 101 and echoes one line (WebSocket style); `/index.html`: a plain request | whether `http.Flusher` and `http.Hijacker` survive the plugin |
| `conc` | 32 goroutines x 50 requests (`-conc`) of the base scenario through one handler; the hosts (also the native one) are built with `-race` | data races, panics, and the set of distinct responses of each request |
| soak | every plugin that passes the base scenario in all engines, loaded in one process per engine (yaegi, yaegi v2, yaegi v2 -aot), 8 goroutines of traffic for `-soak` (10 minutes for the committed results) | heap after GC, RSS and goroutines over time (leaks), panics and timeouts |

`run` runs `headers`, `stream` and `conc` (values of `-scenarios`, see "Other
scenarios"; code in `run/extra.go`) for each plugin after the base scenario
(they need its native host, so they run it too), with the same five hosts (native twice, yaegi, yaegi v2,
yaegi v2 -aot), and compares them to native the same way. For `conc`, the
counts of each distinct response are not compared, only the set of distinct
responses: the order of concurrent requests changes the counts of stateful
plugins (rate limits, caches, ban counters). When the two native runs differ,
the coarse comparison is used, as for the base scenario.

A scenario is never skipped. When it cannot test what it is about for a
plugin, the `applies` column of `results/scenarios.tsv` says why, from the
native transcript: `n/a: native stops at new` (New fails natively), `n/a:
native never reaches next` (the plugin answers every request itself), `n/a:
native does not reach next for sse and ws`. The verdicts are still given.

The stream columns give, per engine, `yes` when the client saw the first SSE
chunk before the end of the response (flush) or when the upgrade answered 101
and echoed (hijack), `no` otherwise, `-` when next was not called. The race
columns count the `WARNING: DATA RACE` reports of each `conc` host. A race
in the native host is a race of the plugin itself. An engine host that has a
race report with a frame of the engine (`github.com/traefik/yaegi/v2/`,
`github.com/traefik/yaegi/`) while neither native run had any race gets the
verdict `race`, a failure of that engine, whatever its output. The native
runs are what tells the two kinds of race apart: the engine runs the plugin
code, so a race of the plugin's own code also has engine frames. When the
plugin races natively, the races of the engines only stay in the counts.

The soak has no native host: one native binary with all the plugins would
need one module with all their dependencies. `results/soak.txt` summarizes
the samples (`results/soak-<engine>.tsv`): after the load, at a quarter of
the traffic, at the end, and 2 seconds after the context given to New is
canceled; it lists the plugins with panics or timeouts. `-soakonly` limits
the soak to some plugins (to find which one leaks); a native host refuses
`-soak`.

`go test ./catalog/vmhost` (in `compare/`) runs the scenarios with yaegi v2 on
`vmhost/testdata/streamplug`, with a 2 second soak. The hosts of these
scenarios and of the soak have no internet, as the ones of the next section.

## Hostile corpus

`vmhost/testdata/hostile/` holds plugins that misbehave on purpose, one per
guard rail: `for {}` in `ServeHTTP` and in
`init`, a type that doubles at each step (load bomb), background goroutines
that spin and sleep, a goroutine bomb, 3000 `time.AfterFunc` callbacks at the
same time, unlock of an unlocked mutex, concurrent map writes, 1 MiB buffers
kept until the memory runs out, `os.Exit` and `log.Fatal`, panics in a
goroutine and in a timer, reads of the host environment, `os.Stdout.Close()`,
writes to `http.DefaultClient`, `http.DefaultTransport` and `net.IPv4bcast`,
routes and flags on the process-wide registries, infinite recursion
(direct and through `strings.Map`), `os.NewFile(1, ...)`. For each one,
`vmhost/hostile.go` names the guard rail that must stop it and checks from
the host that it held: the request ends with the expected panic
(`ErrBudget`, `ErrTooManyGoroutines`, `*vm.FatalError`, `ErrMemory`,
`ErrStackOverflow`, `*vm.RefusedError`...), the state of the host did not
change, and the process is still alive. The plugins are loaded with
`pluginhost.Options()` (with a memory quota of 256 MiB instead of 1 GiB).

```bash
cd compare
go run ./catalog/vmhost hostile -dir .          # all the cases, default mode
go run ./catalog/vmhost hostile -dir . -aot spin unlock
go test ./catalog/vmhost -run TestHostileCorpus  # both modes
```

Each case runs in a child process, since a guard that fails can end the
process (a fatal error of the runtime); the output has one line per case,
`held` or `BROKEN`. `go test ./...` in `compare/` runs the corpus in both modes, so the
guard rails are checked on each run. There is no native or yaegi host for
these plugins: the guard rails are what keeps the host alive.

## Classification

Each engine gets a verdict against the native transcript: `ok` (identical),
`diff` (runs, prints something else), or the stage where it fails (`load`,
`config`, `new`, `run` for a panic or a timeout in a request, `crash`), and in
`conc` `race` (a data race in the engine, see above). When
the two native runs differ (time, random values), only the coarse transcripts
are compared: stages, statuses and header names. HTTP dates and the wording of
a failed connection to a host the machine cannot reach (which resolver
answered, or a client timeout first) are normalized before any comparison.

Each plugin then gets a class, yaegi against yaegi v2 (and against yaegi v2 -aot):

- `same`: same transcript;
- `yaegi v2 fails`: yaegi is right (as native) and yaegi v2 is not, or yaegi v2 fails to
  load where yaegi loads;
- `yaegi fails`: the reverse;
- `both fail`: neither loads;
- `different`: both run, differently, and neither matches native.

`results/scenarios.tsv` holds one line per repository and extra scenario,
`results/scenarios-summary.txt` the counts, `results/soak.txt` and
`results/soak-*.tsv` the soak. `results/catalog.tsv` holds one line per repository, `results/versions.tsv` the tag and commit measured; `out/<plugin>/` (ignored)
holds the transcripts (`*.txt`), the stderr of each host (`*.err`), the exit
status (`*.rc`) and the times (`*.times`).

## Other scenarios

`run` also runs five other scenarios. One flag `-scenarios` selects all of
them (default: all,
`middleware,auth,reload,reloadcancel,reloadsandbox,provider,headers,stream,conc`;
an unknown name stops the runner), and the hosts take the same names with
`-scenario` (default `middleware`), except reloadcancel and reloadsandbox,
which are the reload scenario of the hosts with other flags (below). They use the same hosts and the same
classification. Their results go to `results/auth.tsv`, `results/reload.tsv`,
`results/reloadcancel.tsv`, `results/reloadsandbox.tsv`,
`results/provider.tsv` and `results/scenarios.txt`; the transcripts to
`out/<plugin>/<scenario>-<engine>.txt`.

```bash
cd compare
go run ./catalog/run -j 4 -scenarios auth,reload,provider
go run ./catalog/run -scenarios auth -only 'jwt'   # some plugins, no results files written
```

The runner serves, on `-authaddr` (default `127.0.0.1:38551`, the same for
every host so that the transcripts can be compared), a minimal OIDC provider
(discovery document, `/authorize`, `/token` with the authorization code and
password grants, `/userinfo`, `/introspect`, `/jwks`, and the same endpoints
under Keycloak style paths) and the API mocks of the data files.

- **auth**: the credential plugins listed in `scenarios/auth.yml` get a
  configuration that trusts the local key and issuer, then requests with
  valid and invalid credentials: JWTs signed by the runner (RS256 and HS256:
  valid, expired, wrong signature, missing claims, NumericDate with a
  fraction, wrong issuer, wrong audience, not yet valid, fresh, plus `none`,
  garbage and no token), an OIDC code flow (login redirection, callback with
  the code, request with the session cookie), basic auth, API keys and
  forward-auth services. The driver keeps the cookies set by the responses
  and fills the state and the code of the callback from the last redirection.
  The keys in `scenarios/` are test keys, used only here. The plugins that
  cannot be configured locally stay in `results/auth.tsv` with the reason.
- **reload**: for each middleware, 200 rounds of `CreateConfig`, decoding and
  `New`, with in turn the manifest testData, the defaults of `CreateConfig`,
  and the auth configuration when there is one; then the seven requests of the
  middleware scenario through a last handler built from testData. The outcome
  of each configuration is in the transcript (again if it changes). The
  goroutines and the live heap after round 20 and at the end go to the times
  file: a leak is a growth per round of more than 0.5 goroutine or 64 KiB
  beyond the growth of native in the same scenario. All the rounds share one
  context that is canceled only at the end, so the scenario shows that yaegi v2
  does not keep more than native.
- **reloadcancel**: the same rounds with the context of `New` that Traefik
  gives. At each new dynamic configuration, `RouterFactory.CreateRouters`
  (`pkg/server/routerfactory.go`) cancels the context of the previous
  configuration first, then builds the new handlers with a new context
  (checked on the integration branch of Traefik: the context of `New` is the
  one of `BuildHandlers`, and it is also the context of the call into the
  plugin). So each round has its own context, canceled before the next round
  calls `New` (the host flag `-reloadctx cancel`). Between the two, the old
  handler still serves in Traefik: one request (the first of the middleware
  scenario) goes through the handler of the previous round, whose context is
  done, and its status goes to the transcript. The last context is canceled
  before the goroutines and the heap are measured: what stays are the
  goroutines that do not end with their context.
- **reloadsandbox**: reloadcancel, with the yaegi v2 hosts loading the plugin
  with an `Options.Sandbox` (`vmhost -sandbox`: the empty environment, read
  in the plugin directory and the temporary directory, write in the
  temporary directory, dial 127.0.0.1) and the budget and goroutine cap of
  `pluginhost.Options()`. The native and yaegi hosts are the ones of
  reloadcancel (`engineOpts.sameAs`). It shows that the plugins work the
  same with these limits over 200 reloads. That the limits still hold after
  200 reloads is checked by `TestReloadSandboxLimits`
  (`vmhost/reload_test.go`, `vmhost/testdata/reloadplug`): 200 reloads with
  a canceled context each, a cap of 20 goroutines that a goroutine per `New`
  would pass if a canceled context did not end it, then a `for {}`
  (`ErrBudget`), a read and a write outside the sandbox (refused), the
  environment (empty) and 100 new goroutines (`ErrTooManyGoroutines`), on the
  last handler and on the one of the first round, in both modes.
- **provider**: the four provider plugins (`scenarios/providers.yml`) run as
  Traefik runs them (`pkg/plugins/providers.go`): `CreateConfig`, decoding,
  `New` (yaegi: through the same wrapper package and `PP` interface as
  Traefik), `Init`, `Provide` with a channel of `json.Marshaler` read by
  another goroutine, then `Stop`. The first three messages go to the
  transcript as JSON with sorted keys. The APIs they poll are mocked. The
  goroutines one second after `Stop` are compared with the maximum of the two
  native runs: an engine that keeps even one more (a `Provide` goroutine not
  stopped) gets `goroutines` in the `leak_*` columns of `results/provider.tsv`.

The hosts of every scenario but `middleware` (these three, the extra ones and
the soak) have no internet: `HTTP_PROXY` and `HTTPS_PROXY`
point to a local proxy of the runner that answers 403 to every request (also
`CONNECT`), and `NO_PROXY` keeps the local servers. A plugin that downloads
in `New` (bot-wrangler fetches a list from cdn.jsdelivr.net) gets the same
refusal in every round and every engine, whatever the network of the machine.
Clients that do not read the proxy from the environment are not covered.

For these scenarios the second native run starts 1.5 s after the first: a
plugin that puts the time (in seconds) in a cookie or a token is then seen as
nondeterministic, and only the coarse transcripts are compared.
