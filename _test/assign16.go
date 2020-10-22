package main

type H struct {
	bits uint
}

func main() {
	h := &H{8}
	var x uint = (1 << h.bits) >> 6

	println(x)
}

// Output:
// 4
