package vm

import (
	"reflect"
	"testing"
)

// TestFromNativeCyclicSlice converts back a native value of type T []T that
// contains itself (its native type is []any, see cutNamed). fromNative had
// no cycle check: it never ended and killed the test binary with a fatal Go
// stack overflow. The VM slice now has the same cycle.
func TestFromNativeCyclicSlice(t *testing.T) {
	m, spkg := buildSrc(t, "package p\n\ntype T []T\n")
	typ := spkg.Pkg.Scope().Lookup("T").Type()
	if rt := m.reflectType(typ); rt != reflect.TypeFor[[]any]() {
		t.Fatalf("native type of T: %v, want []interface {}", rt)
	}
	s := []any{nil, []any{}}
	s[0] = s
	v := m.fromNative(reflect.ValueOf(s), typ)
	out := v.R.([]Value)
	if len(out) != 2 {
		t.Fatalf("len %d, want 2", len(out))
	}
	inner := out[0].R.([]Value)
	if &inner[0] != &out[0] {
		t.Errorf("t[0] is not t: the cycle is lost")
	}
	if n := len(out[1].R.([]Value)); n != 0 {
		t.Errorf("len(t[1]) = %d, want 0", n)
	}
}
