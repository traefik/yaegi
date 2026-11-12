package main

func main() {
	println("foo")
	goto L1
	println("Hello")
L1:
	println("bar")
	println("bye")
}

// Output:
// foo
// bar
// bye
