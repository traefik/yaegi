package main

func main() {
	myMap := map[string]int{"a":2}

	for s, _ := range myMap {
		_ = s
	}
	println("ok")
}

// Output:
// ok
