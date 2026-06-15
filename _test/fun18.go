package main

var m = map[string]int{"foo": 1, "bar": 2}

func f(s string) any { return m[s] }

func main() {
	println(f("foo").(int))
}

// Output:
// 1
