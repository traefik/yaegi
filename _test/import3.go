package main

import "./foo"

func main() { println(foo.Bar, foo.Boo) }

// Output:
// init boo
// init foo
// BARR Boo
