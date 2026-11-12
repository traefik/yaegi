package vm

import "testing"

// TestStaticBoxes checks that small integers, bools and the empty string
// share one box per type in an interface (boxes.go),
// that other values get a new box, and that the boxes keep their type and
// value. testdata/ifaceboxes.go checks the results against go run.
func TestStaticBoxes(t *testing.T) {
	const src = `package p

type Kind int

func Small(n int) any    { return n }
func Kinds(n int) any    { return Kind(n) }
func Str(s string) any   { return s }
func Ptr(p *int) any     { return p }
func Flag(b bool) any    { return b }
`
	m, spkg := buildSrc(t, src)
	call := func(name string, v Value) *Iface {
		r := m.Call(m.funcOf(spkg.Func(name)), []Value{v}, nil)
		iv, ok := r.R.(*Iface)
		if !ok {
			t.Fatalf("%s: %T, want *Iface", name, r.R)
		}
		return iv
	}
	same := func(name string, v Value, want bool) {
		t.Helper()
		a, b := call(name, v), call(name, v)
		if (a == b) != want {
			t.Errorf("%s(%v): same box %v, want %v", name, v, a == b, want)
		}
		if a.V != v && !(v.R == nil && a.V.R == nil && a.V.N == v.N) {
			t.Errorf("%s(%v): box holds %v", name, v, a.V)
		}
	}
	same("Small", Value{N: 0}, true)
	same("Small", Value{N: 255}, true)
	same("Small", Value{N: 256}, false)
	same("Small", Value{N: -1}, false)
	same("Kinds", Value{N: 3}, true)
	same("Str", Value{R: ""}, true)
	same("Str", Value{R: "x"}, false)
	same("Ptr", Value{}, false) // a pointer type: a new box each time
	same("Flag", Value{N: 1}, true)
	if a, b := call("Small", Value{N: 3}), call("Kinds", Value{N: 3}); a == b || a.T == b.T {
		t.Error("int(3) and Kind(3) share a box")
	}
}
