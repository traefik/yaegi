package main

type T1 struct {
	Name string
}

func (t T1) genAdd(k int) func(int) int {
	return func(i int) int {
		println(t.Name)
		return i + k
	}
}

var t = T1{"test"}

func main() {
	f := t.genAdd(4)
	println(f(5))
}

// Output:
// test
// 9
