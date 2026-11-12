package vm

import (
	"context"
	"testing"
)

// TestMovJmp checks that the last phi move of an edge and its jump are one
// OpMovJmp, that the loop still computes the right value, and that a
// backward OpMovJmp is counted by the budget like OpJmp (an endless loop
// stops with ErrBudget). Sum is on int64: Sum(50) overflows a 32-bit int.
func TestMovJmp(t *testing.T) {
	const src = `package p

func Sum(n int) int64 {
	var a, b int64 = 0, 1
	for i := 0; i < n; i++ {
		a, b = b, a+b
	}
	return a
}

func Spin() int {
	s := 0
	a, b := 0, 1
	for {
		a, b = b, a+b
		s += a
	}
}
`
	m, spkg := buildSrc(t, src)
	sum := m.funcOf(spkg.Func("Sum"))
	sum.ensure()
	n := 0
	for _, in := range sum.code {
		if in.Op == OpMovJmp {
			n++
		}
	}
	if n == 0 {
		t.Error("Sum: no OpMovJmp")
	}
	if got := m.Call(sum, []Value{{N: 50}}, nil).N; got != 12586269025 {
		t.Errorf("Sum(50) = %d, want 12586269025", got)
	}
	m.Budget = 1e5
	defer func() {
		if r := recover(); r != ErrBudget {
			t.Errorf("Spin: panic %v, want %v", r, ErrBudget)
		}
	}()
	m.callTop(context.Background(), m.funcOf(spkg.Func("Spin")), nil, nil)
}

// TestSwitchTable checks that a switch on an integer with at least
// minSwitch cases is one OpSwitch (switches.go), with a jump table for
// dense keys and a binary search for sparse ones, and that a short one
// keeps its tests. testdata/switchtable.go checks the results against
// go run.
func TestSwitchTable(t *testing.T) {
	const src = `package p

func Dense(x int) int {
	switch x {
	case 0: return 10
	case 1: return 11
	case 2: return 12
	case 3: return 13
	case 5: return 15
	}
	return -1
}

func Sparse(x int64) int {
	switch x {
	case -1 << 40: return 1
	case 7: return 2
	case 1000: return 3
	case 1 << 50: return 4
	}
	return 0
}

func Short(x int) int {
	switch x {
	case 1: return 1
	case 2: return 2
	}
	return 0
}

func Unsigned(x uint64) int {
	switch x {
	case 1<<63 - 2: return 1
	case 1<<63 - 1: return 2
	case 1 << 63: return 3
	case 1<<63 + 1: return 4
	}
	return 0
}

func Spin(x int) int {
	n := 0
	for !(x == 1 || x == 2 || x == 3 || x == 4 || x == 5) {
		n++
	}
	return n
}
`
	m, spkg := buildSrc(t, src)
	sitePC := func(name string) (*switchSite, int) {
		fn := m.funcOf(spkg.Func(name))
		fn.ensure()
		var s *switchSite
		at := -1
		for pc, in := range fn.code {
			if in.Op == OpSwitch {
				if s != nil {
					t.Fatalf("%s: two OpSwitch", name)
				}
				s, at = in.X.(*switchSite), pc
			}
		}
		return s, at
	}
	site := func(name string) *switchSite {
		s, _ := sitePC(name)
		return s
	}
	if s := site("Dense"); s == nil || s.tab == nil || len(s.tab) != 6 || s.lo != 0 {
		t.Errorf("Dense: %+v, want a jump table of 6 entries from 0", s)
	}
	if s := site("Sparse"); s == nil || s.tab != nil || len(s.keys) != 4 {
		t.Errorf("Sparse: %+v, want 4 sorted keys", s)
	}
	if s := site("Short"); s != nil {
		t.Errorf("Short: %+v, want no OpSwitch", s)
	}
	for _, c := range []struct {
		x, want int64
	}{{0, 10}, {3, 13}, {4, -1}, {5, 15}, {-1, -1}, {6, -1}} {
		if got := m.Call(m.funcOf(spkg.Func("Dense")), []Value{{N: c.x}}, nil).N; got != c.want {
			t.Errorf("Dense(%d) = %d, want %d", c.x, got, c.want)
		}
	}
	for _, c := range []struct {
		x, want int64
	}{{-1 << 40, 1}, {7, 2}, {1000, 3}, {1 << 50, 4}, {8, 0}, {-1, 0}} {
		if got := m.Call(m.funcOf(spkg.Func("Sparse")), []Value{{N: c.x}}, nil).N; got != c.want {
			t.Errorf("Sparse(%d) = %d, want %d", c.x, got, c.want)
		}
	}

	// The uint64 keys cross the int64 sign boundary: as registers they are
	// MaxInt64-1, MaxInt64, MinInt64 and MinInt64+1. The span from MinInt64
	// to MaxInt64 is 2^64-1, so this is a binary search, not a table.
	if s := site("Unsigned"); s == nil || s.tab != nil || len(s.keys) != 4 {
		t.Errorf("Unsigned: %+v, want 4 sorted keys", s)
	}
	for _, c := range []struct {
		x    uint64
		want int64
	}{{1<<63 - 2, 1}, {1<<63 - 1, 2}, {1 << 63, 3}, {1<<63 + 1, 4}, {1<<63 - 3, 0}, {1<<63 + 2, 0}, {0, 0}, {1<<64 - 1, 0}} {
		if got := m.Call(m.funcOf(spkg.Func("Unsigned")), []Value{{N: int64(c.x)}}, nil).N; got != c.want {
			t.Errorf("Unsigned(%d) = %d, want %d", c.x, got, c.want)
		}
	}

	// The default edge of the OpSwitch of Spin goes back to the loop body:
	// the budget must count it, so an endless loop stops with ErrBudget.
	s, at := sitePC("Spin")
	if s == nil {
		t.Fatal("Spin: no OpSwitch")
	}
	if int(s.def) >= at {
		t.Fatalf("Spin: default pc %d is not before the OpSwitch at %d", s.def, at)
	}
	if got := m.Call(m.funcOf(spkg.Func("Spin")), []Value{{N: 3}}, nil).N; got != 0 {
		t.Errorf("Spin(3) = %d, want 0", got)
	}
	m.Budget = 1e5
	defer func() {
		if r := recover(); r != ErrBudget {
			t.Errorf("Spin(0): panic %v, want %v", r, ErrBudget)
		}
	}()
	m.callTop(context.Background(), m.funcOf(spkg.Func("Spin")), []Value{{N: 0}}, nil)
	t.Error("Spin(0): returned, want ErrBudget")
}
