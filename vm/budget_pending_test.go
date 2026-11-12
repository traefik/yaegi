package vm

import "testing"

// TestPendingTable checks the table of the accounts of top (budget.go):
// more goroutines than the slots of one bucket (the rest goes to pspill),
// a call from the host inside another one on the same goroutine, and an
// empty table at the end. The keys are not real goroutines: only the
// goroutine of the key uses its entry, here the test goroutine.
func TestPendingTable(t *testing.T) {
	// 3*gWays keys of one bucket, far from the addresses of real goroutines:
	// in the last 16 MB of the address space (kernel addresses on 32-bit
	// and 64-bit systems; a constant that fits in a 32-bit uintptr).
	const base = ^uintptr(0) &^ (1<<24 - 1)
	var keys []uintptr
	want := gbucket(base)
	for g := base; len(keys) < 3*gWays; g += 64 {
		if gbucket(g) == want {
			keys = append(keys, g)
		}
	}
	accts := map[uintptr]*account{}
	for _, g := range keys {
		if peekPending(g) != nil {
			t.Fatalf("key %#x: account before setPending", g)
		}
		accts[g] = &account{}
		if prev, had := setPending(g, accts[g]); prev != nil || had {
			t.Fatalf("key %#x: setPending replaced %v %v", g, prev, had)
		}
	}
	if n := pspill.n.Load(); n != 2*gWays {
		t.Errorf("pspill has %d entries, want %d", n, 2*gWays)
	}
	for _, g := range keys {
		if got := peekPending(g); got != accts[g] {
			t.Fatalf("key %#x: peekPending = %p, want %p", g, got, accts[g])
		}
		// A call from the host inside the call of g.
		inner := &account{}
		prev, had := setPending(g, inner)
		if prev != accts[g] || !had || peekPending(g) != inner {
			t.Fatalf("key %#x: nested setPending: prev %p had %v", g, prev, had)
		}
		restorePending(g, prev, had)
		if peekPending(g) != accts[g] {
			t.Fatalf("key %#x: restorePending did not put back the outer account", g)
		}
	}
	for _, g := range keys {
		restorePending(g, nil, false)
		if peekPending(g) != nil {
			t.Fatalf("key %#x: account after restorePending", g)
		}
	}
	if n := pspill.n.Load(); n != 0 || len(pspill.m) != 0 {
		t.Errorf("pspill not empty: n %d, %d entries", n, len(pspill.m))
	}
	for i := want; i < want+gWays; i++ {
		if pkeys[i].Load() != 0 || pvals[i] != nil {
			t.Errorf("slot %d not free", i)
		}
	}
}
