package main

import (
	"github.com/containous/gi/interp"
)

func main() {
	src := `
package main

func main() {
	println(1)
	for a := 0; a < 10000; a++ {
		if (a & 0x8ff) == 0x800 {
			println(a)
		}
	}
}
`
	i := interp.NewInterpreter()
	i.Eval(src)
}
