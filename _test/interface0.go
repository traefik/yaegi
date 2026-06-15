package main

type sample struct {
	count int
}

func run(inf any, name string) {
	x := inf.(sample)
	println(x.count, name)
}

func main() {
	a := sample{2}
	println(a.count)
	run(a, "truc")
}

// Output:
// 2
// 2 truc
