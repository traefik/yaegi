// Loads of struct and array values without a copy (vm/loads.go): dead
// loads (the range expression of a loop over an array field) and shallow
// loads (a value that only goes to a copying store, a literal or an
// append). Each case checks that later writes to the memory do not reach
// the copies, that the hazards keep their copies (a swap, a read after a
// write, a map entry, a call in between), and that a dead load of a nil
// pointer still panics.
package main

import "fmt"

type Op struct {
	Kind int
	Arg  string
}

type Rule struct {
	ID  int
	Ops [3]Op
	Tag [2]byte
}

type Match struct {
	RuleID int
	Rule   Rule
}

func deadRange(r *Rule) int {
	n := 0
	for i := range r.Ops { // the array is loaded for nothing
		n += i
	}
	for range r.Tag {
		n++
	}
	return n
}

func copyThrough(rules []Rule) {
	// r := rules[0] is a load consumed by the store into the local r,
	// whose address goes to a call: the local keeps a copy.
	r := rules[0]
	rules[0].ID = 99
	rules[0].Ops[1].Arg = "changed"
	fmt.Println(r.ID, r.Ops[1].Arg, rules[0].ID, rules[0].Ops[1].Arg, idOf(&r))
}

func idOf(r *Rule) int { return r.ID }

func literal(rules []Rule) {
	// The literal copies the loaded struct.
	m := Match{RuleID: 1, Rule: rules[1]}
	rules[1].Ops[0].Kind = 7
	fmt.Println(m.Rule.Ops[0].Kind, rules[1].Ops[0].Kind)
	var ms []Match
	ms = append(ms, Match{Rule: rules[2]})
	rules[2].ID = -1
	fmt.Println(ms[0].Rule.ID, rules[2].ID)
}

func appendCopies(rules []Rule) {
	var out []Rule
	out = append(out, rules[0])
	out = append(out, rules[1], rules[2])
	rules[0].ID, rules[1].ID, rules[2].ID = 1000, 1001, 1002
	fmt.Println(out[0].ID, out[1].ID, out[2].ID)
	// Appending into spare capacity still copies.
	buf := make([]Rule, 0, 4)
	buf = append(buf, rules[0])
	rules[0].Ops[2].Arg = "spare"
	fmt.Println(buf[0].Ops[2].Arg)
}

func swap(a []Rule) {
	a[0], a[1] = a[1], a[0]
	fmt.Println(a[0].ID, a[1].ID, a[0].Ops[0].Arg, a[1].Ops[0].Arg)
	x := [2]Op{{1, "a"}, {2, "b"}}
	x[0], x[1] = x[1], x[0]
	fmt.Println(x)
}

func readAfterWrite(p *Rule) {
	t := *p // used after the write below: must be a copy
	p.ID = 5
	p.Ops[0].Arg = "after"
	fmt.Println(t.ID, t.Ops[0].Arg, p.ID, p.Ops[0].Arg)
}

func viaMap(rules []Rule) {
	m := map[int]Rule{}
	m[0] = rules[0]
	rules[0].Ops[0].Kind = 42
	fmt.Println(m[0].Ops[0].Kind, rules[0].Ops[0].Kind)
}

func viaCall(rules []Rule) {
	var dst Rule
	v := rules[0]
	mutate(rules)
	dst = v
	fmt.Println(dst.ID, rules[0].ID)
}

func mutate(rules []Rule) { rules[0].ID = 77 }

func storeTwice(rules []Rule) {
	var a, b Rule
	v := rules[1]
	a = v
	b = v
	rules[1].ID = 3
	a.Ops[0].Arg = "a"
	fmt.Println(a.ID, b.ID, a.Ops[0].Arg, b.Ops[0].Arg, rules[1].ID)
}

func selfAssign(rules []Rule) {
	rules[0] = rules[0]
	rules[1] = rules[0]
	rules[0].ID = 8
	fmt.Println(rules[0].ID, rules[1].ID)
}

func nested() {
	type Inner struct{ A [2]int }
	type Outer struct {
		I Inner
		N int
	}
	o := Outer{I: Inner{A: [2]int{1, 2}}, N: 3}
	p := &o
	q := *p
	p.I.A[0] = 9
	q2 := p.I
	p.I.A[1] = 8
	fmt.Println(q.I.A, q2.A, p.I.A)
}

func nilRange() {
	defer func() { fmt.Println("recovered:", recover() != nil) }()
	var r *Rule
	for i := range r.Ops {
		fmt.Println("no", i)
	}
}

func main() {
	rules := []Rule{
		{ID: 1, Ops: [3]Op{{0, "x"}, {1, "y"}, {2, "z"}}, Tag: [2]byte{'a', 'b'}},
		{ID: 2, Ops: [3]Op{{3, "u"}, {4, "v"}, {5, "w"}}},
		{ID: 3},
	}
	fmt.Println(deadRange(&rules[0]))
	copyThrough(rules)
	literal(rules)
	appendCopies(rules)
	swap(rules)
	readAfterWrite(&rules[2])
	viaMap(rules)
	viaCall(rules)
	storeTwice(rules)
	selfAssign(rules)
	nested()
	nilRange()
}
