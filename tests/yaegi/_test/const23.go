package main

const maxlen = len("hello")

var gfm = [maxlen]byte{}

func main() {
	println(len(gfm))
}

// Output:
// 5
