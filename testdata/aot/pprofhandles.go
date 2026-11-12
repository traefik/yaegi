// runtime/pprof keeps the values given to Profile.Add in a map, and
// Remove looks them up. In -aot mode a value of the program reaches pprof
// as an opaque handle: two handles of the same pointer, or of equal
// values, were different, so Remove found nothing and Add of the same
// value twice did not panic. The handles of equal
// values now hold the same payload. In VM mode pprof gets native copies
// (a documented limit).
//
//aot:lib runtime/pprof
package main

import (
	"fmt"
	"runtime/pprof"
)

type res struct{ name string }

func main() {
	p := pprof.NewProfile("prog.res")
	r1, r2 := &res{"a"}, res{"b"}
	p.Add(r1, 0)
	p.Add(r2, 0)
	p.Add(42, 0)
	fmt.Println(p.Count())
	p.Remove(r1)
	p.Remove(r2)
	fmt.Println(p.Count())
	func() {
		defer func() { fmt.Println("duplicate:", recover() != nil) }()
		p.Add(r1, 0)
		p.Add(&res{"a"}, 0)
		p.Add(r1, 0)
	}()
	fmt.Println(p.Count())
}
