// Variadic functions: no argument (nil slice), an empty spread slice (not
// nil), spreading a slice (shared backing array), a mix of fixed and variadic
// parameters, variadic interface{} and its nil, variadic function values and
// methods, and append/fmt as variadic callees.
package main

import (
	"fmt"
	"strings"
)

func info(xs ...int) string {
	return fmt.Sprintf("len=%d cap=%d nil=%v %v", len(xs), cap(xs), xs == nil, xs)
}

func sum(xs ...int) (s int) {
	for _, x := range xs {
		s += x
	}
	return
}

func mutate(xs ...int) {
	for i := range xs {
		xs[i] *= 10
	}
}

func join(sep string, parts ...string) string { return strings.Join(parts, sep) }

func anys(vs ...interface{}) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d:", len(vs))
	for _, v := range vs {
		fmt.Fprintf(&b, " %T(%v)", v, v)
	}
	return b.String()
}

type logger struct{ prefix string }

func (l logger) logf(format string, args ...interface{}) string {
	return l.prefix + fmt.Sprintf(format, args...)
}

func forward(xs ...int) string { return info(xs...) }

func main() {
	fmt.Println(info())
	fmt.Println(info(nil...))
	fmt.Println(info([]int{}...))
	fmt.Println(info(1))
	fmt.Println(info(1, 2, 3))
	s := make([]int, 2, 5)
	fmt.Println(info(s...))
	fmt.Println(sum(), sum(1), sum(1, 2, 3, 4), sum(s...))
	data := []int{1, 2, 3}
	mutate(data...)
	fmt.Println(data)
	mutate(data[1:]...)
	fmt.Println(data)
	mutate(4, 5)
	fmt.Println(join("-"), join("-", "a"), join(", ", "a", "b", "c"))
	words := []string{"x", "y"}
	fmt.Println(join("+", words...))
	fmt.Println(anys())
	fmt.Println(anys(nil))
	fmt.Println(anys(nil, 1, "s", 2.5, []int{1}))
	var nilIface []interface{}
	fmt.Println(anys(nilIface...))
	fmt.Println(anys([]interface{}{nil, nil}...))
	fmt.Println(anys([]interface{}{[]interface{}{1, 2}}...))
	fmt.Println(anys([]interface{}{1, 2}))
	l := logger{"[log] "}
	fmt.Println(l.logf("%d-%s", 3, "x"))
	fmt.Println(l.logf("plain"))
	f := l.logf
	fmt.Println(f("%v %v", true, nil))
	g := logger.logf
	fmt.Println(g(logger{"<g> "}, "%03d", 7))
	var vf func(...int) int = sum
	fmt.Println(vf(5, 5), vf())
	fmt.Println(forward(), forward(9, 8))
	fmt.Printf("%T %T\n", sum, join)
	b := []byte("ab")
	b = append(b, "cd"...)
	b = append(b, []byte{}...)
	fmt.Println(string(b))
	var ns []int
	ns = append(ns)
	fmt.Println(ns == nil, len(ns))
	ns = append(ns, []int(nil)...)
	fmt.Println(ns == nil)
	ns = append([]int{}, ns...)
	fmt.Println(ns == nil)
	// Spread shares the array with the caller; the plain form copies.
	arr := []int{1, 2, 3}
	keep := func(xs ...int) []int { return xs }
	k1 := keep(arr...)
	k2 := keep(arr[0], arr[1], arr[2])
	arr[0] = 99
	fmt.Println(k1, k2)
	fn := func(prefix string, rest ...func() string) string {
		out := prefix
		for _, r := range rest {
			out += r()
		}
		return out
	}
	fmt.Println(fn("p:", func() string { return "a" }, func() string { return "b" }))
}
