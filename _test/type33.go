package main

func (f *Foo) Bar() int {
	return *f * *f
}

func main() {
}

// Error:
// 3:1: undefined: Foo
