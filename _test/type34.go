package main

type original struct {
	Field string
}

func main() {
	type alias original
	type alias2 alias
	var a = &alias2{
		Field: "test",
	}
	println(a.Field)
}

// Output:
// test
