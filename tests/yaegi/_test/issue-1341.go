package main

import (
	"fmt"
	"time"
)

type Foo struct {
	Loc func() string
}

func main() {
	f := Foo{(&time.Location{}).String}
	fmt.Printf("%q\n", f.Loc())

	loc := time.FixedZone("XYZ", 3600)
	g := Foo{Loc: loc.String}
	fmt.Println(g.Loc())
}

// Output:
// ""
// XYZ
