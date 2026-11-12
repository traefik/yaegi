// Native boundary of fmt (reflect walk): named basic types, types without
// methods and map sorting, given to the print functions of fmt
// (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// each calls every print function of fmt with the same arguments.
func each(format string, args ...any) {
	out("Sprint %q", fmt.Sprint(args...))
	out("Sprintln %q", fmt.Sprintln(args...))
	out("Sprintf %q", fmt.Sprintf(format, args...))
	var b strings.Builder
	fmt.Fprint(&b, args...)
	out("Fprint %q", b.String())
	b.Reset()
	fmt.Fprintln(&b, args...)
	out("Fprintln %q", b.String())
	b.Reset()
	fmt.Fprintf(&b, format, args...)
	out("Fprintf %q", b.String())
	out("Append %q", string(fmt.Append([]byte("A:"), args...)))
	out("Appendln %q", string(fmt.Appendln([]byte("A:"), args...)))
	out("Appendf %q", string(fmt.Appendf([]byte("A:"), format, args...)))
	out("Errorf %q", fmt.Errorf(format, args...).Error())
	fmt.Print(append([]any{cur + ": Print "}, args...)...)
	fmt.Print("\n")
	fmt.Println(append([]any{cur + ": Println"}, args...)...)
	fmt.Printf(cur+": Printf "+format+"\n", args...)
}

// verbs prints each value with each verb.
func verbs(vs []any, vbs ...string) {
	for i, v := range vs {
		for _, vb := range vbs {
			out("%d %s %s", i, vb, fmt.Sprintf(vb, v))
		}
	}
}

// Named basic types: fmt must not take the fast path of the basic type.
type (
	B    bool
	C64  complex64
	C128 complex128
	F32  float32
	F64  float64
	I    int
	I8   int8
	I16  int16
	I32  int32
	I64  int64
	S    string
	U    uint
	U8   uint8
	U16  uint16
	U32  uint32
	U64  uint64
	Uptr uintptr
	Bs   []byte
	Bs8  []U8
	Arr4 [4]U8
)

// Types without methods.
type pt struct{ X, Y int }

type emb struct {
	pt
	Name string
	tags []string
	m    map[string]int
}

type (
	ns     []pt
	nm     map[string]pt
	nf     func(int) string
	nc     chan int
	na     [2]pt
	nptr   *pt
	niface interface{ M() }
)

type deep struct {
	A []map[string][]any
	b *pt
}

// gbox is a generic type without methods.
type gbox[T any] struct {
	V    T
	next *gbox[T]
}

// ik is a struct with an interface field, used as a map key.
type ik struct{ K any }

// sortedMap prints the entries of a map with mixed key types in sorted
// order: fmt sorts different types by an internal order.
func sortedMap(s string) string {
	s = strings.TrimSuffix(strings.TrimPrefix(s, "map["), "]")
	parts := strings.Fields(s)
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func main() {
	named := []any{
		B(true), C64(1 + 2i), C128(-1.5i), F32(1.25), F64(math.Pi), I(-7), I8(-8), I16(16), I32('x'), I64(1 << 40),
		S("str"), U(97), U8(200), U16(65535), U32('é'), U64(1 << 63), Uptr(0x1234), Bs("bytes"), Bs8{104, 105}, Arr4{65, 66, 67, 68},
	}
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln assert x.(T), T unnamed
	probe("Unnamed", func() {
		verbs(named, "%v", "%+v", "%#v", "%T", "%d", "%s", "%q", "%x", "%X", "%t", "%g", "%.2f", "%08.3e", "%c", "%U", "%b", "%o", "%6v|")
		each("%v %v %d %s %x %t", I(1), S("s"), U8(2), Bs("b"), Bs8{1, 2}, B(false))
		var a any = F64(2.5)
		each("%v %.1f %v", a, F32(1.5), C128(1i))
		verbs([]any{
			[]any{I(1), S("a"), Bs("b"), B(true)},
			map[S]I{"b": 2, "a": 1},
			map[I]Bs{2: Bs("two"), 1: nil},
			[2]any{F64(1), U8(2)},
			struct {
				I I
				s S
				b Bs
			}{1, "s", Bs("x")},
			[]Bs{Bs("a"), Bs("b")},
			[]S{"x", "y"},
		}, "%v", "%+v", "%#v", "%d", "%s", "%q", "%x", "%T")
	})
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln reflect type name
	probe("BadVerb", func() {
		verbs([]any{S("s"), I(1), B(true), F64(1), Bs("b"), C64(1), pt{1, 2}, ns{{1, 2}}, nptr(nil), (*pt)(nil), nf(nil), nc(nil), nil},
			"%d", "%s", "%t", "%z", "%e")
		verbs([]any{S("s"), I(1), pt{}, nptr(nil), (*pt)(nil), nf(nil), nc(nil), ns(nil), nm(nil), nil}, "%p", "%#p")
		each("%d|%t|%z|%!", S("x"), I(2), pt{3, 4})
		each("%d %d", I(1))
		each("%d", I(1), S("extra"), pt{5, 6})
	})
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln reflect walk
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln reflect length
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln reflect kind
	probe("Walk", func() {
		vals := []any{
			pt{1, 2}, &pt{3, 4}, emb{pt{5, 6}, "e", []string{"t"}, map[string]int{"k": 1}}, &emb{},
			ns{{1, 2}, {3, 4}}, ns(nil), &ns{{7, 8}}, nm{"b": {2, 2}, "a": {1, 1}}, nm(nil), &nm{"x": {}},
			nf(nil), nc(nil), na{{1, 1}, {2, 2}}, &na{}, nptr(nil),
			deep{A: []map[string][]any{{"k": {pt{1, 1}, I(2), nil, ns{{3, 3}}}}}},
			gbox[int]{V: 1}, gbox[string]{V: "s"}, gbox[pt]{V: pt{9, 9}}, &gbox[ns]{V: ns{{1, 2}}},
			[]any{pt{1, 2}, ns{{3, 4}}, nm{"k": {5, 6}}, na{}, gbox[int]{V: 2}},
			[3]any{emb{}, nil, deep{}},
			map[string]any{"pt": pt{1, 2}, "ns": ns{{3, 4}}, "g": gbox[bool]{V: true}},
			struct {
				P  pt
				p  pt
				Ns ns
				ns ns
				A  any
				a  any
			}{pt{1, 1}, pt{2, 2}, ns{{3, 3}}, ns{{4, 4}}, pt{5, 5}, nm{"k": {6, 6}}},
			[]niface{nil}, map[pt]string{{2, 1}: "b", {1, 2}: "a", {1, 1}: "c"},
			map[na]int{{{2, 0}}: 2, {{1, 0}}: 1}, map[I]S{3: "c", 1: "a", 2: "b"}, map[S]B{"y": true, "x": false},
			map[F64]int{F64(math.NaN()): 0, 2: 2, -1: 1, F64(math.Inf(1)): 3}, map[B]int{true: 1, false: 0},
			map[C128]int{2i: 2, 1: 1, 1i: 0}, map[U8]int{2: 2, 1: 1}, map[Arr4]int{{66, 66, 66, 66}: 1, {65, 65, 65, 65}: 0},
			[]Arr4{{65, 66, 67, 68}}, [2]U8{104, 105}, []U8{104, 105}, [0]pt{}, []ns{nil, {}},
		}
		verbs(vals, "%v", "%+v", "%#v", "%T", "%d", "%s", "%x", "%q")
		f := nf(func(i int) string { return "" })
		c := make(nc, 2)
		out("%T %T %T %t %t %d %d", f, c, gbox[nf]{}, f == nil, c == nil, len(c), cap(c))
		each("%v %+v %#v", pt{1, 2}, ns{{3, 4}}, nm{"a": {5, 6}})
	})
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln panic == of two interfaces: comparing uncomparable type
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln panic panic message with the type name
	probe("MapSort", func() {
		// keys of one program type are sorted by value
		m1 := map[any]int{pt{2, 1}: 3, pt{1, 2}: 2, pt{1, 1}: 1}
		m2 := map[any]string{I(3): "c", I(1): "a", I(2): "b"}
		m3 := map[any]int{ik{I(2)}: 2, ik{I(1)}: 1, ik{nil}: 0}
		m4 := map[any]int{nil: 0, S("x"): 1}
		m5 := map[any]int{na{{2, 2}}: 2, na{{1, 1}}: 1}
		for i, m := range []any{m1, m2, m3, m4, m5} {
			out("%d %v", i, m)
			out("%d %#v", i, m)
		}
		// keys of different types: print the entries sorted here
		mixed := map[any]int{I(1): 1, S("s"): 2, pt{1, 2}: 3, B(true): 4, U8(5): 5, F64(6): 6, na{}: 7, ik{S("k")}: 8, 9: 9, "s": 10}
		out("mixed %d: %s", len(mixed), sortedMap(fmt.Sprint(mixed)))
		out("mixed %%+v: %s", sortedMap(fmt.Sprintf("%+v", mixed)))
		mk := map[ik]int{{I(1)}: 1, {S("a")}: 2, {I(0)}: 0}
		out("ik mixed %d: %s", len(mk), sortedMap(fmt.Sprint(mk)))
		each("%v", map[any]int{pt{2, 2}: 2, pt{1, 1}: 1})
		// a map with keys whose dynamic type is not comparable can not be built
		func() {
			defer func() { out("insert uncomparable key: %v", recover()) }()
			bad := map[any]int{}
			bad[ns{{1, 1}}] = 1
		}()
	})
}
