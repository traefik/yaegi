package main

import "fmt"

func f1(m map[string]string) {
	defer delete(m, "foo")
	defer delete(m, "test")

	fmt.Println(m)
}

func main() {
	m := map[string]string{
		"foo": "bar",
		"baz": "bat",
	}
	f1(m)

	fmt.Println(m)
}

// Output:
// map[baz:bat foo:bar]
// map[baz:bat]
