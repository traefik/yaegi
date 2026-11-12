package main

const buflen = 512

type T struct {
	buf []byte
}

func f(t *T) { *t = T{buf: make([]byte, 0, buflen)} }

func main() {
	s := T{}
	println(cap(s.buf))
	f(&s)
	println(cap(s.buf))
}

// Output:
// 0
// 512
