# Results

Measured from October 17 to November 6, 2026, with yaegi v0.16.1 as the
reference. Method and details are in the linked documents. "yaegi" alone
means yaegi v0.

## Correctness

| Measure | yaegi v0.16.1 | yaegi v2 (VM mode) | yaegi v2 (AOT mode) |
|---|---|---|---|
| Catalog: 93 real Traefik middlewares, same output as the native binary ([CATALOG.md](CATALOG.md)) | 88 | 88 | 88 |
| Catalog: same output as yaegi | | 91 | 90 |
| 82 programs that copy what plugins do, same output as `go run` ([compare/yaegi/REPORT.md](../compare/yaegi/REPORT.md)) | 36 | 37 | 55 |
| 25 common reflect libraries, same output as `go run` ([AOTGEN.md](AOTGEN.md)) | 1 | | 24 |
| Native boundary: rows of the inventory whose probes give the output of `go run` (Go 1.26, [BOUNDARY.md](BOUNDARY.md)) | | 1487 / 1984 | 1657 / 1984 |
| yaegi conformance programs (`Output:` of yaegi master, [conformance/](conformance/)) | | 860 / 868 | |
| External tests of 16 standard library packages (fmt, sort, io, json...) that pass natively ([STDTESTS.md](STDTESTS.md)) | | 847 / 868 | 847 / 868 |

- On the middleware scenario of the catalog, yaegi v2 has no failure of its
  own: the plugin that is not valid Go loads with `IgnoreSoftErrors`, and
  the one AOT difference with yaegi is a timestamp. The other scenarios
  (credentials, reloads, streaming, concurrency, soak) find one yaegi v2
  failure in VM mode (a `json.Number` written as a string).
- In conformance, `type5.go` and `type6.go` print `main.T` like `go run`;
  their expected `int` is a yaegi limit.

## Load time and memory

| Measure | yaegi v0.16.1 | yaegi v2 (VM mode) | yaegi v2 (AOT mode) |
|---|---|---|---|
| Load of a catalog plugin, new process, median | 15 ms | 116 ms | 111 ms |

- About 85 ms of the first load is the decoding of the embedded standard
  library types, paid once per process. Next loads in the same process take
  6 to 24 ms. The largest catalog plugin (traefikoidc) loads in 226 ms
  (warm, `cmd/loadtime`; 52 ms of it is the load-time check).
- Memory kept after a load (`cmd/loadmem`, live heap after a GC): about
  16 MB for any plugin (the standard library types, shared), 37 MB for
  traefikoidc, and 51 MB for the Coraza WAF plugin with the OWASP CRS
  (110 MB once its WAF is built). After the load, only the SSA, the types
  and the syntax of generic functions stay (`vm/shrink.go`).
- Memory per in-flight request (`TestInflightHeap`: 1000 concurrent
  requests blocked in the next handler of a plugin, live heap after a GC):
  about 6 KB, as with yaegi.

## Speed

Time per request of Traefik-like plugins ([BENCHMARKS.md](BENCHMARKS.md),
µs, lower is better, on a shared machine):

| Workload | native | yaegi | yaegi v2 (VM mode) | yaegi v2 (AOT mode) |
|---|---|---|---|---|
| W1 headers | 2.1 | 12.2 | 4.7 | 4.7 |
| W2 JSON | 12.9 | 23.1 (wrong output) | 31.1 (correct) | 29.9 (correct) |
| W3 regexp | 1.23 | 6.1 | 3.1 | 3.2 |

Allocations per request: W1 75 (yaegi 255), W2 367 (yaegi 337), W3 65
(yaegi 149).

In AOT mode, a call into a library costs 1.9 to 20 times compiled Go
(median 4.9×; sort is an outlier at 52× because it calls the program back
at each comparison).
