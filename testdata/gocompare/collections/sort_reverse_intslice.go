// Methods of sort.IntSlice and sort.Float64Slice, called through sort.Reverse and sort.Sort.
package main

import (
	"fmt"
	"math"
	"sort"
)

func main() {
	s := []int{1, 3, 2}
	sort.Sort(sort.IntSlice(s))
	fmt.Println("a", s)
	s = []int{1, 3, 2}
	var x sort.Interface = sort.IntSlice(s)
	x.Swap(0, 1)
	fmt.Println("b", s, x.Len(), x.Less(0, 1))
	s = []int{1, 3, 2}
	r := sort.Reverse(sort.IntSlice(s))
	fmt.Println("c", r.Len(), r.Less(0, 1))
	r.Swap(0, 1)
	fmt.Println("d", s)
	sort.Sort(r)
	fmt.Println("e", s)
	sort.Stable(r)
	fmt.Println("f", s)
	f := sort.Float64Slice{3, 1, 2}
	f.Sort()
	fmt.Println("g", f)
	f = sort.Float64Slice{3, 1, 2}
	sort.Sort(f)
	fmt.Println("h", f)
	f = sort.Float64Slice{3, 1, 2}
	sort.Stable(f)
	fmt.Println("i", f)

	f = sort.Float64Slice{2, math.NaN(), 1, math.Inf(-1)}
	sort.Sort(sort.Reverse(f))
	fmt.Println("j", f, sort.IsSorted(f), sort.IsSorted(sort.Reverse(f)))
	ss := []string{"b", "c", "a"}
	sort.Sort(sort.Reverse(sort.StringSlice(ss)))
	fmt.Println("k", ss)
	s = []int{5, 1, 4}
	r = sort.Reverse(sort.IntSlice(s))
	fmt.Println("l", r)
	fmt.Printf("m %v %+v %#v %T\n", r, r, r, r)
	sort.Sort(sort.Reverse(byLen{"ccc", "a", "bb"}))
	m := byLen{"ccc", "a", "bb"}
	sort.Sort(sort.Reverse(m))
	fmt.Println("n", m)
	sort.Stable(m)
	fmt.Println("o", m, sort.IsSorted(m))
	s = []int{3, 1, 2}
	sort.Sort(sort.Reverse(sort.Reverse(sort.IntSlice(s))))
	fmt.Println("p", s)
	func() {
		defer func() { fmt.Println("q", recover()) }()
		sort.Sort(nil)
	}()
	ip := &sort.IntSlice{3, 1, 2}
	sort.Sort(ip)
	fmt.Println("r", *ip, sort.IsSorted(ip))
	fp := &sort.Float64Slice{3, 1, 2}
	sort.Stable(fp)
	fmt.Println("s", *fp)
	sp := &sort.StringSlice{"c", "a", "b"}
	sort.Sort(sp)
	fmt.Println("t", *sp)
	ip = &sort.IntSlice{1, 3, 2}
	sort.Sort(sort.Reverse(ip))
	fmt.Println("u", *ip)
	ip = &sort.IntSlice{3, 1, 2}
	r = sort.Reverse(ip)
	*ip = sort.IntSlice{1, 2, 3, 4, 5}
	sort.Sort(r)
	fmt.Println("v", r.Len(), *ip)
	fmt.Printf("w %#v %v\n", sort.Reverse(sort.IntSlice(nil)), sort.Reverse(sort.StringSlice{}))
}

type byLen []string

func (b byLen) Len() int           { return len(b) }
func (b byLen) Less(i, j int) bool { return len(b[i]) < len(b[j]) }
func (b byLen) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
