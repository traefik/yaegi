package main

func f1(a int) interface{} { return a + 1 }

func f2(a int64) interface{} { return a + 1 }

func main() {
	c := f1(3)
	println(c.(int))
	b := f2(3)
	println(b.(int64))
}

// Output:
// 4
// 4
