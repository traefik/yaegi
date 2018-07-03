package main

import "github.com/containous/gi/_test/provider"

func main() {
	t := provider.T1{"myName"}
	t.Info()
}

// Output:
// myName
