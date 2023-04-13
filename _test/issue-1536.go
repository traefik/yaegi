package main

var a [len(prefix+path) + 2]int

const (
	prefix = "/usr/"
	path   = prefix + "local/bin"
)

func main() {
	println(len(a))
}

// Output:
// 21
