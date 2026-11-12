// AOT test program: github.com/google/go-cmp on interpreted types.
//
//aot:lib github.com/google/go-cmp/cmp github.com/google/go-cmp/cmp/cmpopts
package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type Point struct{ X, Y int }

// Version has an Equal method (used by cmp).
type Version struct {
	Major, Minor int
	Label        string
}

func (v Version) Equal(o Version) bool { return v.Major == o.Major && v.Minor == o.Minor }

type Node struct {
	Name     string
	Children []*Node
	Attrs    map[string]any
	Ver      Version
	Weight   float64
	When     time.Time
	internal int
}

func main() {
	if benchMain(benchOp) {
		return
	}
	a := &Node{Name: "root", Children: []*Node{{Name: "a"}, {Name: "b", Weight: 1}},
		Attrs: map[string]any{"k": []int{1, 2}, "p": Point{1, 2}}, Ver: Version{1, 2, "x"}, When: time.Unix(5, 0)}
	b := &Node{Name: "root", Children: []*Node{{Name: "a"}, {Name: "b", Weight: 1}},
		Attrs: map[string]any{"k": []int{1, 2}, "p": Point{1, 2}}, Ver: Version{1, 2, "other label"}, When: time.Unix(5, 0)}
	ign := cmpopts.IgnoreUnexported(Node{})
	fmt.Println(cmp.Equal(a, b, ign))
	b.Children[1].Weight = 1.0000001
	fmt.Println(cmp.Equal(a, b, ign), cmp.Equal(a, b, ign, cmpopts.EquateApprox(0, 0.001)))
	fmt.Println(cmp.Diff(a, b, ign) != "")
	c := []Point{{3, 1}, {1, 2}}
	d := []Point{{1, 2}, {3, 1}}
	byX := cmpopts.SortSlices(func(p, q Point) bool { return p.X < q.X })
	fmt.Println(cmp.Equal(c, d), cmp.Equal(c, d, byX))
	abs := cmp.Comparer(func(x, y int) bool { return math.Abs(float64(x)) == math.Abs(float64(y)) })
	fmt.Println(cmp.Equal(Point{1, -2}, Point{-1, 2}, abs))
	upper := cmp.Transformer("upper", strings.ToUpper)
	fmt.Println(cmp.Equal(map[string]string{"k": "abc"}, map[string]string{"k": "ABC"}, upper))
	fmt.Println(cmp.Equal(map[string]int{"a": 1}, map[string]int{"a": 1, "b": 0}, cmpopts.EquateEmpty()))
	fmt.Println(cmp.Equal([]any{1, "a", nil}, []any{1, "a", nil}), cmp.Equal(Version{1, 1, "a"}, Version{1, 1, "b"}))
	var r reporter
	cmp.Equal(Point{1, 2}, Point{1, 3}, cmp.Reporter(&r))
	sort.Strings(r.diffs)
	fmt.Println(r.diffs)
	func() {
		defer func() { fmt.Println("panic:", recover() != nil) }()
		cmp.Equal(Node{internal: 1}, Node{internal: 2})
	}()
	fmt.Println(cmp.Equal(Node{internal: 1}, Node{internal: 2}, cmp.AllowUnexported(Node{})))
}

type reporter struct {
	path  cmp.Path
	diffs []string
}

func (r *reporter) PushStep(ps cmp.PathStep) { r.path = append(r.path, ps) }
func (r *reporter) Report(rs cmp.Result) {
	if !rs.Equal() {
		vx, vy := r.path.Last().Values()
		r.diffs = append(r.diffs, fmt.Sprintf("%#v: %+v != %+v", r.path, vx, vy))
	}
}
func (r *reporter) PopStep() { r.path = r.path[:len(r.path)-1] }

var benchA = &Node{Name: "root", Children: []*Node{{Name: "a"}, {Name: "b"}}, Attrs: map[string]any{"k": []int{1, 2}}}
var benchB = &Node{Name: "root", Children: []*Node{{Name: "a"}, {Name: "b"}}, Attrs: map[string]any{"k": []int{1, 2}}}

// benchOp: compare two small trees.
func benchOp() {
	if !cmp.Equal(benchA, benchB, cmpopts.IgnoreUnexported(Node{})) {
		panic("bench")
	}
}

func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
