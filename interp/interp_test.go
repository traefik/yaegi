package interp

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
