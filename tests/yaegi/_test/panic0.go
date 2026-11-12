package main

func main() {
	foo()
}

func foo() {
	bar()
}

func bar() {
	baz()
}

func baz() {
	panic("stop!")
}

// Error:
// stop!
