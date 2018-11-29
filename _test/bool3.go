package main

func main() {
	m := map[int]bool{0: false, 1: true}
	if m[0] {
		println(0)
	} else {
		println(1)
	}
}

// Output:
// 1
