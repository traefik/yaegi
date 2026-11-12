// In a select, the channel of a receive case is evaluated when the select
// starts; the left side of "case lhs = <-ch" is evaluated only when the case
// is chosen, after the receive (spec, "Select statements"; GOROOT test
// chan/select5.go). A select of a single case is built by go/ssa as a plain
// receive: before x/tools v0.51.0 it evaluated the left side first.
package main

import "fmt"

var trace []string

func fc(c chan int, s string) chan int { trace = append(trace, s); return c }
func fp(p *int, s string) *int         { trace = append(trace, s); return p }
func fi(i int, s string) int           { trace = append(trace, s); return i }
func fb(p *bool, s string) *bool       { trace = append(trace, s); return p }

func main() {
	c := make(chan int, 1)
	var x int
	var ok bool
	m := map[int]int{}
	a := []int{0, 0}

	c <- 1
	select {
	case *fp(&x, "lhs") = <-fc(c, "chan"):
	}
	c <- 2
	select {
	case m[fi(3, "key")] = <-fc(c, "chan2"):
	}
	c <- 3
	select {
	case a[fi(1, "idx")], *fb(&ok, "ok") = <-fc(c, "chan3"):
	}
	// two cases: go/ssa builds a real select
	c <- 4
	select {
	case *fp(&x, "lhs4") = <-fc(c, "chan4"):
	case <-fc(nil, "nil4"):
	}
	fmt.Println(x, m[3], a[1], ok)
	fmt.Println(trace)
}
