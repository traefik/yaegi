package main

func main() {
	a := [256]int{}
	var b uint8 = 12
	a[b] = 1
	println(a[b])
}

// Output:
// 1
