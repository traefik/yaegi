package vm

import (
	"errors"
	"go/types"
	"runtime"
	"testing"
)

// TestReflect2NewArrayBounds checks that reflect2's unsafe_NewArray has the
// bounds of make([]T, n): a plugin may ship its own reflect2 and pass any
// n, and the VM must refuse what would end the host with a fatal
// out-of-memory (n zero values of a large struct, n elements of a type of
// size 0) instead of allocating it.
func TestReflect2NewArrayBounds(t *testing.T) {
	m := &Machine{}
	big := types.NewStruct([]*types.Var{
		types.NewField(0, nil, "a", types.NewArray(types.Typ[types.Byte], 1<<10), false),
	}, nil)
	empty := types.NewStruct(nil, nil)
	for _, c := range []struct {
		name string
		et   types.Type
		n    int64
	}{
		{"negative", types.Typ[types.Int], -1},
		{"gc length limit", types.Typ[types.Int64], 1 << 62},
		{"zero values of a large struct", big, 1 << 20},
		{"elements of size 0", empty, 1 << 40},
	} {
		func() {
			defer func() {
				r := recover()
				err, ok := r.(error)
				var le *LoadError
				var re runtime.Error
				if !ok || !(errors.As(err, &le) || errors.As(err, &re)) {
					t.Errorf("%s: recover() = %v, want a refusal", c.name, r)
				}
			}()
			m.r2NewArray(c.et, c.n)
		}()
	}
	// small arrays still work
	if p := m.r2NewArray(empty, 3); p.R == nil {
		t.Error("unsafe_NewArray of 3 empty structs: nil pointer")
	}
	if p := m.r2NewArray(big, 2); p.R == nil {
		t.Error("unsafe_NewArray of 2 large structs: nil pointer")
	}
}
