# Interpreter lifecycle learnings

- A context-aware evaluation may return while interpreted code is still blocked
  in a native call. Later evaluations must detach from the canceled execution
  root and serialize compiler/frame preparation without reviving that root.
- Source-package compilation and runtime initialization have different commit
  boundaries. Cache a package only after initialization commits; retain prepared
  compiler output so cancellation or panic can retry initialization on a fresh
  root, and roll back an importing package that never reached preparation.
- Ownership cleanup must not traverse compiler-owned symbol maps while another
  incremental compilation mutates them. Publish immutable host-facing symbol
  and global-slot snapshots at successful compiler boundaries.
- Per-write ownership checks must stay O(1): gate registry walks behind exact
  counters (host-shared objects) and empty registries (channels, panic tokens),
  and compute reachability with one traversal per ownership source instead of
  one containment walk per owned object. Otherwise long-lived interpreters that
  accumulate allocations regress quadratically.
- Propagating host-shared storage with an all-pairs fixpoint re-scans the whole
  registry per pass; a worklist (each object transitions once) has the same
  result at a fraction of the cost.
- A retry after a failed source-package init rewinds the type allocator and can
  reallocate frame indexes with different types. resizeFrameTo must re-align
  drifted cells to the allocator before execution, or stale typed temporaries
  from the canceled attempt panic on first write.
- Interpreted-to-native calls must not allocate a fresh MakeFunc wrapper per
  call; memoize the bound wrapper per (value, root, cancel, signature) on the
  metadata group or alias registration grows the registry per call.
- A canceled evaluation's worker unwinds its deferred calls after the API call
  has returned, so the execution gate no longer excludes it from later
  evaluations. While such a zombie unwinds, its interpreted steps hold the
  funcSweep fence exclusively (tracked by a counter, so nested frames and
  native stretches behave correctly): interpreted deferred writes are fenced
  against a later evaluation's interpreted steps on shared containers. Native
  deferred calls run outside the fence — a host function value deferred by
  interpreted code writes through host code the interpreter cannot fence — and
  a zombie defer blocked in a host call still releases the fence and never
  blocks later evaluations.
- Channel receives store an unaddressable copy. Any interpreter path that
  writes a received value directly into a frame cell must store an addressable
  copy instead, or a later write into that cell (for example the regular-
  return Set for `return <-ch`) panics with "reflect: reflect.Value.Set using
  unaddressable value".
- A step that acquires the funcSweep fence must release it in the mode that was
  acquired, not the mode a re-read of zombieDefers would now derive: the
  counter is flipped by the canceled worker's goroutine and can change mid-step
  between acquisition and release. The acquired mode is captured on the frame
  (with save/restore so a reentrant Eval nesting on the same global frame keeps
  the outer mode); the release helpers consult the capture. Re-deriving the
  mode at release fatal-errors in sync ("RUnlock/Unlock of unlocked RWMutex")
  and kills the embedding process.
- Known growth boundary: owned allocations are registered per make/new/composite
  literal and released at frame exit or Eval end, so a single frame that runs
  forever (a loop allocating in an interpreted daemon) grows the registry
  without bound (measured: an interpreted loop of map literals reached ~60 GB
  RSS in minutes, while master stays flat). Bounding this safely requires
  mid-execution reachability over all active frame chains, not just the root.
- Known growth boundary: interpreted function values that cross the Eval API
  keep their metadata marked opaque, which is never deleted, so every returned
  or host-retained closure pins its captures and compiled graph permanently
  (measured: ~255 KB per Eval across 20k sequential Evals). PurgeRetainedFuncs
  is the manual reclamation path. The long-term fix — a weak funcval-keyed
  registry that reclaims metadata when the host drops the wrapper — was
  probed and is mechanically feasible but is a full keying rework, not an
  additive layer: as long as funcMeta holds the reflect.Value key, the map
  itself retains the wrapper and no weak mechanism can observe the drop.
  Verified probe results for the future rework: (1) a func value's identity
  is its funcval address, readable as the single word of a func variable and
  identical to reflect.Value's internal comparison identity, so map[uintptr]
  keying preserves today's key semantics; (2) runtime.SetFinalizer accepts
  the funcval address behind a typed pointer and fires exactly when the
  wrapper becomes unreachable — reflect.Value.Pointer() (a code pointer) and
  raw unsafe.Pointer are rejected; (3) address reuse after a drop needs a
  generation guard (finalizer deletes only if the entry's generation still
  matches); (4) every consumer that uses a map key as a reflect.Value
  (types, invocation, endpoint matching in the purge, frame funcMeta
  slices) must switch to the pointer domain, storing reflect.Type in the
  entry for the convertible-type fallback. None of this is additive on top
  of the existing value-keyed map, so it lands only as a dedicated change
  with the isolation and retention suites re-derived.
- IMPLEMENTED 2026-08-30: the weak funcval-keyed metadata registry described
  above landed as a full keying rework. Keys are the funcval address, read
  as the single word of a func variable (identical to the reflect.Value
  comparison identity of the old canonical keys); every consumer moved to
  the pointer domain — lookup, func-value walks and collectors, the purge,
  the root sweeps, frame key lists, directFuncs source keys, the cloner's
  snapshot maps, and the bound-wrapper cache keys. One entry exists per
  funcval, storing its registered reflect.Type, which collapsed the
  fallback scans into a direct hit plus a single type gate (equal or
  convertible), preserving the value-keyed semantics as a safety net.
  Each insertion arms a runtime.SetFinalizer that deletes the entry when
  the wrapper becomes unreachable; a per-entry generation counter makes a
  stale finalizer harmless when an address is reused, and arming clears any
  leftover finalizer first because sweeps and purges may delete an entry
  while its wrapper is still alive (the binder re-registers such aliases).
  Implementation hazards found on the way, all load-bearing: (1) the
  finalizer closure must never capture the funcvalRef struct (or any
  pointer derived from the wrapper) — capturing it captures the typed
  funcval pointer, and the finalizer itself then keeps the wrapper alive
  forever; capturing the interpreter, key, and generation is required and
  safe; (2) the metadata payload pinned its own
  wrapper through two edges — the invoker keeps the creating frame alive
  and the frame's literal slot kept the wrapper, now reverted by restoring
  literal slots when a frame's last runCfg activation exits (root-frame
  slots are durable REPL storage and are never restored — restoring them
  nils published callbacks), and the lexical clone's funcCarrier, now
  stored as a key rather than a wrapper value; (3) arming a finalizer on a
  funcval that was already freed fatal-errors ("pointer not in allocated
  block"), so registration paths must hold the wrapper live until the
  finalizer is armed, which they do on the calling stack. Retention that
  remains is interpreter-side and master-parity: root result cells (one per
  result-bearing call Eval — the ordinary expression-slot mechanism) and
  package-level variables pin their wrappers, so PurgeRetainedFuncs remains
  the manual reclamation path; the weak registry removes the registry
  itself, its alias keys, and the bound-wrapper cache as independent
  retainers. Isolation suites re-run green under -race. Review follow-ups
  (2026-08-31): the exit-time slot restore is the only frame-cell writer in
  the activation-exit path, so it holds the funcSweep fence in read mode —
  the incremental sweep's capture-cell reads are unfenced and trust the
  fence for exclusivity against this write — and it is skipped when another
  active activation reaches the frame through ancestry or a cloneOf link
  (shared copied cell headers keep master-like semantics for
  goroutine/recursive-closure cases; such entries stay sweep/purge
  reclaimable). A second unfenced frame-cell writer predates this change
  and remains master-parity: the closure invoker's own slot restore in
  buildClosureWrapper runs on host goroutines and inside fence-released
  native calls, so its exclusivity against the sweep's unfenced reads is
  not fence-based. directFuncs activations store their funcval key eagerly
  so eviction paths never derive keys (an allocation) under funcMu. The
  finalizer closure must never capture the funcvalRef (or any pointer
  derived from the wrapper) — capturing the interpreter, key, and
  generation is required and safe. The funcval address-reuse guard is
  belt-and-braces: the runtime never reuses a funcval address before its
  finalizer has executed, so a stale finalizer and a fresh registration
  cannot coexist at one key.
- Reentrancy for the execution gate is an explicit token scoped to the
  goroutine running the execution, not a native stack probe: any goroutine
  running interpreted code has runCfg on its stack, including goroutines
  spawned by an interpreted `go` statement, so a stack probe treats an Eval
  from such a goroutine as reentrant and lets it run concurrently with the
  gated execution (observed as a data race between prepareExecutionFrame and
  the running execution). The bypass is decided live at gate-acquisition
  time: the gate-holding goroutine may always re-enter (including an inline
  source-package init retry whose ambient owner already fired), and any
  other token holder may re-enter only while its execution's owner channel
  is still open — read at decision time, never sampled, because a
  cancellation landing while a canceled worker is parked inside a deferred
  host call must strip the bypass for the rest of the drain (a sampled flag
  is a TOCTOU hole, reproduced end-to-end). The token survives arbitrarily
  deep native callback stacks, nests per interpreter, is never inherited by
  `go`-statement goroutines, and its per-goroutine entry is reclaimed when
  the stack empties. The drain joins the zombie fence accounting at every
  deferred-call boundary, so a cancellation firing mid-drain still fences
  the remaining interpreted deferred steps (at deferred-call granularity: a
  body already in flight finishes its steps on the shared fence).
- Compilation entry points that can import source packages (Compile,
  CompileAST, CompilePath, EvalTest) execute package initialization during
  compilation and must take the execution gate like Eval, or interpreted init
  code overlaps an in-flight evaluation.
- Removing an owned object from the registry is O(1) via its key. Never scan
  the registry to find an object again at deletion time: a per-target scan
  made frame exit quadratic in the allocations of the exiting frame (a
  100k-allocation frame spent ~15s unwinding).
- gc resolves variable-rooted memory operands (selector, index, pointer deref)
  of multi-assignment map destinations at assignment time, like plain
  identifiers. Yaegi still freezes non-ident memory operands before the
  right-hand side, so a right-hand-side call that rebinds the receiver writes
  into the orphaned map; the same late-resolution question applies to slice
  and pointer-deref destinations. A multi-return call RHS into a map-entry
  destination (m[0], x = f()) also still loses the map write.
- Bounding the ownership registries cannot skip registration: allocations made
  before the first cancellation are exactly the ones the first detached-root
  clone must copy, and unregistered native-looking objects are shared with the
  canceled worker (16 detached-root isolation and host-shared tests fail under
  a registration gate). The bound must come from eviction against a root set
  that is a strict superset of every live consumer: durable globals, every
  registered active frame's full ancestor chain, all closure captures, and
  direct funcs. The incremental sweep consumes its trigger at the top of an
  exec step (no locks held) under a TryLock'ed exclusive fence, and stays
  pending while any frame is unwinding defers, because deferred-call values
  are invisible to the root set.
- Purging retained function metadata must be group-scoped: host-bound aliases
  share the metadata group, and lookup's convertible-type fallback would
  resurrect a purged capability through a surviving alias key. Eligibility
  (pending refcounts, panic tokens, live channel sends) must be re-checked
  under the delete-phase lock: panic-token memberships attach under funcMu
  without bumping the group version.
- A func-bearing-value walk can be inverted from visitor-per-registry-entry
  into collect-once-then-intersect (exact canonical set + an any-ambiguous
  flag) with identical results, because pointer/slice/map containers can only
  ever yield ambiguous matches; this removed the per-Eval quadratic term in
  accumulated opaque metadata.
- The directFuncs activation cache must be swept like the ownership
  registries: each entry pins its root frame's globals and its cloned
  wrapper, so a long-lived interpreter otherwise grows one pinned root per
  detached generation plus one dead line per swept-away source. The
  incremental ownedGC sweep evicts entries whose root is no longer live
  (durable root, active frame chain, or retained metadata) or whose source
  and value endpoints both lost their metadata, mirroring the metadata
  purge's endpoint rule. In practice the root rule is a narrow safety net —
  every detach's commit already deletes the abandoned root's entries — so
  the endpoint rule is the workhorse reclaim for dead-source lines.
  Eviction is transparent: the cache is consulted
  only after a successful metadata lookup keyed by the live activation
  root, so a dropped line costs at most one extra clone.
- KNOWN OPEN ITEM RESOLVED 2026-08-30 (was: decision recorded 2026-08-30):
  an incremental Eval beginning with `func` permanently added 1-3 global-frame
  slots on this branch (master adds none) because the anti-replay fix returns
  the wrapper body, which compiles in global scope where each func literal
  allocates a persistent slot (cfg.go funcLit pushes n.findex = sc.add(n.typ)
  in the enclosing scope). Wrapping the body in a one-shot function literal
  does not help: the literal's own value slot is allocated in the enclosing
  scope before its function scope is pushed. The chosen direction was
  codegen for immediately-called function literals at root level, and that
  is what landed: a callExpr whose function is a funcLit, compiled at global
  frame level (sc.global, which pushBloc propagates through root-level
  control bodies) and not under defer, now compiles to a direct activation —
  the literal allocates no value slot (n.findex = notInFrame, n.val = n,
  gen = nop) and the call takes the existing declared-function path of
  run.go call (a *node fun value with an invalid rval). The funcLit's own
  slot, the vestigial call-site sc.add, and the call-func snapshot are all
  skipped; defer-wrapped literals keep the wrapper path. Void IIFEs now add
  zero slots; result IIFEs add only the ordinary call-expression result
  slot, exactly like any other root-level call (len("ab") also adds one per
  Eval on master — that residual is the pre-existing expression-slot
  mechanism, whose cells also pin their results and are purge-reclaimable).
  Transient slot recycling remains rejected (root-frame slots are reachable
  by goroutine-safe code paths), and the scheduler-level one-shot
  declaration remains rejected.
- Binary calls whose parameter is exactly `interface{}` (non-variadic) pass
  the concrete underlying value, with valueInterface boxes and generated
  interface wrapper structs (`IValue` + `W*` func fields) unwrapped at the
  call boundary. Scalars only: slices/maps/arrays are passed untouched — a
  copy would silently break in-place mutation contracts (sort.Slice sorts a
  copy; observed as silent no-op sorting) and change the container's dynamic
  type seen by host type assertions. This is what lets encoding/json reflect
  over interpreted values held in non-empty interfaces (#633, scalar shape).
  Method dispatch is unchanged where it matters: non-empty interface
  parameters keep wrappers, variadic `...any` (fmt family) still re-wraps at
  run time via getBinValue/mapTypes, and a valueInterface whose node matches
  the caller's mapTypes (json.Marshaler for json.Marshal) is wrapped instead
  of unwrapped. Wrapper-shape recognition (`IValue` + only func fields)
  additionally requires an anonymous run-time type or an extract-style name
  starting with `_`, so a user struct that happens to have that field shape
  is never unwrapped. Accepted trade-offs: a value that crossed a binary
  `any` boundary is stored concrete, so a later interpreted type assertion
  to a NON-empty interface on it fails for wrapper-less interpreted dynamic
  types (concrete assertions now succeed, as natively); containers of
  interface-held interpreted values do not marshal (loud
  "unsupported type: func()" error, unchanged from before the concrete-
  argument change); per-element Marshaler dispatch for slices of
  marshaler-bearing structs (#1486) is untouched. Note (verified by review):
  json.Marshal of a value held in a json.Marshaler variable DOES dispatch to
  MarshalJSON — the mapTypes match works there.
