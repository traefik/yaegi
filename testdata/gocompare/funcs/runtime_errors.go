// Runtime errors recovered with their exact messages: nil map write, nil
// pointer dereference, integer divide by zero, index and slice out of range,
// failed type assertions, close of nil and closed channels, negative make
// sizes, and the runtime.Error interface on each recovered value.
package main

import (
	"errors"
	"fmt"
	"runtime"
)

type T struct{ x int }

type I interface{ M() }

func try(name string, f func()) {
	defer func() {
		r := recover()
		re, isRT := r.(runtime.Error)
		var err error
		isErr := false
		if e, ok := r.(error); ok {
			err, isErr = e, true
		}
		fmt.Printf("%s: runtime.Error=%v error=%v: %v\n", name, isRT, isErr, r)
		if isRT {
			fmt.Println("  RuntimeError() call ok, Error():", re.Error())
		}
		var target runtime.Error
		if err != nil {
			fmt.Println("  errors.As runtime.Error:", errors.As(err, &target))
		}
	}()
	f()
}

var sink interface{}

func main() {
	try("nil map write", func() {
		var m map[string]int
		m["a"] = 1
	})
	try("nil map read is ok", func() {
		var m map[string]int
		sink = m["a"]
		fmt.Println("read", sink)
	})
	try("nil pointer", func() {
		var p *T
		sink = p.x
	})
	try("nil pointer write", func() {
		var p *T
		p.x = 3
	})
	try("nil slice ptr", func() {
		var p *[3]int
		sink = p[1]
	})
	try("divide by zero", func() {
		a, b := 10, 0
		sink = a / b
	})
	try("modulo by zero", func() {
		a, b := int64(10), int64(0)
		sink = a % b
	})
	try("uint8 divide by zero", func() {
		a, b := uint8(10), uint8(0)
		sink = a / b
	})
	try("float divide by zero is ok", func() {
		a, b := 1.0, 0.0
		fmt.Println(a/b, -a/b)
	})
	try("index", func() {
		s := []int{1, 2, 3}
		i := 5
		sink = s[i]
	})
	try("negative index", func() {
		s := []int{1, 2, 3}
		i := -1
		sink = s[i]
	})
	try("array index", func() {
		var a [4]int
		i := 4
		a[i] = 1
	})
	try("string index", func() {
		s := "abc"
		i := 3
		sink = s[i]
	})
	try("slice high", func() {
		s := make([]int, 2, 4)
		j := 5
		sink = s[:j]
	})
	try("slice low>high", func() {
		s := []int{1, 2, 3}
		i, j := 2, 1
		sink = s[i:j]
	})
	try("slice 3-index", func() {
		s := make([]int, 2, 4)
		k := 10
		sink = s[0:1:k]
	})
	try("slice beyond len ok within cap", func() {
		s := make([]int, 2, 4)
		j := 4
		fmt.Println(len(s[:j]))
	})
	try("string slice", func() {
		s := "hello"
		j := 10
		sink = s[2:j]
	})
	try("type assertion", func() {
		var i interface{} = "str"
		sink = i.(int)
	})
	try("type assertion nil", func() {
		var i interface{}
		sink = i.(string)
	})
	try("type assertion iface", func() {
		var i interface{} = 3
		sink = i.(I)
	})
	try("type assertion custom", func() {
		var i interface{} = T{1}
		sink = i.(*T)
	})
	try("nil interface method", func() {
		var i I
		i.M()
	})
	try("close nil chan", func() {
		var c chan int
		close(c)
	})
	try("close closed chan", func() {
		c := make(chan int)
		close(c)
		close(c)
	})
	try("send on closed", func() {
		c := make(chan int, 1)
		close(c)
		c <- 1
	})
	try("make negative len", func() {
		n := -1
		sink = make([]int, n)
	})
	try("make len>cap", func() {
		n, c := 5, 2
		sink = make([]int, n, c)
	})
	try("nil func call", func() {
		var f func()
		f()
	})
	try("array conversion", func() {
		s := []int{1, 2}
		sink = [4]int(s)
	})
	try("custom error panic is not runtime", func() {
		panic(errors.New("plain"))
	})
}
