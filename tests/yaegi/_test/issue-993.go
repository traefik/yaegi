package main

var m map[string]int64

func initVar() {
	m = make(map[string]int64)
}

func main() {
	initVar()
	println(len(m))
}

// Output:
// 0
