package main

const regSizeMaskUint32 = ^uint(0)

func main() {
	var dist uint32 = 8
	var extra uint32
	nb := uint(dist-2) >> 1
	dist = 1<<((nb+1)&regSizeMaskUint32) + 1 + extra
	println(dist)
}

// Output:
// 17
