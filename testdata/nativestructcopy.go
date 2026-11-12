// A struct value read from a field of a native struct is a copy: when the
// field is assigned again, the copy keeps the old value (as a func or a
// pointer field, see nativefieldcopy.go). The Value kept the addressable
// reflect.Value of the field, so the copy followed the field.
package main

import (
	"fmt"
	"net/http"
	"time"
)

type wrap struct {
	c http.Cookie
}

func show(t time.Time) { fmt.Println("passed:", t.Year()) }

func main() {
	// a struct field of a native struct value
	var c http.Cookie
	c.Expires = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	e := c.Expires
	c.Expires = time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	fmt.Println(e.Year(), c.Expires.Year())

	// through a native pointer
	pc := &http.Cookie{Expires: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
	e2 := pc.Expires
	pc.Expires = time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	fmt.Println(e2.Year(), pc.Expires.Year())

	// a native struct held in an interpreted struct
	var w wrap
	w.c.Expires = time.Now()
	ex := w.c.Expires
	w.c.Expires = time.Time{}
	fmt.Println(ex.IsZero(), w.c.Expires.IsZero())

	// the copy is kept (appended) after the field changes
	var keep []time.Time
	for i := 0; i < 2; i++ {
		e := c.Expires
		c.Expires = c.Expires.AddDate(1, 0, 0)
		keep = append(keep, e)
	}
	fmt.Println(keep[0].Year(), keep[1].Year(), c.Expires.Year())

	// the copy is passed to a function after the field changes
	e3 := c.Expires
	c.Expires = time.Time{}
	show(e3)

	// a shallow load (stored at once) still copies
	ts := []time.Time{pc.Expires}
	pc.Expires = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	fmt.Println(ts[0].Year(), pc.Expires.Year())
}
