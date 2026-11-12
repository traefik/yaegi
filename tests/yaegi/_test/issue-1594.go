package main

func main() {
	var fns []func()
	for _, v := range []int{1, 2, 3} {
		x := v*100 + v
		fns = append(fns, func() { println(x) })
	}
	for _, fn := range fns {
		fn()
	}
}

// Output:
// 101
// 202
// 303
