package main

func main() {
	var a interface{}
	switch a.(type) {
	case []int:
	case []string:
	}
	println("bye")
}

// Output:
// bye
