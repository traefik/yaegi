package main

func f1(a int) interface{} { return a + 1 }

func main() {
	c := f1(3)
	println(c.(int))
}

// Output:
// 4
