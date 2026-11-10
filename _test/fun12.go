package main

func use(any) {}

func main() {
	z := map[string]any{"a": 5}
	use(z)
	println("bye")
}

// Output:
// bye
