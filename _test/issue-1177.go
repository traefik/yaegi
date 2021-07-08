package main

type counters [3][16]int

func main() {
	cs := &counters{}
	p := &cs[0][1]
	*p = 2
	println(cs[0][1])
}

// Output:
// 2
