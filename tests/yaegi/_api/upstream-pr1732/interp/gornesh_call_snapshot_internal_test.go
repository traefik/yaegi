package interp

import "testing"

func TestGorneshGlobalCallSnapshotsAreCleared(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
snapshotValueGornesh := 0
snapshotIncrementGornesh := func() int { snapshotValueGornesh++; return snapshotValueGornesh }
snapshotCaptureGornesh := func(first, second int) int { return first + second }
snapshotCaptureGornesh(snapshotIncrementGornesh(), snapshotValueGornesh)`); err != nil {
		t.Fatalf("Eval call snapshots: %v", err)
	}

	i.frame.mutex.RLock()
	retained := len(i.frame.callArgs)
	i.frame.mutex.RUnlock()
	if retained != 0 {
		t.Fatalf("global frame retained %d call snapshots after evaluation", retained)
	}
}

func TestGorneshGlobalCallSnapshotsAreClearedAfterPanic(t *testing.T) {
	i := New(Options{})
	if _, err := i.Eval(`
func consumePanickingSnapshotGornesh([]byte, int) {}
func panicSnapshotGornesh() int { panic("snapshot panic") }
`); err != nil {
		t.Fatalf("define panicking snapshot functions: %v", err)
	}
	for iteration := 0; iteration < 10; iteration++ {
		if _, err := i.Eval(`consumePanickingSnapshotGornesh(make([]byte, 1024), panicSnapshotGornesh())`); err == nil {
			t.Fatalf("panicking snapshot iteration %d returned nil error", iteration)
		}
		i.frame.mutex.RLock()
		retained := len(i.frame.callArgs)
		i.frame.mutex.RUnlock()
		if retained != 0 {
			t.Fatalf("global frame retained %d call snapshots after panic iteration %d", retained, iteration)
		}
	}
}
