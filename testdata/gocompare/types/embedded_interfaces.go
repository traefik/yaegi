// Interfaces embedded in structs (partial implementation, overriding one
// method, nil embedded interface call recovered), interface embedding in
// interfaces, and overlapping method sets.
package main

import (
	"fmt"
	"sort"
	"strings"
)

type Reader interface{ Read() string }
type Writer interface{ Write(s string) int }
type ReadWriter interface {
	Reader
	Writer
}

// Overlapping embedded interfaces: allowed since Go 1.14.
type RW2 interface {
	ReadWriter
	Reader
	Close() error
}

type buf struct{ data []string }

func (b *buf) Read() string {
	if len(b.data) == 0 {
		return "<empty>"
	}
	s := b.data[0]
	b.data = b.data[1:]
	return s
}
func (b *buf) Write(s string) int { b.data = append(b.data, s); return len(s) }
func (b *buf) Close() error       { return nil }

type upper struct {
	ReadWriter
}

func (u upper) Read() string { return strings.ToUpper(u.ReadWriter.Read()) }

type onlyReader struct {
	Reader
}

// reverse sorts in the opposite order by overriding Less of an embedded
// sort.Interface.
type reverse struct {
	sort.Interface
}

func (r reverse) Less(i, j int) bool { return r.Interface.Less(j, i) }

func main() {
	b := &buf{}
	var rw ReadWriter = b
	fmt.Println(rw.Write("hello"), rw.Write("go"))
	u := upper{rw}
	fmt.Println(u.Read(), u.Write("x"), u.Read(), u.Read(), u.Read())

	var r Reader = u
	var w Writer = u
	_, ok := r.(Writer)
	fmt.Println(ok, w.Write("y"), r.Read())

	var full RW2 = b
	fmt.Println(full.Close(), full.Write("z"), full.Read())
	var rwFromFull ReadWriter = full
	fmt.Println(rwFromFull.Read())

	o := onlyReader{}
	func() {
		defer func() {
			e := recover()
			fmt.Println("recovered:", e)
		}()
		fmt.Println(o.Read())
	}()
	o.Reader = &buf{data: []string{"q"}}
	fmt.Println(o.Read())

	xs := []int{3, 1, 4, 1, 5, 9, 2, 6}
	sort.Sort(reverse{sort.IntSlice(xs)})
	fmt.Println(xs)
	ss := []string{"b", "c", "a"}
	sort.Sort(reverse{sort.StringSlice(ss)})
	fmt.Println(ss)

	// Interface to interface assignment and assertion.
	var any1 any = rw
	if rr, ok := any1.(Reader); ok {
		fmt.Println("reader", rr.Read())
	}
	if _, ok := any1.(RW2); ok {
		fmt.Println("rw2 too")
	}
	if _, ok := any1.(interface{ Missing() }); !ok {
		fmt.Println("no Missing")
	}
}
