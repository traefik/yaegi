package main

func main() {
	for a := 0; true; a++ {
		println(a)
		if a > 0 {
			break
		}
	}
	println("bye")
}

// Output:
// 0
// 1
// bye
