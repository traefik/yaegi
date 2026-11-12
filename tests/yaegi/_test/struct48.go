package main

type List struct {
	Next *List
	Num  int
}

func add(l *List, n int) *List {
	if l == nil {
		return &List{Num: n}
	}
	l.Next = add(l.Next, n)
	return l
}

func pr(l *List) {
	if l == nil {
		println("")
		return
	}
	print(l.Num)
	pr(l.Next)
}

func main() {
	a := add(nil, 0)
	pr(a)
	a = add(a, 1)
	pr(a)
	a = add(a, 2)
	pr(a)
}

// Output:
// 0
// 01
// 012
