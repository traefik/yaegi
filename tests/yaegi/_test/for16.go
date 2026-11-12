package main

func main() {
	max := 1
	for ; ; max-- {
		if max == 0 {
			break
		}
		println("in for")
	}
	println("bye")
}

// Output:
// in for
// bye
