package main

func use(interface{}) {}

func main() {
	z := map[string]interface{}{"a": 5}
	use(z)
	println("bye")
}

// Output:
// bye
