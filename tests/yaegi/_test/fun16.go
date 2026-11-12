package main

func f1(a int) int { return a + 1 }

func f2(a int) interface{} {
	// TODO: re-enable the optimized case below, once we've figured out why it
	// interferes with the empty interface model.
	// return f1(a)
	var foo interface{} = f1(a)
	return foo
}

func main() {
	c := f2(3)
	println(c.(int))
}

// Output:
// 4
