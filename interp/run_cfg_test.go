package interp

func ExampleRunCfg_1() {
	src := `
package main

func main() {
	println(1)
}
`
	root := SrcToAst(src)
	cfg_entry := root.Child[1].Child[2] // FIXME: entry point should be resolved from 'main' name
	AstToCfg(cfg_entry)
	RunCfg(cfg_entry.Start)
	// Output:
	// 1
}

func ExampleRunCfg_2() {
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

	root := SrcToAst(src)
	cfg_entry := root.Child[1].Child[2] // FIXME: entry point should be resolved from 'main' name
	AstToCfg(cfg_entry)
	RunCfg(cfg_entry.Start)
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
