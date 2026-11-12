// Short-circuit evaluation of && and || with side effects, the order of
// operand evaluation in expressions and calls, and the order of the parts of
// an assignment (index expressions and pointer indirections are evaluated
// first, then the assignments are done left to right).
package main

import "fmt"

var log []string

func t(name string) bool { log = append(log, name+"T"); return true }
func f(name string) bool { log = append(log, name+"F"); return false }
func v(name string, n int) int {
	log = append(log, name)
	return n
}

func flush(label string) {
	fmt.Println(label, log)
	log = nil
}

func three(a, b, c int) int { return a*100 + b*10 + c }

func main() {
	_ = t("a") && f("b") && t("c")
	flush("and:")
	_ = f("a") && t("b")
	flush("and short:")
	_ = t("a") || f("b")
	flush("or short:")
	_ = f("a") || f("b") || t("c") || t("d")
	flush("or:")
	_ = (t("a") || t("b")) && (f("c") || t("d"))
	flush("mixed:")
	_ = !t("a") || (f("b") && t("c")) || t("d")
	flush("mixed2:")
	if x := f("cond"); x && t("never") {
		fmt.Println("never")
	}
	flush("if:")
	r := three(v("x", 1), v("y", 2), v("z", 3))
	fmt.Println(r)
	flush("args:")
	s := v("a", 1) + v("b", 2)*v("c", 3) - v("d", 4)
	fmt.Println(s)
	flush("arith:")
	arr := []int{v("e0", 0), v("e1", 1), v("e2", 2)}
	fmt.Println(arr)
	flush("composite:")
	mp := map[int]int{v("k1", 1): v("v1", 10), v("k2", 2): v("v2", 20)}
	fmt.Println(mp)
	flush("map lit:")

	// Assignment order: indexes evaluated first, then assigned left to right.
	a := []int{0, 0, 0}
	i := 0
	i, a[i] = 1, 9 // a[0] is set: index evaluated before i changes
	fmt.Println(i, a)
	a[i], i = 5, 2 // a[1] = 5
	fmt.Println(i, a)
	x, y := 1, 2
	x, y = y, x
	fmt.Println(x, y)
	b := []int{1, 2, 3}
	j := 0
	j, b[j], b[j+1] = 2, b[j+1], b[j]
	fmt.Println(j, b)
	k := 1
	b[k], k = b[k]*100, b[k]
	fmt.Println(k, b)

	type pair struct{ l, r int }
	p := &pair{1, 2}
	q := p
	p, p.l = &pair{7, 8}, 50 // p.l uses the old p
	fmt.Println(*p, *q)

	c := []int{v("c0", 0), 1, 2}
	c[v("idx", 1)] = v("rhs", 7)
	flush("index assign:")
	fmt.Println(c)
	c[v("i1", 0)], c[v("i2", 2)] = v("r1", 10), v("r2", 20)
	flush("tuple assign:")
	fmt.Println(c)
	c[v("op idx", 1)] += v("op rhs", 5)
	flush("op assign:")
	fmt.Println(c)

	// Calls in an expression are evaluated left to right.
	n := 0
	inc := func() int { n++; return n }
	fmt.Println(inc(), inc()*10, inc()-inc())
	z := []int{inc(), inc(), inc()}
	fmt.Println(z)

	// Map element assignment with a panicking index after the map update?
	m := map[string]int{}
	func() {
		defer func() { recover() }()
		var nilSlice []int
		m["set"], nilSlice[5] = 1, 2
	}()
	fmt.Println("map after partial tuple assign:", m)

	// Bool expressions used as values.
	vals := []bool{t("p") && f("q"), f("r") || t("s")}
	fmt.Println(vals)
	flush("bool values:")
	cnt := 0
	bump := func() bool { cnt++; return cnt < 3 }
	for bump() && bump() {
	}
	fmt.Println("cnt", cnt)
}
