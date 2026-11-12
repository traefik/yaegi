package vm

import (
	"math"
	"testing"
)

// TestAllocReuse checks the allocations that the VM no longer makes per
// operation: the iterator of a range over an interpreted map or a string
// goes back to a pool at the end of the loop (features.go), a small integer
// in an interface uses a static box (boxes.go), a closure is one object
// (closures.go).
func TestAllocReuse(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector allocates on its own: the counts mean nothing")
	}
	const src = `package p

var m = map[string]int{"a": 1, "b": 2, "c": 3}
var big = map[int]int{}

func init() {
	for i := 0; i < 20; i++ {
		big[i] = i
	}
}

func RangeMap() (n int) {
	for k, v := range m {
		n += len(k) + v
	}
	return
}

func RangeBig() (n int) {
	for k, v := range big {
		n += k + v
	}
	return
}

func RangeStr() (n int) {
	for _, r := range "héllo" {
		n += int(r)
	}
	return
}

type Kind int

var sink any

func Box() {
	for i := 0; i < 10; i++ {
		sink = Kind(i)
	}
}

var fsink func() int

func Closure(a, b int) {
	fsink = func() int { return a + b }
}
`
	m, spkg := buildSrc(t, src)
	m.Call(m.funcOf(spkg.Func("init")), nil, nil)
	allocs := func(name string, args ...Value) float64 {
		fn := m.funcOf(spkg.Func(name))
		m.Call(fn, args, nil)
		n := math.Inf(1)
		for range 3 {
			n = min(n, testing.AllocsPerRun(100, func() { m.Call(fn, args, nil) }))
		}
		return n
	}
	for _, c := range []struct {
		name string
		args []Value
		max  float64
	}{
		{"RangeMap", nil, 0},
		{"RangeBig", nil, 0},
		{"RangeStr", nil, 0},
		{"Box", nil, 0},
		{"Closure", []Value{{N: 1}, {N: 2}}, 3}, // the cells of a and b, and the closure (4 before)
	} {
		if got := allocs(c.name, c.args...); got > c.max {
			t.Errorf("%s: %.0f allocations per call, want at most %.0f", c.name, got, c.max)
		}
	}
}
