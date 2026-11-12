// A pointer to a field of a native struct held by the interpreter (the field
// c of an interpreted struct, an element of a slice) sees the value assigned
// to the whole struct afterwards, as in Go. The assignment replaced the
// native memory of the struct, and the pointer kept the old memory.
package main

import (
	"fmt"
	"net/http"
	"time"
)

type wrap struct {
	n int
	c http.Cookie
}

func main() {
	var w wrap
	w.c.Name = "a"
	pn := &w.c.Name
	w.c = http.Cookie{Name: "b"}
	fmt.Println(*pn, w.c.Name)

	// assigned from another cell: the two structs stay apart
	src := wrap{c: http.Cookie{Name: "c", Expires: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}}
	w.c = src.c
	src.c.Name = "z"
	fmt.Println(*pn, w.c.Name, src.c.Name, w.c.Expires.Year())

	// a self assignment and a write through the pointer
	w.c = w.c
	*pn = "d"
	fmt.Println(w.c.Name)

	// an element of a slice
	s := make([]http.Cookie, 2)
	ps := &s[1].Path
	s[1] = http.Cookie{Path: "/p"}
	fmt.Println(*ps, s[1].Path)

	// the whole interpreted struct is assigned: the pointer sees the new value
	pw := &w.c.Name
	w = wrap{n: 1, c: http.Cookie{Name: "e"}}
	fmt.Println(*pw, w.n, w.c.Name)
}
