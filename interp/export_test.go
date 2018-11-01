package interp

func Example_export() {
	src := `package tst

func Exported() { println("Hello from Exported") }
`

	i := NewInterpreter(Opt{}, "export_test")
	i.Eval(src)
	f := (*i.Exports["tst"])["Exported"].(func())
	f()

	// Output:
	// Hello from Exported
}
