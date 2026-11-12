// Multiple return values: passing f() directly as the arguments of g(f()),
// returns of named results, blank identifiers, swaps, returning functions
// that return several values, and comma-ok forms.
package main

import (
	"errors"
	"fmt"
	"strconv"
)

func divmod(a, b int) (int, int) { return a / b, a % b }

func add(a, b int) int { return a + b }

func three() (int, string, error) { return 1, "two", errors.New("three") }

func named(n int) (q, r int, err error) {
	if n < 0 {
		err = fmt.Errorf("negative: %d", n)
		return
	}
	q, r = divmod(n, 7)
	return
}

func shadowed() (x int, err error) {
	x = 1
	if x, err := strconv.Atoi("42"); err == nil {
		_ = x
	}
	return
}

func swap(a, b string) (string, string) { return b, a }

func pairMaker() func() (int, int) {
	i := 0
	return func() (int, int) { i++; return i, i * i }
}

func variadicSink(xs ...int) int { return len(xs) }

func returnsCall() (int, int) { return divmod(17, 5) }

func overrideNamed() (a, b int) {
	a, b = 1, 2
	return b, a
}

func main() {
	q, r := divmod(17, 5)
	fmt.Println(q, r)
	fmt.Println(divmod(-17, 5))
	fmt.Println(add(divmod(20, 3)))
	fmt.Println(variadicSink(divmod(9, 2)))
	a, b, c := three()
	fmt.Println(a, b, c)
	_, s, _ := three()
	fmt.Println(s)
	fmt.Println(three())
	for _, n := range []int{50, -1, 6} {
		fmt.Println(named(n))
	}
	fmt.Println(shadowed())
	x, y := "a", "b"
	x, y = swap(x, y)
	fmt.Println(x, y)
	x, y = y, x
	fmt.Println(x, y)
	pm := pairMaker()
	pm()
	fmt.Println(pm())
	fmt.Println(returnsCall())
	fmt.Println(overrideNamed())
	m := map[string]int{"a": 1}
	v, ok := m["a"]
	fmt.Println(v, ok)
	v, ok = m["z"]
	fmt.Println(v, ok)
	var i interface{} = 3
	n, ok := i.(int)
	fmt.Println(n, ok)
	str, ok := i.(string)
	fmt.Printf("%q %v\n", str, ok)
	ch := make(chan int, 1)
	ch <- 5
	close(ch)
	v1, ok1 := <-ch
	v2, ok2 := <-ch
	fmt.Println(v1, ok1, v2, ok2)
	var e1, e2 error
	_, e1 = strconv.Atoi("x")
	_, e2 = strconv.ParseBool("maybe")
	fmt.Println(e1)
	fmt.Println(e2)
	fs := []func() (int, error){
		func() (int, error) { return 1, nil },
		func() (int, error) { return 0, errors.New("bad") },
	}
	for _, f := range fs {
		if v, err := f(); err != nil {
			fmt.Println("err", err)
		} else {
			fmt.Println("ok", v)
		}
	}
	arr := [3]int{}
	arr[0], arr[1], arr[2] = divmodTriple()
	fmt.Println(arr)
	type st struct{ a, b int }
	var sv st
	sv.a, sv.b = divmod(100, 7)
	fmt.Println(sv)
	mp := map[int]int{}
	mp[1], mp[2] = divmod(7, 2)
	fmt.Println(mp)
}

func divmodTriple() (int, int, int) {
	q, r := divmod(29, 4)
	return q, r, q * r
}
