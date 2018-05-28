package interp

func Example_export1() {
	src := `package tst

func Exported() { println("Hello from Exported") }
`

	i := NewInterpreter(Opt{})
	i.Eval(src)
	f := (*i.Exports["tst"])["Exported"].(func())
	f()

	// Output:
	// Hello from Exported
}
