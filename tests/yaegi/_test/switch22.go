package main

type T struct {
	Name string
}

func f(t interface{}) {
	switch ext := t.(type) {
	case *T:
		println("*T", ext.Name)
	default:
		println("unknown")
	}
}

func main() {
	f(&T{"truc"})
}

// Output:
// *T truc
