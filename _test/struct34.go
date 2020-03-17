package main

type T struct {
	f func(*T)
}

func f1(t *T) { t.f = f2 }

func f2(t *T) { t.f = f1 }

func main() {
	println("ok")
}

// Output:
// ok
