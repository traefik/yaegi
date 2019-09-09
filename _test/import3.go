package main

import "github.com/containous/yaegi/_test/foo"

func main() { println(foo.Bar, foo.Boo) }

// Output:
// init boo
// init foo
// BARR Boo
