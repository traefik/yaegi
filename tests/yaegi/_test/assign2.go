package main

func main() {
	r := uint32(2000000000)
	r = hello(r)
	println(r)
}

func hello(r uint32) uint32 { return r + 1 }

// Output:
// 2000000001
