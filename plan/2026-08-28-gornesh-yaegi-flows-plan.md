# Gornesh Yaegi flow hardening — plan

Date: 2026-08-28

## Goal

Fix the interpreter defects exercised by Gornesh directly in Yaegi, with
upstream-quality regressions and without moving Gornesh-specific lifecycle,
provider, or presentation policy into the interpreter.

## Confirmed candidate flows

The inventory remains open until the current Gornesh worktree, its plans and
tests, and independent reviews have all been reconciled. Initial confirmed
Yaegi behaviors are:

1. Persistent incremental `main` functions are replayed by later `Eval` calls.
2. A top-level immediately invoked function literal is replayed by later
   `Eval` calls.
3. Multi-assignment with mixed call and non-call right-hand sides can silently
   produce incorrect values.
4. Multi-result declarations inside top-level control bodies can lose their
   block-local bindings.
5. Inferred generic global initialization can panic the compiler with a nil
   reflect type instead of returning a normal result or diagnostic.
6. Top-level type switches can panic in statement mode.
7. Incremental parsing rejects mixed declarations and statements in one input.
8. Re-importing a package in a persistent interpreter reports a redeclaration.
9. Range-over-function reports a nil-type failure rather than behaving like
   supported Go or producing a clear unsupported-feature diagnostic.
10. `EvalWithContext` can return before its worker exits; a later evaluation
    can revive the canceled worker and race or leak output.
11. Top-level control-header locals can escape their lexical scope and replace
    persistent globals.
12. A preloaded binary-package symbol used without an import can panic through
    a nil internal type rather than return an undefined-name diagnostic.

Mixed declaration/statement input and range-over-function require explicit
compatibility decisions after reproduction; they are not assumed to have the
same root cause or safe patch as the other items.

## Process

1. Reduce every candidate to a pure-Yaegi executable regression and record the
   baseline result.
2. Classify ownership: Yaegi defect, intentional API/REPL contract, unsupported
   language feature, or Gornesh-only concern.
3. Fix confirmed Yaegi defects at their root with the smallest coherent change.
4. Run focused tests after every fix and add a regression for every discovered
   flaw.
5. Run the repository's complete format, vet, race, and test gates.
6. Obtain independent interpreter, compatibility, and adversarial reviews;
   repeat until no material findings remain.
7. Commit, push, open a draft PR, label it `includes-ai-code`, and audit the
   exact final head and delivery state.

## Completion evidence

- Each in-scope Gornesh flow has a pure-Yaegi regression and an ownership
  decision in the ledger.
- All confirmed interpreter defects pass their focused regressions.
- Full Yaegi gates pass at the final commit.
- Fresh reviewers find no material correctness, compatibility, race, or API
  flaw in the final diff.
- Draft PR state, exact head, and label are verified from GitHub.

## Final implementation status

- All twelve confirmed Gornesh-facing flows have direct Yaegi regressions and
  pass the final full normal/race gates.
- Adversarial follow-up findings for retained ownership, execution
  serialization, nested source-package retry, compiler/export concurrency, and
  compile-only symbol publication are fixed with regressions.
- Independent final review reports zero material findings.
- Implementation commit `7f1ba82427c6c71c04c866f105f3f90cfc4acb7c`
  is pushed on `jmca/gornesh-yaegi-flows`.
- Draft PR #1 is open against `master` and carries only the required
  `includes-ai-code` label. The exact final-head audit follows this
  documentation-only delivery update.
