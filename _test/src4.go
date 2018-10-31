package main

import "github.com/containous/dyngo/_test/provider"

func main() {
	f := provider.Bar
	f()
}

// Output:
// Hello from Foo
