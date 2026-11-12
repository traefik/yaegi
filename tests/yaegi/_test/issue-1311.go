package main

type T struct {
	v interface{}
}

func f() (ret int64, err error) {
	ret += 2
	return
}

func main() {
	t := &T{}
	t.v, _ = f()
	println(t.v.(int64))
}

// Output:
// 2
