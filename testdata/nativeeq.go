// == and != on values of native struct types.
//
// A native struct value (netip.Addr, netip.Prefix, time.Time, image.Point)
// is kept by the VM as a native Go value, not as a list of VM fields. The
// VM == used to walk its fields as VM fields: it found none, so two such
// values were always equal, whatever their content. This program checks
// the places that use ==: plain ==, !=, interfaces, arrays and interpreted
// structs that hold native values, switch, generic code (slices), map keys,
// zero values, copies, pointers, channels, reflect, comparable errors, and the
// panic on uncomparable dynamic types.
package main

import (
	"fmt"
	"image"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"time"
)

type pair struct {
	a netip.Addr
	n int
}

type withTime struct {
	T    time.Time
	Name string
}

type addrErr struct{ a netip.Addr }

func (e addrErr) Error() string { return "addr " + e.a.String() }

func eq[T comparable](x, y T) bool { return x == y }

func neq[T comparable](x, y T) bool { return x != y }

func kind(a, b netip.Addr) string {
	switch a {
	case b:
		return "same"
	case netip.Addr{}:
		return "zero"
	}
	return "other"
}

func ikind(i any, a netip.Addr) string {
	switch i {
	case any(a):
		return "addr"
	case nil:
		return "nil"
	}
	return "other"
}

func mustPanic(name string, f func()) {
	defer func() {
		fmt.Println(name, "panics:", recover() != nil)
	}()
	f()
}

func main() {
	a := netip.MustParseAddr("198.51.100.1")
	b := netip.MustParseAddr("203.0.113.2")
	a2 := netip.AddrFrom4([4]byte{198, 51, 100, 1}) // built another way

	// The reproduction of the issue.
	fmt.Println("addr", a == b, a.Compare(b) == 0, a != b)
	fmt.Println("addr same", a == a2, a != a2)
	fmt.Println("time", time.Unix(1, 0) == time.Unix(2, 0), time.Unix(1, 0) == time.Unix(1, 0))
	t1, t2 := time.Unix(1, 0).UTC(), time.Unix(1, 0).UTC()
	fmt.Println("time utc", t1 == t2, t1 != t2, t1 == t1.Add(time.Second))
	now := time.Now()
	fmt.Println("time mono", now == now, now == now.Round(0), now.Equal(now.Round(0)))

	p1 := netip.MustParsePrefix("10.0.0.0/8")
	p2 := netip.MustParsePrefix("10.0.0.0/16")
	fmt.Println("prefix", p1 == p2, p1 == netip.PrefixFrom(a, 8).Masked() || p1 == netip.MustParsePrefix("10.0.0.0/8"))
	ap1 := netip.MustParseAddrPort("198.51.100.1:80")
	ap2 := netip.MustParseAddrPort("198.51.100.1:81")
	fmt.Println("addrport", ap1 == ap2, ap1 == netip.AddrPortFrom(a, 80))

	// A native struct with exported fields.
	fmt.Println("point", image.Pt(1, 2) == image.Pt(3, 4), image.Pt(1, 2) == image.Point{1, 2})
	fmt.Println("rect", image.Rect(0, 0, 1, 1) == image.Rect(0, 0, 2, 2), image.Rect(0, 0, 1, 1) == image.Rectangle{Max: image.Pt(1, 1)})

	// Zero values.
	var z netip.Addr
	fmt.Println("zero", a == netip.Addr{}, z == netip.Addr{}, z.IsValid(), a != z)
	fmt.Println("zero time", time.Time{} == time.Time{}, t1 == time.Time{})

	// Through interfaces.
	var i1, i2, i3 any = a, b, a2
	fmt.Println("iface", i1 == i2, i1 == i3, i1 != i2, i1 == any(p1))
	var s1, s2 fmt.Stringer = a, b
	fmt.Println("stringer", s1 == s2, s1 == fmt.Stringer(a2))
	fmt.Println("iface time", any(t1) == any(t1.Add(1)), any(t1) == any(t2))
	fmt.Println("iface point", any(image.Pt(1, 2)) == any(image.Pt(1, 3)))

	// Arrays of native structs, and interpreted structs that hold them.
	fmt.Println("array", [2]netip.Addr{a, b} == [2]netip.Addr{a, a}, [2]netip.Addr{a, b} == [2]netip.Addr{a2, b})
	fmt.Println("struct", pair{a, 1} == pair{b, 1}, pair{a, 1} == pair{a2, 1}, pair{a, 1} != pair{b, 1})
	fmt.Println("struct time", withTime{t1, "x"} == withTime{t1.Add(1), "x"}, withTime{t1, "x"} == withTime{t2, "x"})
	fmt.Println("iface struct", any(pair{a, 1}) == any(pair{b, 1}), any([1]netip.Addr{a}) == any([1]netip.Addr{b}))

	// switch on values.
	fmt.Println("switch", kind(a, b), kind(a, a2), kind(netip.Addr{}, b))
	fmt.Println("switch iface", ikind(b, a), ikind(a2, a), ikind(nil, a))

	// Generic code and the slices package.
	fmt.Println("generic", eq(a, b), eq(a, a2), neq(a, b), eq(t1, t1.Add(1)), eq(p1, p2))
	list := []netip.Addr{b, a, b, a2}
	fmt.Println("slices", slices.Contains(list[:1], a), slices.Index(list, a), slices.Equal(list[:2], []netip.Addr{b, b}))
	slices.SortFunc(list, netip.Addr.Compare)
	fmt.Println("compact", len(slices.Compact(list)))

	// Map keys (already right before the fix).
	m := map[netip.Addr]int{a: 1, b: 2}
	m[a2]++
	fmt.Println("map", len(m), m[a], m[b])
	ma := map[[2]netip.Addr]bool{{a, b}: true}
	fmt.Println("map array", ma[[2]netip.Addr{a2, b}], ma[[2]netip.Addr{b, a}])
	ms := map[pair]bool{{a, 1}: true}
	fmt.Println("map struct", ms[pair{a2, 1}], ms[pair{b, 1}])
	mi := map[any]int{a: 1, t1: 2}
	fmt.Println("map any", mi[a2], mi[b], mi[t2])

	// Copies keep the hidden fields.
	c := a
	arr := [1]netip.Addr{a}
	carr := arr
	st := pair{a, 1}
	cst := st
	fmt.Println("copy", c == a, c.String(), carr == arr, carr[0].String(), cst == st, cst.a.String())
	sl := append([]netip.Addr(nil), a, b)
	dst := make([]netip.Addr, 2)
	copy(dst, sl)
	fmt.Println("copy slice", dst[0] == a, dst[1] == b, dst[0] == dst[1])

	// Pointers compare by address, not by content.
	pa, pb := &a, &a2
	fmt.Println("pointer", pa == pb, *pa == *pb, pa == &a)

	// Channels and select.
	ch := make(chan netip.Addr, 2)
	ch <- a
	ch <- b
	var got []netip.Addr
	for range 2 {
		select {
		case v := <-ch:
			got = append(got, v)
		}
	}
	fmt.Println("chan", got[0] == a, got[1] == a, got[0] == got[1])

	// reflect.
	fmt.Println("reflect", reflect.ValueOf(a).Equal(reflect.ValueOf(b)), reflect.ValueOf(a).Equal(reflect.ValueOf(a2)),
		reflect.DeepEqual(a, b), reflect.DeepEqual(a, a2))

	// Comparable error values that hold a native struct. (errors.Is on
	// interpreted error values is handled by its own hooks, see
	// vm/errors.go.)
	var e1, e2 error = addrErr{a}, addrErr{b}
	fmt.Println("error ==", e1 == e2, e1 == error(addrErr{a2}))

	// Uncomparable dynamic types still panic, as in Go.
	mustPanic("builder", func() {
		var x, y any = strings.Builder{}, strings.Builder{}
		_ = x == y
	})
	mustPanic("struct with slice", func() {
		var x, y any = struct{ s []int }{}, struct{ s []int }{}
		_ = x == y
	})
	mustPanic("addr", func() {
		var x, y any = a, b
		_ = x == y
	})
}
