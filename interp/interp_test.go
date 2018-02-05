package interp

import (
	"fmt"
	"testing"
)

func ExampleWalk_1() {
	src := `
package main

func main() {
	println(1)
}
`
	n := Ast(src)
	//n.AstDot()
	n.Walk(func(n *Node) {
		fmt.Println("in:", n.index)
	}, func(n *Node) {
		fmt.Println("out:", n.index)
	})
	// Output:
	// in: 1
	// in: 2
	// out: 2
	// in: 3
	// in: 4
	// out: 4
	// in: 5
	// in: 6
	// out: 6
	// out: 5
	// in: 7
	// in: 8
	// in: 9
	// in: 10
	// out: 10
	// in: 11
	// out: 11
	// out: 9
	// out: 8
	// out: 7
	// out: 3
	// out: 1
}

func BenchmarkWalk(b *testing.B) {
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
	n := Ast(src)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n.Walk(func(n *Node) {}, func(n *Node) {})
	}
}

func ExampleEval_1() {
	src := `
package main

func main() {
	println(1)
}
`
	NewInterpreter().Eval(src)
	// Output:
	// 1
}

func ExampleEval_2() {
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

	NewInterpreter().Eval(src)
	// Output:
	// 1
	// 2048
	// 2304
	// 2560
	// 2816
	// 3072
	// 3328
	// 3584
	// 3840
	// 6144
	// 6400
	// 6656
	// 6912
	// 7168
	// 7424
	// 7680
	// 7936
}
