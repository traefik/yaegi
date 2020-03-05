package main

type T0 struct {
	name string
}

type lookupFunc func(s string) T0

type T1 struct {
	name string
	info lookupFunc
}

func (t T0) F1() bool { println("in F1"); return true }

type T2 struct {
	t1 T1
}

func (t2 *T2) f() {
	info := t2.t1.info("foo")
	println(info.F1())
}

var t0 = T0{"t0"}

func main() {
	t := &T2{T1{
		"bar", func(s string) T0 { return t0 },
	}}

	println("hello")
	println(t.t1.info("foo").F1())
}

// Output:
// hello
// in F1
// true
