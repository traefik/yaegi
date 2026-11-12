// Evaluation order: function calls, method calls and receive operations in
// an expression run left to right; arguments are evaluated before the call;
// the function value itself is evaluated before its arguments; receivers of
// method values; and operands of composite literals, returns, and sends.
package main

import "fmt"

var log []string

func trace(s string, v int) int {
	log = append(log, s)
	return v
}

func flush(label string) {
	fmt.Println(label, log)
	log = nil
}

type obj struct{ name string }

func (o *obj) m(a, b int) int { return a + b }

func getObj(name string) *obj {
	log = append(log, "getObj "+name)
	return &obj{name}
}

func getFn(name string) func(int, int) int {
	log = append(log, "getFn "+name)
	return func(a, b int) int { return a * b }
}

func ret() (int, int, int) {
	return trace("r1", 1), trace("r2", 2), trace("r3", 3)
}

func main() {
	getFn("f")(trace("a1", 2), trace("a2", 3))
	flush("func value then args:")
	getObj("o").m(trace("m1", 1), trace("m2", 2))
	flush("receiver then args:")
	_ = trace("x", 1) + trace("y", 2)*trace("z", 3)
	flush("binary:")
	s := []int{1, 2, 3, 4}
	_ = s[trace("idx", 1)] + s[trace("idx2", 2)]
	flush("indexes:")
	_ = s[trace("lo", 1):trace("hi", 3)]
	flush("slice exprs:")
	ch := make(chan int, 3)
	ch <- trace("send", 7)
	ch <- 8
	ch <- 9
	v := <-ch*10 + trace("after recv", 0) + <-ch
	fmt.Println(v)
	flush("recv:")
	fmt.Println(ret())
	flush("return:")
	type pt struct{ x, y, z int }
	p := pt{y: trace("y", 2), x: trace("x", 1), z: trace("z", 3)}
	fmt.Println(p)
	flush("keyed literal (source order):")
	m := map[string]int{}
	m[fmt.Sprint(trace("key", 1))] = trace("val", 2)
	fmt.Println(m)
	flush("map assign:")
	arr := [3]int{2: trace("e2", 2), 0: trace("e0", 0)}
	fmt.Println(arr)
	flush("array literal:")
	fn := func(a, b, c int) {}
	fn(trace("p1", 1), func() int { log = append(log, "inner closure"); return 0 }(), trace("p3", 3))
	flush("nested call args:")
	var fs [2]func(int) int
	fs[0] = func(n int) int { return n + 1 }
	fs[1] = func(n int) int { return n - 1 }
	fmt.Println(fs[trace("fsidx", 0)](trace("fsarg", 10)))
	flush("indexed func call:")
	defer flush("deferred flush:")
	defer fmt.Println("defer args:", trace("d1", 1), trace("d2", 2))
	flush("after defer statement:")
	go func(a int) {}(trace("go arg", 1))
	flush("go statement args evaluated now:")
	n := 1
	n += n * trace("compound", 2)
	fmt.Println(n)
	flush("compound:")
	str := fmt.Sprint(trace("s1", 1)) + fmt.Sprint(trace("s2", 2))
	fmt.Println(str)
	flush("string concat:")
	b := trace("cmp1", 1) < trace("cmp2", 2) == (trace("cmp3", 3) > 0)
	fmt.Println(b)
	flush("comparisons:")
	conv := float64(trace("conv", 3)) / float64(trace("conv2", 2))
	fmt.Println(conv)
	flush("conversions:")
}
