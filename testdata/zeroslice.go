package main

import (
	"fmt"
	"reflect"
	"strings"
)

type T struct{}

type E [0]int

const maxInt = int(^uint(0) >> 1)

func main() {
	s := make([]T, maxInt)
	fmt.Println(len(s), cap(s), s[maxInt-1], s[3:10:20], len(s[5:]), cap(s[:7:9]))
	p := &s[100]
	*p = T{}
	fmt.Println(*p == T{})
	shouldPanic("len out of range", func() { s = append(s, T{}) })
	var oneElem = make([]T, 1)
	shouldPanic("len out of range", func() { s = append(s, oneElem...) })
	shouldPanic("index out of range", func() { _ = s[len(s)-maxInt-1] })
	z := make([]T, 1<<21)
	neg := -1
	shouldPanic("slice bounds out of range [-1:]", func() { _ = z[neg:] })
	shouldPanic("slice bounds out of range [:-1]", func() { _ = z[:neg] })
	shouldPanic("slice bounds out of range [::-1]", func() { _ = z[0:0:neg] })
	shouldPanic("slice bounds out of range [:2097153]", func() { _ = z[:len(z)+1] })

	big := make([]E, 1<<40, 1<<41)
	big = append(big, E{}, E{})
	fmt.Println(len(big), cap(big) >= 1<<40+2, copy(big, []E{{}, {}}))
	n := 0
	for i := range big[:1000] {
		n += i
	}
	for _, e := range big[:5] {
		fmt.Print(len(e))
	}
	fmt.Println(n)
	small := append([]E{{}}, big[:3]...)
	fmt.Println(len(small), reflect.ValueOf(big[:4]).Len())
	fmt.Println(big[:3], len(append(small, big...)))
}

func shouldPanic(str string, f func()) {
	defer func() {
		err := recover()
		if err == nil {
			panic("did not panic")
		}
		s := err.(error).Error()
		if !strings.Contains(s, str) {
			panic("got panic " + s + ", want " + str)
		}
		fmt.Println("ok", str)
	}()
	f()
}
