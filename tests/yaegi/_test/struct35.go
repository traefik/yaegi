package main

type T struct {
	f func(*T)
}

func f1(t *T) { t.f = f1 }

func main() {
	t := &T{}
	f1(t)
	println(t.f != nil)
}

// Output:
// true
