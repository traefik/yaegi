package main

func f(x int) (int, int) { return x, "foo" }

func main() {
	print("hello")
}

// Error:
// cannot use "foo" (type stringT) as type intT in return argument
