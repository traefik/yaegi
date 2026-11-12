// Nil and empty slices and maps: == nil, len, cap, printing with %v %#v %q,
// range, append, and reading a nil map.
package main

import "fmt"

func main() {
	var ns []int
	es := []int{}
	ms := make([]int, 0)
	fmt.Println(ns == nil, es == nil, ms == nil, len(ns), cap(ns), len(es))
	fmt.Printf("%v %v %#v %#v %#v\n", ns, es, ns, es, ms)
	fmt.Println(ns[:] == nil, ns[0:0] == nil, es[:0] == nil)
	for range ns {
		fmt.Println("never")
	}
	var nb []byte
	fmt.Printf("%q %q %x %s|\n", nb, []byte{}, nb, nb)
	fmt.Println(string(nb) == "", len(string(nb)))

	var nm map[string]int
	fmt.Println(nm == nil, len(nm), nm["x"])
	v, ok := nm["x"]
	fmt.Println(v, ok)
	delete(nm, "x")
	for range nm {
		fmt.Println("never")
	}
	fmt.Printf("%v %#v\n", nm, nm)
	em := map[string]int{}
	fmt.Printf("%v %#v %v\n", em, em, em == nil)

	// Writing to a nil map panics.
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		nm["a"] = 1
	}()

	// Nil slice in a struct and an interface.
	type T struct {
		S []string
		M map[int]bool
	}
	var t T
	fmt.Printf("%v %+v %#v\n", t, t, t)
	var i any = ns
	fmt.Println(i == nil, i.([]int) == nil)
	var i2 any
	fmt.Println(i2 == nil)

	// Slicing a nil slice to zero and the empty slice from an array.
	var arr [0]int
	s := arr[:]
	fmt.Println(s == nil, len(s))
	ns = append(ns, 1)
	fmt.Println(ns == nil, ns)
	ns = ns[:0]
	fmt.Println(ns == nil, len(ns))

	// nil to function and returned.
	f := func(x []string) int { return len(x) }
	fmt.Println(f(nil), f([]string{}))
	g := func() []int { return nil }
	fmt.Println(g() == nil)
	var nilp *[3]int
	fmt.Println(nilp == nil, len(nilp))
}
