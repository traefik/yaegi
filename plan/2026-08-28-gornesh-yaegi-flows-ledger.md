# Gornesh Yaegi flow hardening — ledger

## 2026-08-28

- Fork `jmcarbo/yaegi` cloned at upstream commit
  `fcb76d1ece0c3edc2548c39aa5b170475d2261bb`; initial worktree was clean.
- Created branch `jmca/gornesh-yaegi-flows`.
- Gornesh is currently dirty with user work and is treated as read-only input.
- Prior memory only established that Yaegi/time-context investigations had
  occurred; it explicitly lacked verified fixes. Current repository evidence
  and executable reproductions are the authority.
- Current Gornesh plans and `LEARNINGS.md` identify the initial candidate flows
  listed in the plan, including two newer uncommitted workarounds for replayed
  top-level IIFEs and lost multi-result bindings in top-level control bodies.
- Started three independent read-only reviews covering integration-flow
  inventory, Yaegi internals/root causes, and adversarial ownership/validation.
- The integration inventory confirmed twelve Yaegi-facing behaviors. It also
  excluded Gornesh-owned output shaping, host APIs, child lifecycle, budgets,
  provider streaming, workspace security, prompt time/location behavior, and
  model-authored map JSON choices.
- Baseline `go test ./interp` failed after 390 seconds because several existing
  tests require the checkout at `$GOPATH/src/github.com/traefik/yaegi`; all
  reported failures were that path precondition, not candidate-flow failures.
  Full closure must use a compatible temporary GOPATH layout.
- At the end of inventory, no Yaegi implementation changes had been made.
- Entrypoint/IIFE lane, pass@2: baseline focused tests reproduced retained
  `main` replay, IIFE replay/value corruption, and IIFE clobbering of `main`.
  The parser retry now returns the synthetic wrapper body, and `CompileAST`
  schedules `main` only when the current AST declares it. Focused tests plus
  existing entrypoint/REPL coverage pass under `-race -count=5`.
- The user explicitly confirmed that loop-local `id` and file/content variables
  reported as undefined are mandatory. They are tracked under the top-level
  control-body multi-result flow, with exact call-result loop regressions and
  downstream Gornesh file/spawn loop qualification required before closure.
- Generic-global lane, pass@4 after two review expansions: inferred and
  explicit generic calls are prepared recursively inside nested expressions
  and composite literals. Focused tests cover arithmetic, nested calls,
  distinct slice/map values, and missing/extra argument diagnostics without
  internal panics; focused race tests and existing generic fixtures pass.
- Incremental binary-import lane, pass@2: identical default, alias, dot, and
  blank imports are idempotent across separate Eval cells, while duplicate
  imports in one source and alias/dot collisions retain ordinary diagnostics.
  Failed Eval retry and existing redeclaration coverage pass under race.
- The first full temporary-GOPATH `go test ./...` attempt found three concrete
  integration failures: command cancellation setup raced later compilation,
  cross-Eval multi-result declarations persisted zero values, and the initial
  import fix accepted same-source duplicates. The import failure is fixed; the
  assignment failure remains in its active lane.
- Cancellation/lifecycle retry work replaced mutable run-id revival with an
  immutable per-execution owner, then added a serialized compiler/preparation
  boundary. Independent review exposed aliased frame slots, resizing the wrong
  root, writes after native calls returned, callback owner rebinding, and lost
  deferred cleanup. The current patch deep-copies frame slots, resizes the
  captured root, checks cancellation before native results publish, binds
  callbacks to their originating owner, and still runs deferred cleanup.
  Focused Eval, Execute, file-path, source-directory, callback, metadata-growth,
  and post-native publication tests pass under `-race -count=10` where added.
- The command cancellation race trace is gone. Its child-process test remains
  sensitive to a fixed 500 ms local startup delay: repeated local runs can be
  killed by the signal before the REPL installs its handler, while the test's
  intended `CI=true` timeout multiplier passes repeatedly. This is tracked as
  test-harness timing, not evidence of a remaining interpreter race.
- A fresh review also reproduced left-to-right call-argument snapshot failure
  (`inc(), n, inc(), n` observed later values). A separate focused lane is
  adding a regression and root fix.
- Assignment/scope lane, pass@2: ordinary multi-assignment now snapshots every
  RHS in order and freezes map receivers/keys; the same sequencing applies to
  multi-value `var` declarations and `for` post clauses. Control-header locals
  remain lexical, top-level multi-result values persist across Evals, exact
  imported `(string, error)` ID/content loop regressions pass, and the existing
  `eval0.go` fixture passes under focused race/adjacent gates.
- A full-package attempt made against an intermediate detached-frame patch
  timed out in `_test/fun27.go`: deep-copying every `frame.clone` had broken
  lexical closure sharing, so an interpreted goroutine never called
  `WaitGroup.Done`. The checker taught that closure capture and canceled-root
  detachment require different clone semantics. They are now split; the exact
  fixture passes under `-race -count=10`.
- Mixed incremental cells, pass@1: scanner-based depth-zero segmentation now
  parses declaration and statement runs with their native grammars, then
  executes the merged global block in lexical order. Tests cover declarations
  before/after statements, imports, functions/methods, recursion, semicolon and
  newline forms, final values, no replay, and unchanged explicit-package file
  syntax. Focused and adjacent parser/REPL/entrypoint race gates pass.

## 2026-08-29

- Panic escape ownership no longer uses a mutable aggregate-wide reference
  count. Each interpreted panic now carries one idempotent lifecycle token with
  its exact owned objects, interpreted funcs/groups, and retaining frames.
  Write barriers extend nested and cyclic membership, recovery adopts only that
  token, and recover/re-panic, defer replacement, canceled callbacks, host/API
  publication, and repeated terminal observation finish it once.
- A focused failure left two function wrappers at root retention `panic` after
  recover/re-panic and replacement. The checker taught that function cleanup
  can migrate an active panic wrapper into a new root group; token membership
  must migrate to that group rather than remain attached only to the creation
  group. The mutation/replacement churn now returns both owned-object and func
  metadata to baseline.
- Detached panic values now snapshot per token and per root generation rather
  than sharing one pending clone through each owned object. A repeated-detach
  regression exposed stale generation objects/functions; commit now diffs and
  removes abandoned generation metadata before advancing the token snapshot.
- Detached-root cloning rebases `unsafe.Pointer` values into the cloned owned
  pointer or slice allocation, preserving exact interior offsets and aliases
  with typed pointers. The public pointer-struct and slice-backing regression
  passes under `-race -count=3`.
- Direct callback lineage promotion now registers the raw source, every active
  generation alias, and the promoted clone against the same next-generation
  carrier. The public raw-plus-prior-generation counter oracle changed from the
  reproduced split result `22` to the required `23`; normal `-count=5` and race
  runs pass.
- Focused panic lifecycle matrix (late mutation, nested/cyclic graphs,
  recover/re-panic, defer replacement, API/host publication, overlapping
  shared/disjoint func groups, repeated detach, canceled native/raw callbacks,
  and old-root cleanup) passes normal `-count=3` and race `-count=1`.
- A broader `TestGornesh` adjacency run exposed
  `TestGorneshInterpretedFuncMetadataConcurrentChildReleaseIsBounded` at 100
  entries versus baseline 0. This test was outside the registered baseline and
  does not exercise panic, detach, unsafe pointers, or direct lineage; it is
  recorded as an unresolved adjacent-tree finding rather than silently folded
  into this lane.
- The adjacent child-release leak is fixed by tracking exact function-group
  membership and terminal child-result ownership. The regression and broader
  metadata lifecycle gates return to the zero-entry baseline under race.
- Upstream compatibility attempt 1 found `_test/chan8.go`: cancellable boolean
  receive discarded the value selected through the slow path. Fast and slow
  select paths now publish the same `(value, ok)` pair; the exact fixture passes
  normal `-count=10`, race `-count=5`, and CLI output is `true/true`.
- The same upstream attempt found `_test/interface8.go`: generic preparation
  treated `(*Hello)(nil)` as an unresolved generic expression. Typed nil
  conversion is now recognized during the prepass; the exact fixture and
  generic-global matrix pass normal `-count=10`, race `-count=5`.
- Successful context APIs originally canceled their own retained callbacks on
  return. `EvalWithContext`, `ExecuteWithContext`, file EvalPath, and directory
  EvalPath now keep an independent successful owner live. Their public retained
  functions remain callable; normal `-count=20` and race `-count=10` pass.
- Result publication now uses a ready/decision handshake at the exact execution
  boundary. A context cancellation that wins the decision cannot publish a
  discarded result or reclassify queued callback/channel metadata as opaque.
- Live execution owners are serialized with an interpreter gate. A canceled API
  owner releases the gate before an abandoned native call exits, while unrelated
  plain Eval waits behind a live owner. Synchronous writer, Stringer, and
  conversion-hook reentry is admitted only when the current stack contains the
  paused `runCfg` execution. The first broad gate timed out when all reentry was
  serialized; the stack-scoped retry passes exact reentry/owner tests normal
  `-count=50`, race `-count=20`.
- Panic during call-argument evaluation left retained `callArgs` snapshots on
  the execution frame. Exact-frame finalization now clears them, and the added
  panic snapshot regression passes with the broader ownership suite.
- Source package initialization is split into prepared, failed, and committed
  states. Cancellation or panic never caches success; retries use a detached
  root, rerun globals/init/main as appropriate, and fence late abandoned
  generations. Direct import, source-directory/main, and repeated panic tests
  pass normal `-count=50`, race `-count=20`.
- Final source-package review found a nested case: cancellation while `outer`
  compiled `inner` could leave `rdir["outer"]` and cause a false import cycle.
  In-progress package builds now carry their owner and roll back only that
  package's unprepared scope/AST/generic products. The new nested regression
  retries while the first native initializer is still blocked, then verifies
  the abandoned stack cannot clobber the committed result. It passes normal
  `-count=50`, race `-count=20`, plus adjacent import/publication gates normal
  `-count=20`, race `-count=10`.
- Broad race qualification attempt 1 then exposed compiler mutation of a
  package symbol map while canceled callback cleanup traversed it for ownership
  reachability. Compiler completion now publishes immutable symbol snapshots
  and durable global-slot indexes; ownership, `Globals`, and `Symbols` read
  only those snapshots. A first locking retry was rejected by the existing
  source-directory test because host inspection must remain nonblocking while
  source discovery waits. The copy-on-publish retry preserves that contract.
  The exact former race passes `-race -count=100`, export/publication adjacency
  passes `-race -count=20`, and the broad Gornesh matrix passes normal
  `-count=3`, race `-count=2`.
- Current documented ownership boundaries: values explicitly returned to host
  code are shallow host-owned values; arbitrary raw `uintptr` is opaque; the
  bundled `unsafe` hook is trusted to report pointer provenance; declarations
  completed by a canceled incremental compile may remain as zero-initialized
  compiler state, while source-package runtime initialization is transactionally
  retryable.
- Static qualification used the repository-pinned golangci-lint `v2.4.0` under
  Go `1.25.7`, because that binary panics while loading Go 1.26 export data.
  The first pinned run reported 45 diff-local findings. Dead ownership helpers
  and fields were removed, constant-argument APIs were simplified, synchronous
  and context-aware gate acquisition were separated, empty test barriers gained
  observable protected work, and changed files were formatted with `gofumpt`.
  The final pinned run reports `0 issues`. The locally installed `v2.12.2`
  additionally reports newer baseline/style diagnostics and is not the CI pin.
- A fresh reviewer then found a deterministic public-API panic after
  `Compile` without `Execute`: `Globals` and `Symbols` indexed published global
  variables before the frame had materialized their slots. Host export readers
  now omit only unmaterialized variables and expose them after `Execute`. The
  public main/named-package regression passes normal `-count=100`, race
  `-count=25`; adjacent constants, types, and functions pass normal `-count=50`,
  race `-count=10`. The final reviewer reports zero material findings.
- Final Yaegi source gates pass: `CI=true go test ./...` in the required GOPATH
  layout (`interp` 285.059s), `CI=true go test -race ./interp` (340.339s),
  `go build ./...`, repository-pinned lint with zero issues, `go mod tidy` with
  no `go.mod`/`go.sum` change, and `git diff --check`. `go vet ./...` reports
  only `stdlib/unsafe/unsafe.go:68:18: possible misuse of unsafe.Pointer`, the
  expected trusted unsafe-adapter warning.
- Downstream Gornesh was kept read-only and pointed at this checkout through
  `/tmp/gornesh-yaegi-final.mod`. All 81 adapter tests except the obsolete
  `TestEvalRecoversCompilerPanic` assertion pass normal `-count=3`, race
  `-count=2`; that assertion now fails because the formerly panicking generic
  global correctly succeeds. The exact public runtime/session matrix for
  persistent state, file-loop `content/readErr`, subagent map-loop
  `id/spawnErr`, later no-replay health Evals, callbacks, cancellation, and
  complex map-reduce passes normal `-count=10`, race `-count=5`. The dirty
  Gornesh package's broader run also has two unrelated user-work failures in
  `session_cover_test.go`; no Gornesh files were changed.

## Delivery

- Implementation commit `7f1ba82427c6c71c04c866f105f3f90cfc4acb7c`
  was pushed to `origin/jmca/gornesh-yaegi-flows`.
- Draft PR #1 (`https://github.com/jmcarbo/yaegi/pull/1`) targets `master` and
  carries only the required `includes-ai-code` label.
- This documentation-only update records the completed delivery state; the
  final local, remote, and PR head equality is audited after it is pushed.
