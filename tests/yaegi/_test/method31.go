package main

import "fmt"

var db dbWrapper

type dbWrapper struct {
	DB *cmap
}

func (d *dbWrapper) get() *cmap {
	return d.DB
}

type cmap struct {
	name string
}

func (c *cmap) f() {
	fmt.Println("in f, c", c)
}

func main() {
	db.DB = &cmap{name: "test"}
	db.get().f()
}

// Output:
// in f, c &{test}
