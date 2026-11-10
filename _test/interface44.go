package main

type S struct {
	a int
}

func main() {
	var i any = S{a: 1}

	s, ok := i.(S)
	if !ok {
		println("bad")
		return
	}
	println(s.a)
}

// Output:
// 1
