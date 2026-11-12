package main

func f1(a int) interface{} { return a + 1 }

func f2(a int) interface{} { return f1(a) }

func main() {
	c := f2(3)
	println(c.(int))
}

// Output:
// 4
