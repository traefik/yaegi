package main

type T struct {
	num [tnum + 2]int
}

const tnum = 23

func main() {
	t := T{}
	println(len(t.num))
}

// Output:
// 25
