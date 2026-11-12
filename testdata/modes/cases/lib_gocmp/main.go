// github.com/google/go-cmp: Equal and Diff on interpreted types, with
// options (AllowUnexported, Comparer, Transformer, cmpopts) and an Equal
// method.
package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type Point struct{ X, Y int }

type Config struct {
	Name   string
	Ports  []int
	Labels map[string]string
	Origin *Point
	Tags   []string
	secret string
}

type Version struct{ Major, Minor int }

func (v Version) Equal(o Version) bool { return v.Major == o.Major }

type Release struct {
	V    Version
	Note string
}

func main() {
	a := Config{Name: "a", Ports: []int{80, 443}, Labels: map[string]string{"k": "v"}, Origin: &Point{1, 2}, Tags: []string{"y", "x"}, secret: "s1"}
	b := Config{Name: "a", Ports: []int{80, 8443}, Labels: map[string]string{"k": "w", "n": "new"}, Origin: &Point{1, 3}, Tags: []string{"x", "y"}, secret: "s2"}

	fmt.Println(cmp.Equal(a, a, cmp.AllowUnexported(Config{})))
	d := cmp.Diff(a, b, cmpopts.IgnoreUnexported(Config{}))
	// The diff output uses random spaces (non-breaking or not) on purpose.
	fmt.Println(strings.Join(strings.Fields(strings.ReplaceAll(d, " ", " ")), " "))
	fmt.Println(cmp.Equal(a, b, cmpopts.IgnoreUnexported(Config{}), cmpopts.IgnoreFields(Config{}, "Ports", "Labels", "Origin"), cmpopts.SortSlices(func(x, y string) bool { return x < y })))

	func() {
		defer func() { r := recover(); fmt.Println("panic:", r != nil) }()
		cmp.Equal(a, b)
	}()

	fmt.Println(cmp.Equal(Release{Version{1, 2}, "x"}, Release{Version{1, 9}, "x"}))
	byLen := cmp.Comparer(func(x, y string) bool { return len(x) == len(y) })
	fmt.Println(cmp.Equal([]string{"aa", "b"}, []string{"zz", "c"}, byLen))
	sorted := cmp.Transformer("sort", func(in []int) []int { out := append([]int(nil), in...); sort.Ints(out); return out })
	fmt.Println(cmp.Equal([]int{3, 1, 2}, []int{1, 2, 3}, sorted))
	fmt.Println(cmp.Equal(map[Point]string{{1, 1}: "a"}, map[Point]string{{1, 1}: "a"}), cmp.Equal(&Point{1, 2}, &Point{1, 2}))
	fmt.Println(cmp.Equal(1.0, 1.05, cmpopts.EquateApprox(0.1, 0)), cmp.Equal([]int{}, []int(nil), cmpopts.EquateEmpty()))
}
