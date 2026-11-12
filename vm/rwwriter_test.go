package vm

import (
	"math/bits"
	"reflect"
	"testing"
)

// TestRWFeatBits checks that each feature bit of rwwriter.go names the
// method at the same index in rwFeatMethods: rwFeatures sets bit i for
// rwFeatMethods[i], and the features of rwcombos_gen.go follow the same
// order. The names are only read here: they document the bits.
func TestRWFeatBits(t *testing.T) {
	want := []struct {
		bit  int
		name string
	}{
		{rwFlush, "Flush"},
		{rwHijack, "Hijack"},
		{rwReadFrom, "ReadFrom"},
		{rwPush, "Push"},
		{rwCloseNotify, "CloseNotify"},
		{rwUnwrap, "Unwrap"},
		{rwFlushError, "FlushError"},
		{rwSetReadDeadline, "SetReadDeadline"},
		{rwSetWriteDeadline, "SetWriteDeadline"},
		{rwEnableFullDuplex, "EnableFullDuplex"},
	}
	if len(want) != len(rwFeatMethods) {
		t.Fatalf("%d feature bits, %d methods in rwFeatMethods", len(want), len(rwFeatMethods))
	}
	for _, w := range want {
		i := bits.TrailingZeros(uint(w.bit))
		if got := rwFeatMethods[i].Name; got != w.name {
			t.Errorf("bit %d: method %s, want %s", i, got, w.name)
		}
	}
	if rwNumFeat != len(rwFeatMethods) {
		t.Errorf("rwNumFeat is %d, want %d", rwNumFeat, len(rwFeatMethods))
	}
}

// TestRWCombos checks the wrapper type of every feature mask: it has the
// three methods of http.ResponseWriter and, outside the exception of
// rwComboMask, exactly the optional methods of the mask. A control method
// that the type does not have must not be in the wrapper: native code that
// asserts w.(interface{ SetWriteDeadline(time.Time) error }) fails, as in Go.
func TestRWCombos(t *testing.T) {
	for mask := uint16(1); mask < 1<<rwNumFeat; mask++ {
		want := rwComboMask(mask)
		mk := rwCombos[want]
		if mk == nil {
			t.Fatalf("mask %#x: no wrapper", mask)
		}
		wt := reflect.TypeOf(mk(rwBase{f: rwFeat{mask: mask}}))
		for i, meth := range rwFeatMethods {
			_, ok := wt.MethodByName(meth.Name)
			if has := want&(1<<i) != 0; ok != has {
				t.Errorf("mask %#x: %v has %s: %v, want %v", mask, wt, meth.Name, ok, has)
			}
		}
		if mask&rwRare == 0 && want != mask {
			t.Errorf("mask %#x: wrapper of mask %#x", mask, want)
		}
		if n := wt.NumMethod(); n != 3+bits.OnesCount16(want) {
			t.Errorf("mask %#x: %v has %d exported methods", mask, wt, n)
		}
	}
}
