package main

func f(b uint) uint {
	return uint(1) + (0x1 >> b)
}

func main() {
	println(f(1))
}

// Output:
// 1
