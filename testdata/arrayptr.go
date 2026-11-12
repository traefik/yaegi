package main

import (
	"bytes"
	"fmt"
	"runtime"
)

// Slice to array pointer conversions ((*[N]T)(s) and [N]T(s)) share the
// backing array of the slice, and storing a whole array or struct keeps
// the pointers into it valid.

type pair struct{ a, b int }

type inner struct {
	n  int
	xs [2]int
}

type outer struct {
	in  inner
	tag string
}

func convert(s []int, n int) (err any) {
	defer func() { err = recover() }()
	switch n {
	case 4:
		_ = (*[4]int)(s)
	case 5:
		_ = [5]int(s)
	}
	return nil
}

func main() {
	s := []int{1, 2, 3, 4, 5, 6}
	p := (*[3]int)(s[1:])
	p[0] = 20
	s[2] = 30
	fmt.Println(s, *p, &p[0] == &s[1])

	*p = [3]int{7, 8, 9}
	fmt.Println(s)
	*(*[3]int)(s[3:]) = *(*[3]int)(s[2:5])
	fmt.Println(s)

	a := [4]int(s)
	a[0] = 100
	fmt.Println(a, s[0])

	fmt.Println(convert(s[:3], 4))
	fmt.Println(convert(s[:3], 5))

	var ns []int
	fmt.Println((*[0]int)(ns) == nil, (*[0]int)(s[:0]) != nil)

	// a byte slice made by native code
	b := bytes.Repeat([]byte("ab"), 3)
	bp := (*[4]byte)(b[1:])
	bp[0] = 'X'
	b[2] = 'Y'
	fmt.Println(string(b), string(bp[:]), [2]byte(b))

	// pointers into an array or a struct see whole stores
	var arr [3]int
	e := &arr[1]
	arr = [3]int{4, 5, 6}
	pa := &arr
	*pa = [3]int{7, 8, 9}
	fmt.Println(*e)

	var o outer
	f := &o.in.xs[1]
	n := &o.in.n
	o = outer{inner{3, [2]int{4, 5}}, "t"}
	fmt.Println(*f, *n)

	ps := []pair{{1, 2}, {3, 4}}
	q := &ps[1].b
	ps[1] = pair{5, 6}
	fmt.Println(*q)

	// slicing an array pointer made from native bytes: out of range
	// bounds give a runtime.Error, as for any slice
	nb := (*[4]byte)(bytes.ToUpper([]byte("abcd")))
	lo, hi, mx := 2, 9, 4
	fmt.Println(sliceErr(func() { _ = nb[lo:hi] }))
	lo, hi = 3, 2
	fmt.Println(sliceErr(func() { _ = nb[lo:hi] }))
	lo, hi, mx = 0, 2, 5
	fmt.Println(sliceErr(func() { _ = nb[lo:hi:mx] }))
	lo = -1
	fmt.Println(sliceErr(func() { _ = nb[lo:] }))
	fmt.Println(sliceErr(func() { _ = nb[1:3:4] }), string(nb[1:3:4]))

	// a large slice of zero-size elements (a zslice in the VM)
	zs := make([]struct{}, 1<<21)
	za := (*[3]struct{})(zs)
	fmt.Println(len(za), za[2], [2]struct{}(zs))
	zb := (*[1 << 21]struct{})(zs)
	fmt.Println(len(zb), zb[1<<20], len(zb[5:]))
	za[1], *zb = struct{}{}, [1 << 21]struct{}{}
	fmt.Println(za == (*[3]struct{})(zs[5:]), &zb[9] == &zs[7], len(zs))
	// A too short zslice panics. The array length stays <= 31: on arm64,
	// ppc64, riscv64 and s390x, Go 1.26 prints a wrong array length in this
	// panic when the length is a constant > 31 and the slice length is not
	// a constant (cmd/compile/internal/arm64/ssa.go: LoweredPanicBoundsCR
	// moves the constant to a register but does not set xIsReg), so `go run`
	// is not a valid reference there. zs[:30] is still a zslice in the VM.
	func() {
		defer func() { fmt.Println(recover()) }()
		_ = (*[31]struct{})(zs[:30])
	}()

	// pointer identity
	x := [3]int{}
	fmt.Println(&x == (*[3]int)(x[:]))
	m := map[*[3]int]int{&x: 1}
	fmt.Println(m[(*[3]int)(x[:])])
}

// sliceErr reports whether f panics with a runtime.Error.
func sliceErr(f func()) (ok bool) {
	defer func() {
		_, ok = recover().(runtime.Error)
	}()
	f()
	return false
}
