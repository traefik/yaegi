# Round 3 — growth remediation design (2026-08-30)

Design round for the two unbounded-growth criticals (F1/F1c owned registries,
F2 funcMeta opaque retention) plus a complete growth inventory. Design work by
three independent expert passes; rulings are evidence-backed. Implementation
follows the specs below.

## Ruling 1 — the sticky registration gate is dead

Skipping `registerOwnedValue`/`registerOwnedChannel` until the first
cancel/detach (bounding never-canceled interpreters) was tested empirically
against a scratch clone: 16 of the PR's own tests fail, including
`TestGorneshDetachedRootIsolatesCanceledDeferredAggregateWrites` (zombie
deferred writes reach the new root: "old deferred write reached new graph")
and the host-shared publication tests. Pre-cancel allocations are exactly the
ones the first detach must clone; unregistered ⇒ `cloneMap/clonePointer/
cloneSlice` return the original (owned.go ~2958-3013) ⇒ shared with the
zombie, and `hostSharedEstimate` stays 0 so host-identity preservation breaks.
No arming point can fix this. Registration must remain universal; bounding
must come from eviction.

## Ruling 2 — the pruning collector's invariant

`collectReachableObjectsLocked` (owned.go ~2455-2459) does NOT descend into
unregistered Map/Ptr/Slice containers (`objects := ownedObjectsForValueLocked;
if len == 0 return`). This is safe only under the inductive invariant
"unregistered ⇒ unreachable from every cell a live or future execution can
touch", preserved because sweeps evict containers together with everything
reachable only through them, and because new sweeps must use a root set that
is a SUPERSET of the existing ones (Eval-end sweep + frame-exit release).

## F1/F1c remediation — bounded incremental ownership sweep ("ownedGC")

IMPLEMENTED 2026-08-30 (interp/owned.go armOwnedGCLocked/maybeRunOwnedGCSweep/
ownedGCSweepLocked, interp/run.go execWithFuncSweepFence + runCfg registration,
interp/program.go prepareExecutionFrame secondary trigger; tests in
interp/gornesh_owned_gc_internal_test.go). Deviations from the ruling as
written: (1) the eviction predicate follows the actual sweepRootOwnedObjectsLocked
body, which does NOT exempt hostShared objects (the `!hostShared` term in the
paraphrase below does not exist in the code); (2) capture cells are read
directly under funcMu with the exclusive fence instead of via
snapshotFuncMetaCapture, which acquires funcMu.RLock (self-deadlock under the
sweep's funcMu.Lock) and returns a copy that would defeat the interval
collector's interior-pointer keep-alive; (3) the channel "owner not active"
predicate keeps only non-root owners with a live activeFrames entry — root-owned
channels follow the existing sweepRootOwnedChannels semantics (evictable when
unreachable), which is required for boundedness under top-level churn — and
channels referenced by non-terminal send values are pinned explicitly, since a
channel buffered inside another channel is not reflect-traversable; (4) a new
frameDrains counter (incremented under the exclusive fence in the runCfg exit
path) gates the whole sweep: a draining frame's remaining deferred call values
are invisible to the root set, so the sweep stays pending while any drain is
active. All Gornesh tests, including the 63 detached-root isolation tests,
pass under -race; the full interp suite matches its pre-existing GOPATH-layout
failure baseline.

Universal registration preserved; run the same eviction as
`sweepRootOwnedObjects` incrementally against a complete active-frame root set.

State (interp): `activeFrames map[*frame]int` (refcount, funcMu-guarded, root
reentrancy-safe), `ownedGCPending/ownedGCInFlight atomic.Bool`,
`ownedRegistrations int` (funcMu). Constants: cap 1<<16 entries, amortize
1<<12 registrations.

Arming (inside existing funcMu sections only, never taking the fence):
increment `ownedRegistrations` at every registry insert
(registerOwnedValue ~196, registerOwnedChannel ~218, registerNativeResultValuesFromExec
~989, cloner commits ~3303/~3438, adoptOwnedObjectLocked ~1858,
publishOwnedChannelLocked ~438); when registry size ≥ cap and registrations ≥
amortize, set `ownedGCPending`.

Trigger — the load-bearing locking decision: consume at the TOP of
`execWithFuncSweepFence` (run.go ~506) before any acquisition, where the
goroutine holds no locks: if pending && inFlight.CAS, `funcSweepMu.TryLock()`,
sweep, unlock; always reset inFlight (panic-safe defer); TryLock loss ⇒ retry
next step. Secondary trigger: inside `prepareExecutionFrame` after
`interp.mutex.Unlock()` while the fence write is still held. Never upgrade the
fence while holding funcMu (the fence is not reentrant; an inline upgrade from
registerOwnedValue would fatal against write-held callers like
publishHostValueLocked).

Root set (all under funcMu; frame cells via snapshotFrameValues under
f.mutex.RLock after funcMu): (1) durable globals =
snapshotOwnedReachabilityValues(interp.frame); (2) EVERY frame in activeFrames
plus its full anc chain up to its root, ALL cells, regardless of funcState —
this covers parked-in-native and zombie-draining frames (zombie frames stay
registered until after the run.go release sequence); (3) every funcMeta group
capture cell; (4) directFuncs values. Eviction: exactly the
sweepRootOwnedObjects predicate (`!hostShared && channelRefs == 0 && no panic
token && !reachable && !(Ptr && visited.contains)`), via
unregisterOwnedObjectLocked; channels per sweepRootOwnedChannels semantics
(retire terminal sends; owner-not-active predicate). Collector extension: add
`reflect.Chan` to collectReachableObjectsLocked recording channel registry
hits (additive; existing callers pass nil).

Frame registry maintenance: `activeFrames[f]++` in the existing
`funcState = funcFrameActive` window (run.go ~381-383); decrement/delete in a
fresh funcMu window immediately after the release sequence (run.go ~447),
before the repanic. runCfg is the ONLY activation site (5 call sites:
runOnFrame ~293, declared-wrapper invoke ~1633, go-stmt ~2044, interpreted
call ~2047, closure-wrapper invoke ~2686; zombie drain re-enters via the
wrappers).

Semantics: strictly more conservative than the Eval-end sweep (superset root
set) ⇒ never evicts anything the current sweep would keep ⇒ cancel-isolation
preserved with no relaxation. Residuals to document: forever-blocked
goroutines pin one activeFrames entry each (Go-like); workloads whose live set
exceeds the cap pay one O(live+registry) sweep per amortization window.

Test plan: registry bounded under a ~1M-allocation root loop (sample from a
watcher goroutine); same for channel churn; isolation preserved across a
cancel occurring after a long pre-cancel allocation loop; live captures kept
while sweeps run; sweep under a zombie-draining fence (no deadlock).

## F2 remediation — PurgeRetainedFuncs + per-Eval walk inversion

Opaque funcMeta entries are created by four writers (Eval returns, contended
fence preserve-all, channel publish, panic publish) and deleted by none; only
panic-published groups demote back (adoptOwnedPanicGroupLocked ~1836). The
permanent population is Eval-returned funcs and contended-fence preserve-alls.

API: `func (interp *Interpreter) PurgeRetainedFuncs() int` — number of
metadata entries removed; idempotent. Experimental doc comment must state the
contract: values reachable from package-level variables are never purged and
keep re-binding; a purged value remains callable (self-contained MakeFunc
wrapper) but permanently executes against its original root — after a later
cancel/detach its writes land in the abandoned root and lose channel-ownership
tracking. WARNING: do not call from interpreted code, from a host callback of
a running evaluation, or while paused under the debugger. Called from an
unrelated goroutine it blocks until evaluations quiesce.

Mechanics (group-scoped — REQUIRED because callOwnerBinder aliases share
`meta.group` and `lookupInterpretedFunc`'s convertible-type fallback would
resurrect a key-scoped purge):
1. Snapshot under funcMu.RLock: candidates (retention == opaque, any root
   generation), group members (all keys sharing each group), group captures +
   versions; eligibility per group: `pending == 0 && len(panicTokens) == 0 &&
   no non-terminal ownedChannelSend references it`.
2. Anchors (fence held): current root global cells (directFuncs values are deliberately NOT anchors: cloned wrappers get fresh groups and the clone cache holds self-referential entries, which would keep purge non-idempotent; directFuncs entries are instead cleaned by either endpoint matching a deleted key).
   Collect exact canonical func values once; any ambiguous func-capable
   container marks all candidates live. Capture fixpoint via
   snapshotFuncMetaCapture (mirrors sweepRootInterpretedFuncs).
3. Delete under funcMu.Lock: candidate keys of live groups skipped; deleted
   groups delete ALL member keys (aliases); delete directFuncs entries whose
   source key was deleted; rebuild each affected frame's funcMeta slice
   (template: deleteUnreachableRootFuncMeta ~836-841).
4. Then re-run the owned-object sweep via a factored
   `sweepRootOwnedObjectsLocked(root)` (split the fence-acquiring wrapper from
   the body) to free captures pinned only by deleted frames; the exact
   hostSharedEstimate is maintained by unregisterOwnedObjectLocked.

Locking order (verified consistent): funcSweepMu (exclusive) → interp.mutex(R)
→ funcMu (R then L) → frame.mutex (R).

Per-Eval CPU (O(n²) in accumulated entries): invert visitor-per-entry into
collect-once-then-intersect in preserveReturnedInterpretedFuncsLocked,
markInterpretedFuncMetadataEscapedLocked, rollbackInterpretedFuncPanicEscape,
and sweepRootInterpretedFuncs' seed scan (exact-set + anyAmbiguous flag;
ambiguous ⇒ all candidates live, faithful to funcValueVisitor's
ptr/slice/map ambiguity rules). No walk caps (truncation is either unsound or
re-creates the leak). Land the inversion with the purge.

No automatic eviction: its victims are by construction the host's live
callbacks — strictly worse than manual purge.

Test plan: purge frees after the host drops callbacks (count returns to
baseline; second purge returns 0); a still-retained purged callback stays
callable, misses lookupInterpretedFunc, and exhibits the documented stale-root
behavior after a cancel/detach; globals-reachable callbacks (incl. declared
funcs) are kept and still re-bind; groups pinned by undelivered channel sends
or panic tokens are skipped until retired; -race loop of purge × Eval ×
Globals × Symbols × channel sends; directFuncs source-key cleanup.

## Inventory — complete growth audit (branch vs master)

Everything else audited is bounded, transient, or pre-existing:
panic tokens (finished on every terminal path), callArgs (cleared per call),
group.bound (capped 1024 + reset), scopes/srcPkg/publishedSrcPkg/
globalVarIndexes (replaced wholesale), deferred (frame lifetime), zombie/go
goroutines (frame retention, no registry). REPL accumulation in interp.roots
(+1/Eval, AST pinned forever) is pre-existing and identical on master.
PR-introduced overheads: ~10-12% retained bytes per REPL Eval (larger per-root
AST/exec metadata), and the func-leading slot regression below.

## Func-leading REPL Eval slot growth — finding + dead ends (RESOLVED 2026-08-30)

Finding (probe-verified): an incremental Eval beginning with `func` (bare
literal or IIFE) permanently adds +1..+3 global-frame slots on this branch
(60k slots / 173 MB after 20k Evals); master adds 0. Cause: master's parse
retry returned the synthetic-main FuncDecl (function scope, no global slots)
but replayed it on every later Eval; the PR's anti-replay fix returns the
wrapper BODY (ast.go ~619), compiling the statements in the global scope where
`case funcLit: n.findex = sc.add(n.typ)` (cfg.go ~583, the else branch of the
isRootImmediateFuncLit gate) permanently extends the
global frame.

Dead ends (attempted and reverted): (a) wrapping the body in a one-shot void
FuncLit call — the outer funcLit itself still allocates a global slot
(cfg.go:574 allocates the literal's value slot in the ENCLOSING scope before
pushing the function scope), net +3/Eval unchanged; void-result IIFEs broke
(`return voidCall()` rejected); (b) forwarding the inner result via a
synthesized outer function of the inner's type — same slot count, same void
breakage. Sound fixes require one of: codegen for immediately-called funcLits
without a registry slot; transient/recycled slot storage for root-level
literal entrypoints (must be goroutine-safe under `go`); or the scheduler-
level one-shot FuncDecl design the PR explicitly rejected.

IMPLEMENTED 2026-08-30 — the chosen direction, direct-activation codegen
(interp/cfg.go isRootImmediateFuncLit + interp/gornesh_root_iife_slot_
internal_test.go): a callExpr whose function is a funcLit, compiled at
global frame level (sc.global, propagated by pushBloc through root-level
control bodies) and not under defer, compiles to a direct activation. The
literal allocates no value slot (n.findex = notInFrame, n.val = n via the
funcDecl fallthrough, gen = nop), the call skips the vestigial aGetFunc
sc.add and the call-func snapshot, and at runtime genValue returns the
*node value whose rval is invalid, so run.go call takes the existing
declared-function activation path. defer-wrapped literals keep the wrapper
path. Measured: void IIFEs add 0 slots per Eval (was +2); result IIFEs add
only the ordinary call-expression result slot (was +3), matching any other
root-level call such as `len("ab")` (+1/Eval on master and this branch —
the pre-existing expression-slot mechanism whose cells also pin their
results; PurgeRetainedFuncs remains their manual reclamation path).
Transient slot recycling remains rejected (root-frame slots are reachable
by goroutine-safe code paths), and the scheduler-level one-shot
declaration remains rejected.

## Weak funcval-keyed registry — IMPLEMENTED 2026-08-30

The F2 long-term fix (LEARNINGS "Known growth boundary: interpreted
function values that cross the Eval API") landed as the recorded full
keying rework: registry keys are funcval addresses (the word a func
variable stores — identical identity to the old canonical reflect.Value
keys), every consumer moved to the pointer domain (lookup, walks and
collectors, purge, root sweeps, frame key lists, directFuncs source keys,
cloner snapshot maps, bound-wrapper cache keys), one entry per funcval
storing its registered reflect.Type, and each insertion arms a
SetFinalizer-based eviction guarded by a per-entry generation counter.
Load-bearing implementation hazards (all recorded in LEARNINGS): the
finalizer closure must capture scalars only (capturing the funcvalRef
keeps the wrapper alive); the payload's own retention edges were broken by
restoring literal slots at last-activation exit (root slots exempt — they
are durable REPL storage) and by keying funcCarrier; arming requires the
wrapper to still be live. Result-cell and package-variable retention
remain (master parity, purge-reclaimable); the registry, its alias keys,
and the bound-wrapper cache are no longer independent retainers.
