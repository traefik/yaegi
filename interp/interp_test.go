package interp

import (
	"testing"
)

func TestWalk_1(t *testing.T) {
	src := `
package main

func main() {
	println(1)
}
`
	n := Ast(src)
	//n.AstDot()
	n.Walk(func(n *Node) {
		println("in:", n.index)
	}, func(n *Node) {
		println("out:", n.index)
	})
}

func TestWalk_2(t *testing.T) {
	src := `
package main

func main() {
	println(1)
}
`
	n := Ast(src)
	n.Walk2(func(n *Node) {
		println("in:", n.index)
	}, func(n *Node) {
		println("out:", n.index)
	})
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

func BenchmarkWalk2(b *testing.B) {
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
		n.Walk2(func(n *Node) {}, func(n *Node) {})
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
