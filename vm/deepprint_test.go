package vm

import (
	"go/types"
	"testing"
)

// TestNestedDeepProbeBound checks the probe of deepprint.go on a value it
// cannot finish: a DAG of []any that shares its halves has 2^25 paths in a
// few cells. The probe looks at nestProbeMax values and must then say
// "deep": a wide part looked at first must not hide a deep part, which the
// AOT copy of fmt would walk on the host stack until a fatal overflow.
func TestNestedDeepProbeBound(t *testing.T) {
	m := &Machine{}
	anyT := types.NewInterfaceType(nil, nil)
	sliceT := &RType{T: types.NewSlice(anyT)}
	intT := &RType{T: types.Typ[types.Int]}
	v := Value{R: &Iface{T: intT, V: Value{N: 1}}}
	for range 25 {
		v = Value{R: &Iface{T: sliceT, V: Value{R: []Value{v, v}}}}
	}
	if !m.nestedDeep(v, maxBoxDepth, false) {
		t.Error("a value the probe cannot finish: nestedDeep = false, want true")
	}
	small := Value{R: &Iface{T: sliceT, V: Value{R: []Value{{R: &Iface{T: intT, V: Value{N: 1}}}}}}}
	if m.nestedDeep(small, maxBoxDepth, false) {
		t.Error("[]any{1}: nestedDeep = true, want false")
	}
}
