package vm

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

// TestPrintNativeIface checks the (type,data) words that println prints for
// an interface held as a native reflect.Value. testdata/printiface.go cannot
// reach this case (the VM keeps the interfaces that come from native code
// as *Iface), so it is tested here: the type word is the dynamic type, not
// the static interface type, and the data word is 0 only for a nil
// pointer-shaped value.
func TestPrintNativeIface(t *testing.T) {
	rtype := func(x any) uintptr { return (*[2]uintptr)(unsafe.Pointer(&x))[0] }
	iface := func(x any) reflect.Value { return reflect.ValueOf(&x).Elem() } // Kind Interface
	n := 5
	for _, c := range []struct {
		name     string
		v        reflect.Value
		typ      uintptr
		dataZero bool
	}{
		{"int", iface(7), rtype(7), false},
		{"string", iface("s"), rtype(""), false},
		{"struct", iface(struct{ a, b int }{}), rtype(struct{ a, b int }{}), false},
		{"pointer", iface(&n), rtype(&n), false},
		{"typed nil pointer", iface((*int)(nil)), rtype((*int)(nil)), true},
		{"typed nil map", iface(map[int]int(nil)), rtype(map[int]int(nil)), true},
		{"one nil pointer field", iface(struct{ p *int }{}), rtype(struct{ p *int }{}), true},
		{"int, not an interface", reflect.ValueOf(7), rtype(7), false},
		{"nil interface", iface(nil), 0, true},
	} {
		got := formatPrintIface(c.v)
		if want := "(" + printHex(c.typ) + ","; !strings.HasPrefix(got, want) {
			t.Errorf("%s: got %s, want the type word %s", c.name, got, printHex(c.typ))
		}
		if zero := strings.HasSuffix(got, ",0x0)"); zero != c.dataZero {
			t.Errorf("%s: got %s: data word 0 is %v, want %v", c.name, got, zero, c.dataZero)
		}
	}
	if refAddr(struct{ a, b int }{}) == 0 {
		t.Errorf("refAddr of a non-nil value of an unknown shape is 0, like nil")
	}
}
