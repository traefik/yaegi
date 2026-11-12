package main

var aa = [...]int{1, 2, 3}

const maxlen = cap(aa)

var gfm = [maxlen]byte{}

func main() {
	println(len(gfm))
}

// Output:
// 3
