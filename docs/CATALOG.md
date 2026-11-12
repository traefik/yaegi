# Traefik plugin catalog: yaegi against yaegi v2

Measured again on 23 October 2026, on main d418abb, with every scenario (section "Measured again on current main" below; the commit and the commands are in `compare/catalog/results/MEASURED.txt`). The sections after it are the earlier measures, from 17 to 19 October 2026, kept for their analysis. Harness, method and commands: `compare/catalog/README.md`. Raw data: `compare/catalog/results/` (`catalog.tsv`, `summary.txt`, `deps.tsv`, `versions.tsv`; for the credential, provider and reload scenarios: `auth.tsv`, `provider.tsv`, `reload.tsv`, `reloadcancel.tsv`, `reloadsandbox.tsv`, `scenarios.txt`; for the extra scenarios: `scenarios.tsv`, `scenarios-summary.txt`, `soak.txt`, `soak-*.tsv`). The class names of the raw data are: same, different, yaegi v2 fails, yaegi fails, both fail.

## Measured again on current main (23 October 2026)

All the scenarios ran again on main d418abb (`go run ./compare/catalog/run -j 4`, then `-scenarios reloadcancel,reloadsandbox`, then `-reuse -soak 10m`), with the same 100 repositories at the same tags, every host but the middleware one behind the no-network proxy. The `race` verdicts of `conc` are measured this time, not derived. The machine was shared (load average 25 to 75 on 12 cores): the load times and the request counts of the soak are not comparable with the earlier runs.

| scenario | yaegi v2 same as native (VM / AOT) | yaegi v2 against yaegi, VM mode | AOT mode | changes since 17 to 19 October |
|---|---|---|---|---|
| middleware (93) | 88 / 88 (yaegi 88) | 91 same, 2 both fail | 90 same, 1 different, 2 both fail | Aetherinox loads now (`IgnoreSoftErrors`); its output differs only by a timestamp, and native does not build, so it cannot be seen as nondeterministic (the AOT "different") |
| auth (17 configured) | 15 / 16 (yaegi 15) | 14 same, 1 yaegi v2 fails, 1 different, 1 yaegi fails | 15 same, 1 different, 1 yaegi fails | **new yaegi v2 failure in VM mode**: agilezebra/jwt-middleware, see G3 below |
| reload, one context (93) | | 90 same, 2 both fail, 1 yaegi fails | 88 same, 1 yaegi v2 fails, 1 different, 2 both fail, 1 yaegi fails | the AOT "yaegi v2 fails" is ciaranj/cacheify, whose cache on disk outlives the process (the first host gets a miss); holysoles/bot-wrangler is identical again (no-network proxy) |
| reloadcancel (new, 93) | | 89 same, 2 both fail, 2 yaegi fail | 89 same, 1 different, 2 both fail, 1 yaegi fails | no yaegi v2 failure and no yaegi v2 leak |
| reloadsandbox (new, 93) | | 85 same, 3 yaegi v2 fail, 2 different, 2 both fail, 1 yaegi fails | same | the yaegi v2 failures and one different are refusals of the sandbox (below) |
| provider (4) | 4 / 4 (yaegi 3) | 3 same, 1 yaegi fails | same | Service Fabric: yaegi v2 kept 6 goroutines more than native after `Stop` (G4, fixed since) |
| headers (93, 53 apply) | 84 / 85 ok (yaegi 85) | 89 same, 1 yaegi v2 fails, 1 different, 2 both fail | 89 same, 2 different, 2 both fail | the yaegi v2 failure is darkweak/go-esi, which writes a map from several goroutines (`fatal error: concurrent map writes`, natively too in `conc`) |
| stream (93, 52 apply) | 88 / 88 ok (yaegi 82) | 84 same, 6 yaegi fail | same | **Flush and Hijack survive every plugin that loads**: the 7 plugins of G1 below get the native flush and hijack; "Hijack lost" is gone |
| conc (93, 52 apply), `-race` | 83 / 82 ok (yaegi 72) | 74 same, 12 yaegi fail, 3 different, 4 both fail | 73 same, 1 yaegi v2 fails, 13 yaegi fail, 3 different, 3 both fail | no `race` verdict for yaegi v2 in either mode (G2 is fixed): yaegi v2 reports races in 7 plugins, all racing natively too (native 8, yaegi 20); the AOT "yaegi v2 fails" is traefik/plugindemo, which races natively (its set of responses changes from run to run); darkweak/souin hit the goroutine cap because the hosts never ended the request contexts (G5, a harness artifact) |

### Reloads with the context that Traefik gives (reloadcancel)

On the integration branch of Traefik (`pkg/server/routerfactory.go`), each new dynamic configuration cancels the context of the previous one first (`cancelPrevState`), then builds the new handlers with a new context; that context is the one given to `New` (and to the call into the plugin, `Plugin.Call(ctx, "New", ctx, ...)`). The reloadcancel scenario does the same, and sends one request to the handler of the previous round after its context is canceled (in Traefik the old handlers serve until the new ones replace them).

- The verdicts are the ones of reload, with no yaegi v2 failure: the old handlers answer the same in every engine.
- The goroutines that plugins start in `New` and stop with its context end in every engine: traefikoidc, umami-feeder, ipwhitelistshaper, traefik-warp and the bad bot blocker keep nothing per round any more (they kept 1 to 2.7 goroutines and 30 to 97 KiB per round in reload). 9 plugins still keep goroutines per round, natively too (souin and the queue manager 2, bandwidthlimiter and torblock 1, the OIDC and authentik plugins 0.67, simplecache and cacheify 0.5, traefik-wol 0.14). Their heap per round is still larger under yaegi v2: about 60 KiB against 13 to 58 for yaegi and 4 natively for souin and the queue manager, about 30 KiB against 5 and 2 for bandwidthlimiter and torblock.
- No yaegi v2 leak verdict (yaegi: libops/captcha-protect, 1 goroutine per round).

### Reloads with a sandbox and a budget (reloadsandbox)

The yaegi v2 hosts load the plugin with an `Options.Sandbox` (empty environment, read in the plugin directory and in the temporary directory, write in the temporary directory, dial 127.0.0.1) and the budget and goroutine cap of `pluginhost.Options()`, then run reloadcancel. 85 plugins give the same transcript as yaegi. The others are refusals of the sandbox, as it should refuse: traefik/plugin-simplecache and ciaranj/cacheify keep their cache in `/tmp` (`stat /tmp: denied by the plugin sandbox`; the host gives a temporary directory of its own), smerschjohann/mtlswhitelist reads `/proc/net/route`; Miromani4/traefik-plugin-AdminAPI_WebUI panics in `New` natively in this run (it depends on files of its directory) and not in the sandbox. That the limits still hold after 200 reloads is checked by a test, `TestReloadSandboxLimits` (`compare/catalog/vmhost`): after 200 reloads with a canceled context each, the budget stops a `for {}`, the sandbox refuses a read and a write outside its paths and gives an empty environment, and the goroutine cap stops 100 new goroutines, on the last handler and on the one of the first round, in both modes. The cap (20) would be passed if a canceled context did not end the goroutine that each `New` starts.

### Soak

84 plugins selected, 78 running in one process per engine (6 fail in `New` in every engine: network refused by the proxy, files with a path relative to the working directory), 8 goroutines of traffic for 10 minutes, the three engines at the same time:

| engine | heap after load | heap at 1/4 | heap at the end | goroutines 1/4 / end / after cancel | requests in 10 min |
|---|---|---|---|---|---|
| yaegi | 336 MB | 338 MB | 338 MB | 88 / 66 / 51 | 36 880 |
| yaegi v2 | 359 MB | 370 MB | 369 MB | 96 / 72 / 57 | 37 036 |
| yaegi v2 -aot | 359 MB | 369 MB | 368 MB | 96 / 72 / 57 | 37 036 |

No leak in any engine (the heap is flat after the warm up), no panic. yaegi v2 holds about 31 MB more than yaegi for 78 plugins (85 MB on 19 October). MarkusJx/traefik-wol times out in every engine (it sends a wake-on-LAN packet and waits); yaegi also times out twice on darkweak/souin.

### yaegi v2 failures found by this run

- **G3. VM mode: a `json.Number` made by a `json.Decoder` is encoded as a string** (agilezebra/jwt-middleware, auth scenario: `{"id":7}` in a claim becomes `{"id":"7"}` in its header). Bisected to d1c7346 ("json.Encoder and json.Decoder go to the AOT copy in default mode"): the decoder of the AOT copy makes the values of `UseNumber`, and `json.Marshal` of default mode (the native one) does not know the `json.Number` of the copy, so it writes a string. AOT mode is right, and so was VM mode on 19 October. It belongs to "Silent differences in the default (VM) mode" of `TODO.md`. Repro, `go run` and yaegi v2 with `-aot` print `{"id":7}`, yaegi v2 in default mode prints `{"id":"7"}`:
  ```go
  d := json.NewDecoder(strings.NewReader(`{"id":7}`))
  d.UseNumber()
  var m map[string]any
  d.Decode(&m)
  b, _ := json.Marshal(m)
  fmt.Println(string(b))
  ```
- **G4. A plugin client with a `Timeout` and a nil `Transport` used the legacy cancellation of net/http** (dariopb/traefikServiceFabricPlugin, provider scenario: 15 and 13 goroutines after `Stop` against 9 natively). The machine's own `DefaultTransport` was reached through a wrapper type that net/http does not know, so each request with a timeout set `Request.Cancel` and started one more goroutine. Fixed since.
- **G5. darkweak/souin passed the goroutine cap under 32 concurrent clients in this run: a harness artifact, not yaegi v2** (`conc`, both modes): 146 of 1600 requests panicked with `vm: too many goroutines (MaxGoroutines)` (1000 in `pluginhost.Options()`, the value of the Traefik integration). For each request, souin starts a goroutine that waits for the end of the request context (`<-req.Context().Done()`, `pkg/middleware/middleware.go`). The hosts built their requests with `httptest.NewRequest` and never ended that context, so each request left one goroutine, natively too: at the same load, the native build keeps 1147 goroutines (2290 at 64 clients), and yaegi v2 with no cap keeps the same number within 2, in both modes. net/http ends the context of a request when its handler returns, and the hosts now do the same (`newRequest` in `compare/catalog/vmhost/driver.go`, shared by the three hosts; `TestRequestContextDone`). With the cap of 1000, souin then peaks at 97 goroutines natively, 119 under yaegi v2 and 132 under `-aot` at 32 clients (211, 251 and 259 at 64), with no panic, and 4 goroutines left at the end in every engine. The 1000 of the Traefik integration covers about 500 concurrent requests per souin instance.
- The heap kept per plugin instance is still 2 to 15 times the native one for the plugins that keep a handler per reload (above); traefikoidc in reload (one context): 97.5 KiB per round against 12.3 natively, over the leak threshold in both modes (the pattern of "yaegi v2 bugs found", item 3, below).

Not yaegi v2: ciaranj/cacheify (cache on disk shared by the hosts), Aetherinox (a timestamp, native does not build), darkweak/go-esi and unsoon/traefik-open-policy-agent (concurrent map writes of the plugin), acouvreur/traefik-modsecurity-plugin (now `diff` in every engine in headers, conc and reload, the same way: its WAF backend is behind the no-network proxy), Miromani4/traefik-plugin-AdminAPI_WebUI and wiltonsr/ldapAuth (their native run changed, the same in every engine).

Load times (`load_ms`, new process, 4 in parallel), on the loaded machine: median yaegi 31 ms, yaegi v2 220 ms, `-aot` 226 ms (21, 134 and 131 ms on 17 October on a quieter machine). Not comparable; to measure again on a quiet machine.

## Status of 17 October (phase 2 bis)

Out of the 93 measured middlewares, yaegi, yaegi v2 and yaegi v2 `-aot` each give the same output as the native binary for 88 plugins. When we compare the two engines directly:

| class | yaegi v2 (default) | yaegi v2 -aot | end of phase 2 (default / -aot) | after natfast and task 6 | first measure |
|---|---|---|---|---|---|
| same | 90 | 90 | 87 / 89 | 85 | 68 |
| yaegi v2 fails | 1 | 1 | 4 / 2 | 6 | 22 |
| yaegi fails | 0 | 0 | 0 / 0 | 1 | 1 |
| both fail | 2 | 2 | 2 / 2 | 1 | 1 |
| different | 0 | 0 | 0 / 0 | 0 | 1 |

yaegi v2 loads without the Go toolchain: the yaegi v2 hosts of the harness run with `PATH=/usr/bin:/bin` (no `go`) and without `GOROOT`, and with the Traefik import restrictions (task 4). The only yaegi v2 failure left, in both modes, is a plugin that is not valid Go: yaegi v2 is at the level of yaegi on the catalog, except for this plugin.

What changed since the first measure:
- **natfast fix** (cause A, 12 plugins): the fast path of `ServeHTTP` falls back to the general conversion when the `ResponseWriter` is an interpreted value. jptosso/coraza-traefik and ciaranj/cacheify also benefit from it.
- **Task 6, loading without toolchain**: `vm.Load` reads the plugin module and its `vendor/` directly, and type checks everything against the embedded stdlib types (`stdtypes/`). Cause B disappears: darkweak/go-esi (inconsistent vendoring), Lepkem (module graph), zekihan (go.mod in Go 1.27; the version is capped to the VM version, as yaegi ignores it). Since 22 October 2026, both need an option that the catalog host sets (`Options.IgnoreInconsistentVendoring`, `Options.AllowNewerGo`): the zero `Options` refuse them, as `go build` does (see "Options that accept what go build refuses" in the README). blueshift-labs also passes: the sources of `cmp`, `iter`, `slices` and `maps` are embedded, so the instances of their generic functions are interpreted.
- **Task 3** (`context.WithValue`): tomMoulard/fail2ban and darkweak/souin in `-aot`.
- **Task 4** (import restrictions): jptosso/coraza-traefik becomes "both fail". Its vendored `go.uber.org/atomic` imports `unsafe` without `useUnsafe`, and Traefik refuses this too.
- **Task 7** (plugin reflect routed to the twin): maxlerebourg/crowdsec-bouncer-traefik-plugin and wiltonsr/ldapAuth in `-aot` (`reflect.Indirect`).
- **Task 8** (containers, `runtime.SetFinalizer`): libops/captcha-protect in both modes (go-cache), darkweak/souin in default mode.
- **Phase 2 bis**:
  - native registry generated for the Go version of the host (`stdsyms/`): MarkusJx/traefik-wol (unexported native type `net.conn`);
  - interpreted pointers with methods that go through a native `any` and come back: negasus/traefik-plugin-bridge in default mode;
  - per-machine entries in the AOT maps indexed by `reflect.Type`: lukaszraczylo/traefikoidc in default mode.

Measured again on 18 October 2026 after new load-time refusals (refusal of `os.StartProcess`, `net/http/cgi`, `unsafe.Pointer` without `useUnsafe`... in the loaded code, copied environment, `docs/SANDBOX.md`): same classes for the 93 plugins, none of them uses what is now refused. Median load: yaegi v2 116 ms, `-aot` 111 ms, yaegi 15 ms (the machine had less load than the day before).

yaegi v2 failure left:

| plugin | mode | cause |
|---|---|---|
| Aetherinox/traefik-api-token-middleware | both | invalid Go code (unused variables), also refused by the Go compiler |

**Update (21 October 2026)**: `Options.IgnoreSoftErrors` (set by `pluginhost.Options()`, which the catalog host `vmhost` uses) loads a plugin whose only type errors are unused variables, imports or labels (soft errors of go/types), as yaegi does. Aetherinox/traefik-api-token-middleware then loads and its middleware transcript is the same as yaegi's (timestamps masked), in default and AOT modes. `pluginhost.Options()` also sets `Options.IgnoreUnusedExprs`: an expression statement whose value is not used (`c.S` alone, "is not used" for go/types) does not refuse the plugin either, as in yaegi. It also sets `Options.IgnoreDuplicateDecls`: a function, method, type or constant declared twice with the same text (a copy of the plugin file left next to it, as in nasim-samimi/adm, outside this catalog) is kept once; two different declarations of a name still refuse the plugin. Since 22 October 2026, `pluginhost.Options()` also sets `YaegiTags`, `AllowNewerGo`, `ImportFromArchive` and `IgnoreInconsistentVendoring`: what the loader did for every plugin before (the `// yaegi:tags` lines, a `go` line newer than the host, a package of the archive that a nested module does not vendor, a `vendor/` that does not match `go.mod`) is now off by default, as in `go build`. The tables below are from the earlier runs.

The harness normalizes HTTP dates and the text of a failed connection to a host that does not exist (the resolver answered, or the client timed out): these differences came from the environment, not from the engines (sablier, ondemand).

Load times (`load_ms`, new process, harness with 4 in parallel on 4 cores):

| engine | median | 10th percentile | 90th percentile | max |
|---|---|---|---|---|
| yaegi | 18 ms | 8 ms | 56 ms | 664 ms |
| yaegi v2 | 130 ms | 104 ms | 175 ms | 542 ms |
| yaegi v2 -aot | 132 ms | 103 ms | 181 ms | 472 ms |
| yaegi v2, end of phase 2 | 123 ms | 100 ms | 172 ms | 388 ms |
| yaegi v2, first measure (`go list`) | 562 ms | 308 ms | 6.9 s | 9.3 s |

Of these 130 ms, about 85 ms are spent decoding the stdlib types, paid once per process. `cmd/loadtime` measures the next loads, one after the other: 6 ms for plugindemo or plugin-rewritebody, 14 ms for geoblock, 24 ms for crowdsec-bouncer, 134 ms for traefik-oidc-auth and 290 ms for traefikoidc, the largest plugin of the catalog. The rest of the time is split between parsing and type checking the plugin code and building the SSA. The bytecode is not cached on disk: the VM compiles each function at its first call, so this cost is not part of the load (see `TODO.md`, "Performance").

**Load-time check (19 October 2026).** `vm.Load` now compiles every function the plugin can reach, on a machine it then drops, and refuses the plugin when its own code holds something the VM cannot run (`vm/preflight.go`). On the 93 middlewares, in default mode and in `-aot`: the 90 plugins that loaded still load, none is refused by the check, and the 3 load failures are the ones of the table above (Aetherinox, coraza-traefik, cors-preflight). The full harness gives the same verdict for every plugin in both modes (90 same, 1 yaegi v2 failure, 2 both fail). Cost of the check (`cmd/loadtime -n 9`, the `check` column, the plugin's machine is not touched): 0.5 ms for plugindemo and plugin-rewritebody, 7 ms for geoblock and crowdsec-bouncer, 25 ms for traefik-oidc-auth, 94 ms for traefikoidc (72 ms in `-aot`), out of 443 ms for its whole load on this machine. Memory kept after the load of traefikoidc: 41.4 MB against 40.4 MB without the check (compiling on the plugin's own machine kept 50 MB: the code of functions that never run). The loader also sets the `purego` build tag: it changes the files of 3 plugins (traefikoidc, darkweak/souin, MuXiu1997/traefik-github-oauth-plugin vendor xxhash, x/crypto, circl or protobuf), with the same verdicts.

A limit that appeared during task 6 and was removed in phase 2 bis: the native registry came from the yaegi v0.16.1 symbols, which stop at the Go 1.22 API (`strings.SplitSeq` was "not in registry"). It is now generated for the Go version of the host (`stdsyms/`, `cmd/stdsyms`), with the yaegi restrictions.

## Credentials, providers and reloads (19 October 2026)

"Identical to yaegi" is not "correct". The middleware scenario sent a fake JWT, so no plugin ever took its valid-credential path, checked a real signature or decoded real claims. The 4 provider plugins were out of scope, and nothing rebuilt a middleware the way a configuration reload does. Three scenarios cover this now (harness and method: `compare/catalog/README.md`, section "Other scenarios"; data: `compare/catalog/scenarios/`; raw results: `compare/catalog/results/auth.tsv`, `reload.tsv`, `provider.tsv`, `scenarios.txt`). Same hosts as the middleware scenario: native build (the oracle, run twice), yaegi v0.16.1 configured like Traefik, yaegi v2, yaegi v2 `-aot`.

| scenario | plugins | native output: yaegi | yaegi v2 | yaegi v2 -aot | yaegi v2 against yaegi (default and -aot) |
|---|---|---|---|---|---|
| auth (valid and invalid credentials) | 17 configured, 2 not | 15 | 16 | 16 | 15 same, 1 yaegi v2 fails (Aetherinox, invalid Go), 1 yaegi fails |
| reload (200 rounds of CreateConfig + decode + New, then requests) | 93 | 87 | 88 | 87 | default: 89 same, 1 yaegi v2 fails (Aetherinox), 1 yaegi fails, 2 both fail; -aot: 88 same, 2 yaegi v2 fail (Aetherinox, and holysoles/bot-wrangler-traefik-plugin on a network answer, see below) |
| provider (NewProvider, Init, Provide, Stop) | 4 | 3 | 4 | 4 | 3 same, 1 yaegi fails |

Measured on main 3c3d930 merged into the branch. In short: on every valid-credential path that the harness can reach locally, yaegi v2 gives the native output in both modes. The JWT paths go through golang-jwt v4 and v5 (`jwt.Parse` with `MapClaims`, `NumericDate`, `ParseWithClaims` patterns) and through hand-written verifiers; they are right in default mode, which confirms the fixes of the JWT claims on real plugins. Two yaegi v2 differences show up that the transcripts do not count (see "yaegi v2 bugs found" below).

### auth

Each credential plugin gets a configuration in `scenarios/auth.yml` that trusts a local key, a local issuer or a local mock of its service. The runner signs the tokens itself (RS256 with a test key served as a JWKS, HS256 with a test secret): valid, expired, bad signature, minimal claims, `exp` with a fraction (NumericDate), wrong issuer, wrong audience, not yet valid, fresh (recent `iat`), `alg: none`, garbage, no token. The OIDC plugins also run the code flow against the local provider: login redirection, callback with the code (the local `/token` puts the nonce of the redirection in the ID token), then a request with the session cookie.

| plugin | class | what is tested | valid path reached in native | yaegi | yaegi v2 | yaegi v2 -aot |
|---|---|---|---|---|---|---|
| 23deg/jwt-middleware | jwt | HS256 token variants | yes | ok | ok | ok |
| agilezebra/jwt-middleware | jwt | RS256 variants, keys from the local issuer (discovery + JWKS), `require` on aud and groups, claims to headers | yes | ok | ok | ok |
| traefik-plugins/traefik-jwt-plugin | jwt | RS256 variants, PEM key and JWKS, claims to headers | yes | ok | ok | ok |
| MuXiu1997/traefik-github-oauth-plugin | oauth | HS256 session cookie (golang-jwt v4), the `/_auth` callback through its mocked server, then the new cookie | yes | ok | ok | ok |
| luizfonseca/traefik-github-oauth-plugin | oauth | same, with `exp` as `jwt.NewNumericDate` and teams | yes | run | ok | ok |
| sevensolutions/traefik-oidc-auth | oidc | bearer access tokens (JWKS), claim assertion, templated headers, code flow and session | yes | ok | ok | ok |
| lukaszraczylo/traefikoidc | oidc | bearer path (`enableBearerAuth`), code flow and encrypted session | yes | ok | ok | ok |
| Gwojda/keycloakopenid | oidc | Keycloak paths on the local issuer, code flow, tokens checked by introspection | yes | ok | ok | ok |
| andrewkroh/google-oidc-auth-middleware | oidc | partial: redirection to Google, forged cookie, bad callback | no (Google endpoints are constants) | ok | ok | ok |
| xabinapal/traefik-authentik-forward-plugin | forward-auth | mocked authentik outpost, session cookie, redirect paths | yes | ok | ok | ok |
| fosrl/badger | forward-auth | mocked Pangolin API, session cookie, session exchange | yes | ok | ok | ok |
| ZeroGachis/traefik-auth-middleware | basic | user and password exchanged at the local token endpoint (password grant) | yes | ok | ok | ok |
| leonjza/trauth | basic | htpasswd with bcrypt and `{SHA}`, then the gorilla session cookie | yes | ok | ok | ok |
| dtomlinson91/traefik-api-key-middleware | apikey | header and bearer keys | yes | ok | ok | ok |
| Septima/traefik-api-key-auth | apikey | header, bearer, query and path keys | yes | ok | ok | ok |
| paladium/traefikkeycloak | other | cookie presence only (its verification is commented out) | yes | ok | ok | ok |
| Aetherinox/traefik-api-token-middleware | apikey | header and bearer tokens | native does not build (invalid Go) | diff | load | load |

Not configured, with the reason (they stay in `results/auth.tsv`): wiltonsr/ldapAuth (needs an LDAP server, the harness has no LDAP mock), hhftechnology/tailscale-access (no credential: it checks the Tailscale address range of the client). andrewkroh/google-oidc-auth-middleware is only partly covered (above).

Fixed in the data files while setting up, not in the engines: traefik-plugins/traefik-jwt-plugin fetches its JWKS in a goroutine started by `New`, so the result of the first requests depended on whether that goroutine had finished (the native and the interpreted runs differed for this reason only). `ForceRefreshKeys: true` makes the lookup of an unknown `kid` synchronous. The second native run now starts 1.5 s after the first, so that plugins which put the time in seconds in a cookie (trauth) are seen as nondeterministic; their transcripts are then compared coarsely, and the cookie contents were checked by hand (next section).

### provider

The 4 provider plugins run as Traefik runs them (`pkg/plugins/providers.go`): yaegi through the same `wrapper` package and `PP` interface, yaegi v2 through `Plugin.Call` of `New` and `Object.Call` (`internal/pluginhost`), native through a direct call. The APIs they poll are mocked (`scenarios/providers.yml`): Proxmox VE (one node, a running and a stopped VM, a container), Service Fabric (a stateful service with labels in an XML extension, a partition, two replicas), another Traefik (`/api/rawdata`). The first three configurations sent on the channel are compared as JSON with sorted keys.

| plugin | native | yaegi | yaegi v2 | yaegi v2 -aot |
|---|---|---|---|---|
| traefik/pluginproviderdemo | ok | ok | ok | ok |
| NX211/traefik-proxmox-provider | ok | ok | ok | ok |
| im-kulikov/traefik-provider | ok | ok | ok | ok |
| dariopb/traefikServiceFabricPlugin | ok | diff: no message, a panic in vendored `github.com/jjcollinge/servicefabric` (`withContinue`) inside the polling goroutine | ok | ok |

pluginproviderdemo adds a router when the current minute is even: it is compared on the stages and the number of messages only (`coarse` in providers.yml). Its emitted JSON was identical across the four engines in this run.

### reload

For each of the 93 middlewares: 200 rounds of `CreateConfig`, decoding and `New`, with in turn the manifest testData, the defaults of `CreateConfig` alone and the auth configuration when there is one, then the 7 requests of the middleware scenario through a last handler. The verdicts are the same as in the middleware scenario (snapt/traefik-nova-plugin and ciaranj/cacheify are identical but not native, as before), plus libops/captcha-protect where yaegi loses the error of `New` ("invalid handler type <nil>" instead of "siteKey is required"), and holysoles/bot-wrangler-traefik-plugin in `-aot`: its `New` downloads a list from cdn.jsdelivr.net, and at rounds 176 and 178 the proxy of the measuring machine answered "Bad Gateway" then "Forbidden" where it had answered "Forbidden" before. This is the network, not the engine (the first run of this branch, before the merge of main, had this plugin identical in both modes): the `yaegi v2 fails` of this row in `results/reload.tsv` and in the table above is not a yaegi v2 failure. The comparison was not changed to hide it; instead, since the review of this branch, the hosts of the scenarios use a local proxy that refuses every internet host (`compare/catalog/README.md`, "Other scenarios"), so such a plugin gets the same answer in every round and engine. The committed results were measured before this change and were not measured again (the plugin sources are not on the review machine): an open task in `TODO.md`. A panic in `New` is compared on the fact only, as a panic during a request (native and yaegi report "panic: ...", yaegi v2 "vm: <value>": "vm: panic: ..." for `panic(x)`, "vm: runtime error: ..." for a runtime error, "vm: regexp: Compile(...)" for a panic of a standard library function; darkweak/souin panics on the empty configuration in all engines). Every "vm: " error of `New` counts as a panic, except the known yaegi v2 failures (budget, stack overflow and the other errors of the vm package, which are "vm: vm...", Goexit, nesting limit of recursive types, unsupported construct, method refused at run time): these are not panics and stay compared as they are (`newPanicMsg` in `compare/catalog/run/scenarios.go`).

Leaks beyond what native leaks: yaegi on libops/captcha-protect (2.5 goroutines per round against 1.5), and yaegi v2 `-aot` on lukaszraczylo/traefikoidc for the heap, just over the threshold (79.1 KiB per round against 14.3 for native; in the first run native kept 25.5 and `-aot` 76.4, under the threshold). The second one is the pattern of the next table, not a separate leak. 13 plugins start goroutines in `New` that never stop (a timer, a cleaner, a refresh loop): native leaks them too, one handler per round. The live heap that such a handler keeps is larger with yaegi v2:

| plugin | goroutines per round (all engines) | heap per round, KiB: native | yaegi | yaegi v2 | yaegi v2 -aot |
|---|---|---|---|---|---|
| hhftechnology/ipwhitelistshaper | 2 | 3.7 | 15.3 | 60.2 | 60.2 |
| hhftechnology/traefik-queue-manager | 2 | 2.8 | 12.5 | 58.6 | 58.5 |
| astappiev/traefik-umami-feeder | 1 | 9.9 | 16.0 | 55.0 | 55.0 |
| darkweak/souin | 1.5 | 1.7 | 30.3 | 43.0 | 43.1 |
| lukaszraczylo/traefikoidc | 2.5 to 3.1 | 14.3 | 59.4 | 75.2 | 79.1 |
| l4rm4nd/traefik-warp | 1 | 1.9 | 8.1 | 30.5 | 30.5 |
| hhftechnology/bandwidthlimiter | 1 | 1.6 | 5.2 | 30.1 | 30.1 |

The other 80 middlewares keep a flat heap and a flat number of goroutines in every engine. Traefik rebuilds the middlewares at each change of the dynamic configuration, so with such a plugin each reload costs about 2 to 4 times more memory under yaegi v2 than under yaegi (median 30 KiB against 12).

### yaegi v2 bugs found

The parent session opens the fix PRs; repros below are complete `main` packages (`go run` against `go run ./cmd/yaegi`).

1. **gob: the wire type of an interpreted struct has no name (default mode).** leonjza/trauth stores a `User` in a gorilla session, which gob-encodes it. Under yaegi v2 the type is registered with the right name (`github.com/leonjza/trauth.User`) but its struct description is anonymous (`\x01\x02` instead of `\x01\x01\x04User`). The session still works inside yaegi v2, so the transcript does not show it (the cookie is time-dependent and compared coarsely); a cookie written by yaegi v2 differs from the one native writes. `-aot` is right. yaegi registers the type under the name `struct { Username string; Authenticated bool }`, which is worse.
   ```go
   package main

   import ("bytes"; "encoding/gob"; "fmt")

   type User struct { Username string; Authenticated bool }

   func main() {
   	gob.Register(User{})
   	var b bytes.Buffer
   	gob.NewEncoder(&b).Encode(map[string]any{"user": User{"user42", true}})
   	fmt.Printf("%q\n", b.Bytes()) // yaegi v2: ...\xff\x81\x03\x01\x02\xff\x82... go run: ...\xff\x81\x03\x01\x01\x04User\x01\xff\x82...
   }
   ```
2. **json: `UnmarshalTypeError.Type` loses the named type (default mode).** Seen on MuXiu1997/traefik-github-oauth-plugin while its mock was misconfigured (its error message goes into the 500 response): yaegi v2 and yaegi print `struct { Message string "json:\"msg\"" }` where native prints `model.ResponseError`. `-aot` is right. The final configuration does not reach this error path any more, so the tables do not count it.
   ```go
   package main

   import ("encoding/json"; "fmt")

   type ResponseError struct { Message string `json:"msg"` }

   func main() {
   	var e ResponseError
   	fmt.Println(json.Unmarshal([]byte(`5`), &e)) // go run: ...of type main.ResponseError; yaegi v2: ...of type struct { Message string "json:\"msg\"" }
   }
   ```
3. **Memory kept per plugin instance** (reload table above): 14 to 79 KiB per leaked handler under yaegi v2 (median 30), against 2 to 59 KiB under yaegi (median 12) and 0.5 to 14 KiB native (median 2), on the 13 plugins that leak a handler per round. Not a correctness bug, but it multiplies the cost of a plugin that leaks goroutines on each reload. Repro: `go run ./compare/catalog/run -scenarios reload -only ipwhitelistshaper`, then `out/hhftechnology_ipwhitelistshaper/reload-*.times`.

yaegi bugs found on the way (nothing to fix here, they are differences yaegi v2 does not have): the Service Fabric provider panics in its polling goroutine; luizfonseca/traefik-github-oauth-plugin signs `{"exp":{}}` (the `MarshalJSON` of `jwt.NumericDate` is lost on the yaegi side) and then rejects its own cookie; libops/captcha-protect loses the error returned by `New`.

### Limits

- The credentials are only the ones a local setup can provide: no LDAP server, no Google or GitHub, no mTLS client certificates (smerschjohann/mtlswhitelist is not covered), no Redis session store.
- The OIDC provider is minimal: it does not check the client secret, PKCE or the redirect URI, and it signs every code; a plugin that relies on the provider for these checks is only tested on its own checks.
- Tokens with fixed dates (valid until 2100, `iat` in 2023) make the transcripts reproducible between runs; plugins that bound the age of `iat` (lukaszraczylo/traefikoidc) are tested with the `fresh` variant and the code flow, whose tokens change at each run.
- The reload rounds run one after the other, in one goroutine; concurrent requests during a reload belong to the streaming and concurrency scenarios.
- Leak thresholds: 0.5 goroutine or 64 KiB per round beyond native. The heap of native itself varies between runs (traefikoidc: 25.5 then 14.3 KiB per round), so a value near the threshold can change class between runs.
- Plugins that download from the internet in `New` (bot-wrangler, cloudflare lists, etc.) went through the proxy of the measuring machine in the committed run; its answers can change during a run of 200 rounds. The runner now gives these hosts a local proxy that refuses every internet host, so the next run is stable (clients that ignore the proxy settings of the environment are not covered).
- The reload rounds share one context, canceled at the end only: the scenario compares the goroutines kept with native, it does not check that an old instance stops when its context ends. `vmhost` loads without `Sandbox` or budget, so the limits after many reloads are not checked. Both are measured since 23 October 2026 (reloadcancel and reloadsandbox, at the top).
- Provider: the goroutines one second after `Stop` are now compared with native (more than 2 beyond native is a leak, `leak_*` columns of `provider.tsv`). The committed `provider.tsv` predates these columns; by its recorded counts (native 1→3, 1→1, 1→3, 1→9 on the four providers, yaegi v2 and `-aot` the same, yaegi 1→1 on Service Fabric) no engine keeps more than native.

## Extra scenarios: headers, streaming, concurrency, soak (19 October 2026)

The base scenario covered less than it seemed: one `New`, the `testData` only, and 7 sequential requests into an `httptest.ResponseRecorder`, one plugin per process. It could not see the lost header writes or the lost `Flusher` and `Hijacker` (both fixed since). Four scenarios were added (`compare/catalog/vmhost/driver_extra.go` and `compare/catalog/run/extra.go`, method in `compare/catalog/README.md`, "Extra scenarios"):

- **headers**: 5 requests with several values per header; next echoes the request headers and sets response headers with several values (`Set-Cookie`, `Vary`...).
- **stream**: a real `httptest.Server`. SSE: next writes, flushes, waits until the client has read the first chunk (2 s at most), writes again. WebSocket style: raw HTTP/1.1 Upgrade, next hijacks, answers 101 and echoes one line.
- **conc**: 32 goroutines x 50 requests through one handler; every host, the native one included, is built with `-race`. The set of distinct responses of each request is compared, not their counts. A data race in the engine gives the verdict `race` (a failure of that engine), even when the output is the native one: a race report with a frame of the engine (`github.com/traefik/yaegi/v2/`, `github.com/traefik/yaegi/`) while neither native run reported a race. The native runs are needed to tell an engine race from a race of the plugin's own code, since the engine runs that code and its frames are in the reports of both (`markEngineRaces` in `run/extra.go`).
- **soak**: all the plugins that pass the base scenario in every engine, in one process per engine, 8 goroutines of traffic for 10 minutes; heap after GC, RSS and goroutines sampled.

Raw data: `compare/catalog/results/scenarios.tsv`, `scenarios-summary.txt`, `soak.txt`, `soak-*.tsv`. Same plugins and versions as above (93 middlewares). Measured on main of 19 October 2026 evening (after the load preflight, the Go version guard, the plugin stdio and restrict-reflect merges); a first measure on the main of the morning gave the same classes, within the random cases listed below.

### Results

Verdicts against native (93 plugins per scenario; `applies` = the scenario tests its topic for the plugin, from the native transcript):

| scenario | applies | yaegi ok | yaegi v2 ok | yaegi v2 -aot ok | class yaegi v2: same / yaegi v2 fails / different / both fail | class -aot |
|---|---|---|---|---|---|---|
| headers | 53 | 87 | 87 | 87 | 90 / 1 / 0 / 2 | 90 / 1 / 0 / 2 |
| stream | 52 | 83 | 82 | 82 | 84 / 2 / 5 / 2 | 84 / 2 / 5 / 2 |
| conc | 52 | 69 (17 race) | 86 | 82 (4 race) | 70 / 1 / 1 / 3, and 18 yaegi fail | 68 / 3 / 1 / 5, and 16 yaegi fail |

The 41 or so `n/a` lines are not skipped: they run and are compared, but the native plugin never calls next (36 auth or allowlist plugins without valid credentials or with a client address they refuse, the noop plugin...) or stops at New (3, network refused by the proxy) or does not build (1). Valid credentials are the topic of the auth scenario (section "Credentials, providers and reloads" above).

What survives the plugin in the stream scenario (52 plugins where it applies; flush = the client saw the first SSE chunk before the end; hijack = 101 and echo):

| plugins | flush native / yaegi / yaegi v2 / -aot | hijack native / yaegi / yaegi v2 / -aot |
|---|---|---|
| 39 | yes / yes / yes / yes | yes / yes / yes / yes |
| 3 (tomMoulard/fail2ban, XciD/traefik-plugin-rewrite-headers, cookielab/traefik-middleware-request-logger) | yes / no / no / no | yes / yes / no / no |
| 3 (traefik/plugin-rewritebody, dirtycajunrice/subfilter, tomMoulard/htransformation) | no / no / no / no (the plugin buffers) | yes / yes / no / no |
| 1 (darkweak/go-esi) | yes / no / no / no | no / no / no / no |
| 3 (Lepkem/traefik-plugin-response-code-override, hhftechnology/bandwidthlimiter, traefik/plugin-simplecache) | no everywhere | no everywhere |
| 2 (jptosso/coraza-traefik, Medzoner/traefik-plugin-cors-preflight) | yes natively, do not load in any engine | |
| 1 (ciaranj/cacheify) | cache on disk between processes, see below | |

Races in `conc` (plugins with at least one `WARNING: DATA RACE`): native 8, yaegi 25, yaegi v2 8, yaegi v2 -aot 12. The 8 of yaegi v2 default mode are the 8 of native (races of the plugins themselves: darkweak/go-esi, darkweak/souin, ghnexpress/traefik-ratelimit, hhftechnology/bandwidthlimiter, nzin/traefik-cluster-ratelimit, traefik/plugindemo, unsoon/traefik-open-policy-agent, wiltonsr/ldapAuth), and no report has a frame in the shadows of the VM. The 4 more of `-aot` are a yaegi v2 bug (G2 below): they have the verdict `race` (agilezebra/jwt-middleware, PascalMinder/geoblock, traefik-plugins/traefik-jwt-plugin, ciaranj/cacheify). yaegi has 17 more than native (21 in the first measure), in yaegi itself (not studied further: yaegi is the reference we replace, not the subject); they have the verdict `race` too.

The `race` verdicts of the committed `scenarios.tsv` and `scenarios-summary.txt` were not measured again: the rule came with the review of the branch, after the measure, and the plugin sources are not on the review machine. They were derived from the race columns of the measured run with the rule of `markEngineRaces` (an engine with race reports, verdict `ok` or `diff`, `races_native` 0, native built and not crashed). The file has no column for the races of the second native run, so a plugin that raced only in it would have no `race` verdict in a new run; the frames of the reports were not checked either (every report of an engine host has engine frames, see above). A new run with `-reuse` on the same `out/` gives the exact values. (Measured since, on 23 October 2026: see the top of this document.)

Soak (`results/soak.txt`), 79 plugins running in each process (81 selected, 2 excluded because they end in a fatal error under concurrency, natively too: unsoon/traefik-open-policy-agent and darkweak/go-esi, concurrent map writes):

| engine | heap after load | heap at 1/4 | heap at the end | goroutines 1/4 / end / after cancel | requests in 10 min |
|---|---|---|---|---|---|
| yaegi | 354 MB | 356 MB | 357 MB | 72 / 39 / 19 | 96 419 |
| yaegi v2 | 434 MB | 447 MB | 442 MB | 76 / 51 / 34 | 110 600 |
| yaegi v2 -aot | 433 MB | 440 MB | 440 MB | 69 / 38 / 20 | 108 306 |

No leak in any engine: the heap after GC is flat after warm up, and the goroutines go back near their count after the load (27 or 28) once the context of New is canceled; yaegi v2 default keeps about 14 more after cancel (and already 13 more at the end of the traffic). Studied since: they are not a leak, but the idle keep-alive connections (two goroutines each) of the DefaultTransport that each machine has (a clone of the host's, `vm/hostregs.go`), where yaegi and the AOT copies share the host's; they close after `IdleConnTimeout` (90 s) and, since, when the machine is gone (`TestGoroutinesAfterCancel`). yaegi v2 holds about 85 MB more than yaegi for 79 plugins (the stdlib types and the compiled code). No panic in this run; 1 yaegi timeout (chaitin/traefik-safeline) and 2 `-aot` timeouts (xabinapal/traefik-authentik-forward-plugin, a request to a host that does not answer). In the first measure, traefik/plugin-simplecache panicked in every engine (it also panics natively under `conc`).

### yaegi v2 failures, by cause

**G1. An interpreted ResponseWriter loses `Flush` and `Hijack` when it goes to native code** (7 plugins: tomMoulard/fail2ban, XciD/traefik-plugin-rewrite-headers, cookielab/traefik-middleware-request-logger, traefik/plugin-rewritebody, dirtycajunrice/subfilter, tomMoulard/htransformation, darkweak/go-esi; both modes). The wrapper types of these plugins define `Flush` and `Hijack`; native next only sees `Header`, `Write` and `WriteHeader`. yaegi keeps `Hijack` (a composed wrapper) but loses `Flush` too. Fixed since (`vm/rwwriter.go`): a vmhost built from the fix gives the native transcript for the 7 plugins, in both modes. Repro:

```go
type wrapper struct{ http.ResponseWriter }
func (w wrapper) Flush() { fmt.Println("Flush called") }
func (w wrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) { return nil, nil, errors.New("hijack reached the wrapper") }

func main() {
	rc := http.NewResponseController(wrapper{httptest.NewRecorder()})
	fmt.Println("flush:", rc.Flush())
	_, _, err := rc.Hijack()
	fmt.Println("hijack:", err)
}
```

`go run`: `Flush called`, `flush: <nil>`, `hijack: hijack reached the wrapper`. yaegi v2 and yaegi v2 -aot: `flush: feature not supported`, `hijack: feature not supported`.

**G2. `-aot`: the shadows of the VM cells are shared by all the goroutines of a machine** (agilezebra/jwt-middleware: wrong responses, `token is unverifiable: signing method (alg) is unspecified`; data races without wrong output seen in PascalMinder/geoblock, ciaranj/cacheify, traefik-plugins/traefik-jwt-plugin). `vm/twin_value.go`: when AOT code takes the address of a cell that holds a native named type in VM representation (`json.RawMessage`), the cell gets a `shadowRef`, recorded in `m.shadows`, one list per `Machine`. `flushShadows` runs at every native to VM transition, in any goroutine, and detaches all the shadows of the machine, also the ones that AOT code of another goroutine is still writing (race between `shadowRef.detach`/`fromNativeDeepC` and `aot/encoding/json.(*decodeState).object`). Default mode is not affected. Repro (no `-race` needed to see the wrong result):

```go
func main() {
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				var doc struct {
					Keys json.RawMessage `json:"keys"`
				}
				if err := json.Unmarshal([]byte(`{"keys":[1,2,3]}`), &doc); err != nil || string(doc.Keys) != "[1,2,3]" {
					panic(fmt.Sprint(err, string(doc.Keys)))
				}
			}
		}()
	}
	wg.Wait()
	fmt.Println("ok")
}
```

`go run` and yaegi v2: `ok`. yaegi v2 -aot: `panic: <nil>` (an empty `Keys`), on every run; with a `-race` build of yaegi v2, `WARNING: DATA RACE` in `shadowRef.detach`.

**Not yaegi v2**: Aetherinox/traefik-api-token-middleware (invalid Go, as in the base scenario).

### Differences that come from the environment or from timing

- ciaranj/cacheify keeps its cache in a file that outlives the process: the first host of a scenario gets a miss, the next ones a hit (already the case in the base scenario).
- darkweak/souin (`conc`): an HTTP cache with a TTL in seconds; under `-race` the slower engines get more hits and `Age` headers. One yaegi request timed out.
- darkweak/go-esi (`conc`): the plugin writes a map without a lock; both native runs and yaegi ended in `fatal error: concurrent map writes`, yaegi v2 did not this time (class "yaegi fails"). In the first measure only one native run crashed. This is luck, not a difference of the engines.
- acouvreur/traefik-ondemand-plugin, adyanth/traefik-container-manager, hhftechnology/traefik-queue-manager (`conc`): random session ids, or the wording of a failed DNS lookup under load (which resolver, or a client timeout first). The two native runs differ too, so the coarse sets are compared, and they are equal.

### Known limits

- No native host for the soak: one native binary with all the plugins would need one module with all their dependencies. A leak would show as growth in every engine; `-soakonly` and a native single plugin soak (not implemented) would then tell which plugin leaks.
- In the soak process, 2 plugins fail in New in every engine because they open files with a path relative to the working directory (negasus/traefik-plugin-ip2location, JonasSchubert/traefik-allow-countries): the soak process does not run in the plugin directory.
- The headers scenario finds no yaegi v2 difference on the catalog. The bug of the lost in-place header write (`r.Header[k][i] = v`) is caught by the harness test (`TestHeadersScenario`, `vmhost/testdata/streamplug`, which fails before the fix), but no catalog plugin writes headers in place this way.
- `conc` compares sets of responses; a plugin whose output depends on the order of requests (rate limits) can still differ for reasons of timing. None did in this measure, except the cases above.
- The `-race` builds slow the engines down differently, so timing dependent plugins are less comparable under `conc` than in the base scenario.
- An engine race in a plugin that also races by itself (native reports a race) does not give the verdict `race`: the reports stay in the race counts only. yaegi v2 default mode has no race beyond the plugins' own in this measure.
- The committed results were measured with the network of the measuring machine. Since the merge with the credential scenarios, the hosts of headers, stream, conc and the soak use the local proxy that refuses every internet host, as the auth, reload and provider ones (`noNetEnv` in `run/scenarios.go`); not measured again. (Measured since, on 23 October 2026, behind the proxy: see the top.)

## First measure (task 1, before the fixes)

The first measure did not pass the gate: yaegi gave the native output for 88 plugins, yaegi v2 for 67 (67 also in `-aot`). The next sections describe this measure and its causes. The full table at the end of the document gives the current status.

## Scope

- **Source of the list**: plugins.traefik.io cannot be reached from the measuring machine (proxy). The list is the 100 most starred repositories of the GitHub topic `traefik-plugin` (988 repositories in total), read on 17 October 2026: `compare/catalog/plugins.txt`. This sample is biased toward popular plugins, so toward the ones that matter.
- **Version**: the latest semver tag of each repository (the catalog serves tagged versions), otherwise the default branch (2 repositories).
- **Out of scope**: 4 providers, 2 wasm plugins, 1 repository without `.traefik.yml`. This leaves 93 yaegi middlewares.
- **Depth**: the native reference handles the 7 requests for 87 plugins. For 5 others, `New` also fails in native (network access to Cloudflare, jsDelivr, etc., refused by the proxy) or a request panics; these plugins only test loading and configuration. One plugin does not compile in native (see below). 5 plugins have a non-deterministic native output (identifiers, nonces): in this case only the steps, the statuses and the header names are compared.

## Method in short

Each plugin goes through four hosts that share the same driver (`compare/catalog/vmhost/driver.go`):
- a native binary (reference, run twice);
- yaegi v0.16.1 configured like Traefik (`pkg/plugins/middlewareyaegi.go`: GOPATH, `stdlib.Symbols`, unsafe and syscall only with `useUnsafe`);
- yaegi v2 in default mode;
- yaegi v2 `-aot`.

Everywhere, mapstructure decodes `testData` into the result of `CreateConfig`, with the Traefik options. Then 7 requests go through the handler. The transcript (step reached, status, headers, body, what the next handler receives) is compared byte by byte, except HTTP dates.

Fairness: yaegi v2 only sees the plugin repository and its `vendor/`, like yaegi in Traefik. In the first measure, its loader (`go list`) ran with `GOPROXY=off` and a module cache that only held Go toolchains. Since task 6, it reads the repository directly, and the yaegi v2 hosts run without `go` in the PATH and without `GOROOT`.

Two fixes in `vm/plugin.go` were needed to make the measure meaningful (tests in `vm/plugin_test.go`):
- `Load` treated all non-stdlib dependencies as native. It now interprets them, like `RunMain`, including the plugin subpackages. Without this, 9 more plugins failed with "native func not in registry".
- `Object.Decode` (`Plugin.DecodeConfig` at the time) lets a native decoder (mapstructure) fill the interpreted configuration.

## Causes of the yaegi v2 failures in the first measure

The 22 plugins classified as "yaegi v2 fails" (default mode):

| cause | plugins | count |
|---|---|---|
| A. Native `ServeHTTP` called with a `ResponseWriter` from the plugin: the fast path of `vm/natfast.go` passes `nil` instead of falling back to the general conversion | traefik/plugin-rewritebody, traefik/plugin-simplecache, tomMoulard/htransformation, tomMoulard/fail2ban, packruler/rewrite-body, packruler/traefik-themepark, XciD/traefik-plugin-rewrite-headers, safing/scanblock, juitde/traefik-plugin-fail2ban, hhftechnology/bandwidthlimiter, dirtycajunrice/subfilter, cookielab/traefik-middleware-request-logger | 12 |
| B. `go list` loader: vendoring inconsistent with `go.mod` (darkweak/go-esi), module graph incomplete when offline (Lepkem/traefik-plugin-response-code-override), `go.mod` in Go 1.27 while yaegi v2 is compiled with 1.26 (zekihan/traefik-real-ip) | 3 plugins | 3 |
| C. Native boundary: named type lost in a native round trip, "interface is *struct {...}, not *T" (darkweak/souin, negasus/traefik-plugin-bridge); interpreted channel passed to native code (lukaszraczylo/traefikoidc); `reflect.StructOf` on an unexported embedded field (libops/captcha-protect); native type `net.conn` missing from the registry (MarkusJx/traefik-wol) | 5 plugins | 5 |
| D. Generic stdlib function called outside `-src`: `slices.Contains` (blueshift-labs/traefik-block-regex-urls) | 1 plugin | 1 |
| E. Invalid Go code (unused variables) that yaegi accepts and that the Go compiler also refuses (Aetherinox/traefik-api-token-middleware) | 1 plugin | 1 |

**Cause A, checked.** `vm/natfast.go`, case `"ServeHTTP"`: `rw, _ := natAny(regs[a[0]]).(http.ResponseWriter)`. When the plugin wraps the `ResponseWriter` in its own type (body rewrite, cache, logging, rate limiting), the assertion fails, `rw` is `nil` and the next handler panics. Experiment outside the branch: when this fast path falls back (`return Value{}, false` if one of the two assertions fails), these 12 plugins give exactly the native output. This is the most common pattern in plugins that touch the response.

**Cause A** is fixed (commit "vm: fall back from the ServeHTTP fast path for interpreted writers", test `testdata/fastfallback.go`).

**Cause B** disappeared with task 6 (loading without `go list`, from the repository and `vendor/`).

**Cause E** is debatable. The plugin is not valid Go: a real compiler refuses it. Accepting this code would mean copying a yaegi tolerance.

In `-aot`, negasus/traefik-plugin-bridge and lukaszraczylo/traefikoidc pass. Two others fail: maxlerebourg/crowdsec-bouncer-traefik-plugin and wiltonsr/ldapAuth, on `reflect.Indirect` called by the plugin itself (the VM reflect does not provide it, task 7). Also, libops/captcha-protect crashes on `runtime.SetFinalizer` applied to an opaque value.

**Projection made at the time**: with A fixed and B removed by task 6, 7 yaegi v2 failures would remain (C, D, E). The real result is 6: D (`slices.Contains`) is also solved by the embedded sources of task 6.

## yaegi failures and shared cases

- **yaegi fails**: jptosso/coraza-traefik. yaegi does not load `github.com/jptosso/coraza-waf/v2`. yaegi v2 panicked in `New` (cause A); since the fix, it gives the native output.
- **Both fail**: Medzoner/traefik-plugin-cors-preflight. It imports `github.com/rs/zerolog` without vendoring it: the plugin cannot be used in Traefik.
- **Different**: ciaranj/cacheify. The three engines differed from native; it was cause A, and it is identical since the fix.
- **Identical but wrong**: snapt/traefik-nova-plugin. yaegi and yaegi v2 both panic where native returns 502.

## Load times (first measure)

Load time of a plugin (`load_ms`), on the 93 plugins:

| engine | median | 10th percentile | 90th percentile | max |
|---|---|---|---|---|
| yaegi | 39 ms | 13 ms | 127 ms | 806 ms |
| yaegi v2 | 562 ms | 308 ms | 6.9 s | 9.3 s |
| yaegi v2 -aot | 572 ms | 310 ms | 7.1 s | 9.5 s |

The yaegi v2 load includes `go list`. As soon as a plugin has a non-stdlib dependency, even a simple subpackage, it also includes type checking the whole stdlib from source (`LoadAllSyntax`), hence the 90th percentile at 7 s. This is the main argument for task 6.

## Reflect libs missing from `aot/`

`compare/catalog/deps.sh` lists, for each plugin, the non-stdlib packages that import `reflect` or `unsafe`. The result is small: catalog plugins depend very little on third-party reflect libs.

| lib | reflect / unsafe | plugins |
|---|---|---|
| github.com/golang-jwt/jwt | reflect | MuXiu1997/traefik-github-oauth-plugin, luizfonseca/traefik-github-oauth-plugin |
| github.com/google/go-querystring | reflect | MuXiu1997/traefik-github-oauth-plugin, unsoon/traefik-open-policy-agent |
| github.com/go-ldap/ldap, github.com/go-asn1-ber/asn1-ber | reflect | wiltonsr/ldapAuth |
| github.com/oschwald/maxminddb-golang | reflect | david-garcia-garcia/traefik-geoblock |
| github.com/ip2location/ip2location-go | unsafe | david-garcia-garcia/traefik-geoblock |
| go.uber.org/zap, go.uber.org/atomic | reflect, unsafe | jptosso/coraza-traefik |
| golang.org/x/xerrors | reflect | chaitin/traefik-safeline |
| github.com/rs/zerolog, golang.org/x/sys | reflect, unsafe | Medzoner/traefik-plugin-cors-preflight (not vendored) |

Already covered by `aot/`: gopkg.in/yaml.v3 and github.com/spf13/cast (darkweak/souin).

On the stdlib side, the reflect packages that matter are already in `aot/`: `encoding/json` (49 plugins), `text/template` (16), `html/template` (8), `log/slog` (6), `encoding/gob` (6), `encoding/xml` (1).

8 plugins use `reflect` in their own code: david-garcia-garcia/traefik-geoblock, holysoles/bot-wrangler-traefik-plugin, libops/captcha-protect, maxlerebourg/crowdsec-bouncer-traefik-plugin, renanqts/xdpfail2ban, sevensolutions/traefik-oidc-auth, wiltonsr/ldapAuth, zekihan/traefik-real-ip. This is the topic of task 7, more than AOT for new libs.

## Full table (23 October 2026)

Verdicts against native: `ok` (identical output), `diff` (runs, different output), otherwise the failing step (`load`, `config`, `new`, `run` for a panic or a timeout during a request, `crash`, `build` when the native binary does not compile).

| plugin | version | native | yaegi | yaegi v2 | yaegi v2 -aot | class | class -aot |
|---|---|---|---|---|---|---|---|
| 1cedsoda/traefik-umami-plugin | v1.0.3 | ok | ok | ok | ok | same | same |
| 23deg/jwt-middleware | v0.1.3 | ok | ok | ok | ok | same | same |
| acouvreur/traefik-modsecurity-plugin | v1.3.0 | ok | ok | ok | ok | same | same |
| acouvreur/traefik-ondemand-plugin | v1.3.1 | ok | ok | ok | ok | same | same |
| adyanth/traefik-container-manager | v0.1.2 | ok | ok | ok | ok | same | same |
| Aetherinox/traefik-api-token-middleware | v0.1.4 | build | diff | diff | diff | same | different |
| agence-gaya/traefik-plugin-blockuseragent | v0.1.8 | ok | ok | ok | ok | same | same |
| agence-gaya/traefik-plugin-cloudflare | v1.2.0 | ok | ok | ok | ok | same | same |
| agilezebra/jwt-middleware | v1.6.0 | ok | ok | ok | ok | same | same |
| andrewkroh/google-oidc-auth-middleware | v0.4.0 | ok | ok | ok | ok | same | same |
| astappiev/traefik-umami-feeder | v1.4.1 | ok | ok | ok | ok | same | same |
| BetterCorp/cloudflarewarp | v1.3.3 | ok | ok | ok | ok | same | same |
| blueshift-labs/traefik-block-regex-urls | v0.0.1 | ok | ok | ok | ok | same | same |
| chaitin/traefik-safeline | v1.4.0 | ok | ok | ok | ok | same | same |
| ciaranj/cacheify | v1.0.0 | ok | diff | diff | diff | same | same |
| cookielab/traefik-middleware-request-logger | v0.0.11 | ok | ok | ok | ok | same | same |
| darkweak/go-esi | v0.0.6 | ok | ok | ok | ok | same | same |
| darkweak/souin | plugins/beego/v1.7.9 | ok | ok | ok | ok | same | same |
| david-garcia-garcia/traefik-geoblock | v1.2.1 | ok | ok | ok | ok | same | same |
| Desuuuu/traefik-cloudflare-plugin | v1.0.0 | ok | ok | ok | ok | same | same |
| Desuuuu/traefik-real-ip-plugin | v1.1.0 | ok | ok | ok | ok | same | same |
| DIE-Bonn/MatomoTracking | v1.2.0 | ok | ok | ok | ok | same | same |
| dirtycajunrice/subfilter | v0.5.0 | ok | ok | ok | ok | same | same |
| discoverygarden/traefik-ultimate-bad-bot-blocker | v0.4.0 | ok | ok | ok | ok | same | same |
| dkijkuit/checkheadersplugin | v0.3.1 | ok | ok | ok | ok | same | same |
| dtomlinson91/traefik-api-key-middleware | v0.1.2 | ok | ok | ok | ok | same | same |
| fosrl/badger | v1.7.0 | ok | ok | ok | ok | same | same |
| ghnexpress/traefik-ratelimit | v0.0.20 | ok | ok | ok | ok | same | same |
| Gwojda/keycloakopenid | v0.1.36 | ok | ok | ok | ok | same | same |
| hhftechnology/bandwidthlimiter | v1.0.1 | ok | ok | ok | ok | same | same |
| hhftechnology/ipwhitelistshaper | v1.0.8 | ok | ok | ok | ok | same | same |
| hhftechnology/statiq | v1.0.1 | ok | ok | ok | ok | same | same |
| hhftechnology/tailscale-access | v2.0.0 | ok | ok | ok | ok | same | same |
| hhftechnology/traefik-queue-manager | v1.0.3 | ok | ok | ok | ok | same | same |
| holysoles/bot-wrangler-traefik-plugin | v0.10.2 | ok | ok | ok | ok | same | same |
| jdel/staticresponse | v0.0.1 | ok | ok | ok | ok | same | same |
| jeppestaerk/traefik-xff-to-xrealip | v0.1.3 | ok | ok | ok | ok | same | same |
| JonasSchubert/traefik-allow-countries | v1.2.0 | ok | ok | ok | ok | same | same |
| jptosso/coraza-traefik | HEAD | ok | load | load | load | both fail | both fail |
| jpxd/torblock | v0.1.1 | ok | ok | ok | ok | same | same |
| juitde/traefik-plugin-fail2ban | v0.3.0 | ok | ok | ok | ok | same | same |
| kingjan1999/traefik-plugin-query-modification | v1.0.0 | ok | ok | ok | ok | same | same |
| kvncrw/denyip | v1.3.0 | ok | ok | ok | ok | same | same |
| l4rm4nd/traefik-warp | v1.1.5 | ok | ok | ok | ok | same | same |
| leonjza/trauth | v1.6.7 | ok | ok | ok | ok | same | same |
| Lepkem/traefik-plugin-response-code-override | v0.0.3 | ok | ok | ok | ok | same | same |
| libops/captcha-protect | v1.15.1 | ok | ok | ok | ok | same | same |
| luizfonseca/traefik-github-oauth-plugin | v0.7.0 | ok | ok | ok | ok | same | same |
| lukaszraczylo/traefikoidc | v1.0.45 | ok | ok | ok | ok | same | same |
| madebymode/traefik-modsecurity-plugin | v1.6.0 | ok | ok | ok | ok | same | same |
| MarkusJx/traefik-wol | v0.1.1 | ok | ok | ok | ok | same | same |
| maxlerebourg/crowdsec-bouncer-traefik-plugin | v1.7.1 | ok | ok | ok | ok | same | same |
| Medzoner/traefik-plugin-cors-preflight | v1.3.0 | ok | load | load | load | both fail | both fail |
| Miromani4/traefik-plugin-AdminAPI_WebUI | v1.3.1 | ok | ok | ok | ok | same | same |
| MuXiu1997/traefik-github-oauth-plugin | v0.3.1 | ok | ok | ok | ok | same | same |
| negasus/traefik-plugin-bridge | v0.1.1 | ok | ok | ok | ok | same | same |
| negasus/traefik-plugin-ip2location | v0.1.0 | ok | ok | ok | ok | same | same |
| nscuro/traefik-plugin-geoblock | v0.14.0 | ok | ok | ok | ok | same | same |
| NX211/traefik-webfinger | v0.3.5 | ok | ok | ok | ok | same | same |
| nzin/traefik-cluster-ratelimit | v1.2.0 | ok | ok | ok | ok | same | same |
| packruler/rewrite-body | v1.2.0 | ok | ok | ok | ok | same | same |
| packruler/traefik-themepark | v1.4.2 | ok | ok | ok | ok | same | same |
| paladium/traefikkeycloak | v1.11 | ok | ok | ok | ok | same | same |
| PascalMinder/geoblock | v0.3.8 | ok | ok | ok | ok | same | same |
| Paxxs/traefik-get-real-ip | v1.0.4 | ok | ok | ok | ok | same | same |
| PseudoResonance/cloudflarewarp | v1.4.2 | ok | ok | ok | ok | same | same |
| renanqts/xdpfail2ban | v0.0.1 | ok | ok | ok | ok | same | same |
| sablierapp/sablier-traefik-plugin | v1.3.1 | ok | ok | ok | ok | same | same |
| safing/scanblock | v1.0.0 | ok | ok | ok | ok | same | same |
| Septima/traefik-api-key-auth | v0.3.0 | ok | ok | ok | ok | same | same |
| sevensolutions/traefik-oidc-auth | v0.21.0 | run | ok | ok | ok | same | same |
| smerschjohann/mtlswhitelist | v0.4.2 | ok | ok | ok | ok | same | same |
| snapt/traefik-nova-plugin | v1.1.2 | ok | run | run | run | same | same |
| solution-libre/traefik-plugin-robots-txt | v0.2.2 | ok | ok | ok | ok | same | same |
| taskmedia/ddns-allowlist | v1.7.0 | ok | ok | ok | ok | same | same |
| tomMoulard/fail2ban | v0.9.0 | ok | ok | ok | ok | same | same |
| tomMoulard/htransformation | v0.4.1 | ok | ok | ok | ok | same | same |
| tomMoulard/traefik-plugin-waeb | v1.0.1 | ok | ok | ok | ok | same | same |
| traefik-contrib/noop | v0.1.0 | ok | ok | ok | ok | same | same |
| traefik-plugins/traefik-jwt-plugin | v0.10.0 | ok | ok | ok | ok | same | same |
| traefik-plugins/traefikgeoip2 | v0.22.0 | ok | ok | ok | ok | same | same |
| traefik/plugin-blockpath | v0.3.0 | ok | ok | ok | ok | same | same |
| traefik/plugin-log4shell | v0.1.2 | ok | ok | ok | ok | same | same |
| traefik/plugin-rewritebody | v0.3.1 | ok | ok | ok | ok | same | same |
| traefik/plugin-simplecache | v0.2.1 | ok | ok | ok | ok | same | same |
| traefik/plugindemo | v0.2.2 | ok | ok | ok | ok | same | same |
| TRIMM/traefik-maintenance | v1.0.1 | ok | ok | ok | ok | same | same |
| unsoon/traefik-open-policy-agent | v1.2.1 | ok | ok | ok | ok | same | same |
| wiltonsr/ldapAuth | v0.1.12 | run | ok | ok | ok | same | same |
| xabinapal/traefik-authentik-forward-plugin | v1.0.1 | ok | ok | ok | ok | same | same |
| XciD/traefik-plugin-rewrite-headers | v0.0.4 | ok | ok | ok | ok | same | same |
| zekihan/traefik-real-ip | v0.1.25 | ok | ok | ok | ok | same | same |
| ZeroGachis/traefik-auth-middleware | v1.1.0 | ok | ok | ok | ok | same | same |
