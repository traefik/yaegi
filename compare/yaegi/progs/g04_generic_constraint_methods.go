package main

import (
	"fmt"
	"strconv"
)

type Stringer interface{ String() string }

type Celsius float64

func (c Celsius) String() string { return strconv.FormatFloat(float64(c), 'f', 1, 64) + "C" }

type Named interface {
	comparable
	Name() string
}

type Svc struct{ N string }

func (s Svc) Name() string { return s.N }

func Join[T Stringer](xs []T) string {
	out := ""
	for _, x := range xs {
		out += x.String() + ";"
	}
	return out
}

func Index[T Named](xs []T, name string) int {
	for i, x := range xs {
		if x.Name() == name {
			return i
		}
	}
	return -1
}

type List[T any] struct {
	head *node[T]
}

type node[T any] struct {
	v    T
	next *node[T]
}

func (l *List[T]) Push(v T) { l.head = &node[T]{v, l.head} }
func (l *List[T]) All() []T {
	var out []T
	for n := l.head; n != nil; n = n.next {
		out = append(out, n.v)
	}
	return out
}

func main() {
	fmt.Println(Join([]Celsius{1, 2.5}))
	fmt.Println(Index([]Svc{{"a"}, {"b"}}, "b"))
	var l List[Svc]
	l.Push(Svc{"x"})
	l.Push(Svc{"y"})
	fmt.Println(l.All())
}
