// Native boundary of sort: program types given to sort (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"sort"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

type person struct {
	name string
	age  int
}

// people is a named slice type.
type people []person

// list is a generic slice type.
type list[T any] []T

// byAge implements sort.Interface with value receivers.
type byAge []person

func (a byAge) Len() int           { return len(a) }
func (a byAge) Less(i, j int) bool { return a[i].age < a[j].age }
func (a byAge) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// byName implements sort.Interface with pointer receivers.
type byName struct{ p []person }

func (a *byName) Len() int           { return len(a.p) }
func (a *byName) Less(i, j int) bool { return a.p[i].name < a.p[j].name }
func (a *byName) Swap(i, j int)      { a.p[i], a.p[j] = a.p[j], a.p[i] }

func data() []person {
	return []person{{"carl", 30}, {"ann", 25}, {"bob", 35}, {"dan", 25}, {"eve", 30}}
}

func main() {
	// row: sort Slice,SliceStable reflect *
	probe("Slice", func() {
		p := people(data())
		sort.Slice(p, func(i, j int) bool { return p[i].name < p[j].name })
		out("people by name: %v", p)
		p = people(data())
		sort.SliceStable(p, func(i, j int) bool { return p[i].age < p[j].age })
		out("people stable by age: %v", p)
		l := list[int]{5, 2, 8, 1}
		sort.Slice(l, func(i, j int) bool { return l[i] < l[j] })
		out("list: %v", l)
		ls := list[string]{"b", "c", "a"}
		sort.SliceStable(ls, func(i, j int) bool { return ls[i] > ls[j] })
		out("list desc: %v", ls)
		var x any = []person(data())
		sort.Slice(x, func(i, j int) bool { return x.([]person)[i].age > x.([]person)[j].age })
		out("in any: %v", x)
		a := [3]int{3, 1, 2}
		sort.Slice(a[:], func(i, j int) bool { return a[i] < a[j] })
		out("array slice: %v", a)
		e := people{}
		sort.Slice(e, func(i, j int) bool { return true })
		out("empty: %v", e)
		var n people
		sort.SliceStable(n, func(i, j int) bool { return true })
		out("nil: %v", n)
	})
	// row: sort Slice,SliceStable reflect walk
	probe("Slice/panic", func() {
		for _, x := range []any{person{"a", 1}, 3, map[string]int{"a": 1}, &[]int{1, 2}} {
			func() {
				defer func() {
					r := recover()
					out("%T: panic %T: %v", x, r, r)
				}()
				sort.Slice(x, func(i, j int) bool { return false })
				out("%T: no panic", x)
			}()
			func() {
				defer func() {
					r := recover()
					out("stable %T: panic %T: %v", x, r, r)
				}()
				sort.SliceStable(x, func(i, j int) bool { return false })
				out("stable %T: no panic", x)
			}()
		}
	})
	// row: sort SliceIsSorted reflect *
	probe("SliceIsSorted", func() {
		p := people(data())
		out("people: %v", sort.SliceIsSorted(p, func(i, j int) bool { return p[i].name < p[j].name }))
		sort.Slice(p, func(i, j int) bool { return p[i].name < p[j].name })
		out("sorted: %v", sort.SliceIsSorted(p, func(i, j int) bool { return p[i].name < p[j].name }))
		l := list[int]{1, 2, 3}
		out("list: %v", sort.SliceIsSorted(l, func(i, j int) bool { return l[i] < l[j] }))
		out("empty: %v", sort.SliceIsSorted(people{}, func(i, j int) bool { return true }))
		func() {
			defer func() {
				r := recover()
				out("int: panic %T: %v", r, r)
			}()
			out("int: %v", sort.SliceIsSorted(3, func(i, j int) bool { return true }))
		}()
	})
	// sort.Interface of the program and of sort, with Sort and Stable.
	probe("Sort/Interface", func() {
		a := byAge(data())
		sort.Sort(a)
		out("byAge: %v", a)
		a = byAge(data())
		sort.Stable(a)
		out("stable byAge: %v", a)
		n := &byName{data()}
		sort.Sort(n)
		out("byName: %v", n.p)
		a = byAge(data())
		sort.Sort(sort.Reverse(a))
		out("reverse byAge: %v %v", a, sort.IsSorted(sort.Reverse(a)))
	})
	// sort.IntSlice and friends: the sort is in place.
	probe("Sort/IntSlice", func() {
		s := []int{3, 1, 2}
		sort.Sort(sort.IntSlice(s))
		out("Sort IntSlice: %v", s)
		s = []int{3, 1, 2}
		sort.Stable(sort.IntSlice(s))
		out("Stable IntSlice: %v", s)
		s = []int{3, 1, 2}
		sort.Sort(sort.Reverse(sort.IntSlice(s)))
		out("Sort Reverse IntSlice: %v", s)
		s = []int{3, 1, 2}
		sort.Stable(sort.Reverse(sort.IntSlice(s)))
		out("Stable Reverse IntSlice: %v", s)
		f := []float64{2.5, -1, 3}
		sort.Sort(sort.Float64Slice(f))
		out("Sort Float64Slice: %v", f)
		st := []string{"b", "c", "a"}
		sort.Sort(sort.StringSlice(st))
		out("Sort StringSlice: %v", st)
		is := sort.IntSlice{9, 8, 7}
		is.Sort()
		out("IntSlice.Sort: %v", is)
		var iface sort.Interface = sort.IntSlice{5, 4}
		sort.Sort(iface)
		out("in interface: %v", iface)
	})
}
