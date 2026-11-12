package main

func main() {
	var a interface{}
	a = []int{3}
	switch a.(type) {
	case []int:
		println("a is []int")
	case []string:
		println("a is []string")
	}

	var b interface{}
	b = []string{"hello"}
	switch b.(type) {
	case []int:
		println("b is []int")
	case []string:
		println("b is []string")
	}
	println("bye")
}

// Output:
// a is []int
// b is []string
// bye
