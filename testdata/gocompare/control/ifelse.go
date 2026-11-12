// if/else chains with init statements: scope of the init variables, else-if
// chains, shadowing in the init, and calls with side effects in the init.
package main

import (
	"errors"
	"fmt"
	"strconv"
)

var calls int

func next() int { calls++; return calls }

func classify(n int) string {
	if n < 0 {
		return "neg"
	} else if n == 0 {
		return "zero"
	} else if n%2 == 0 {
		return "even"
	} else {
		return "odd"
	}
}

func parse(s string) (int, error) {
	if v, err := strconv.Atoi(s); err != nil {
		return 0, fmt.Errorf("parse %q: %w", s, err)
	} else if v > 100 {
		return v, errors.New("too big")
	} else {
		return v * 2, nil
	}
}

func main() {
	for _, n := range []int{-5, 0, 2, 7, 100, -1} {
		fmt.Print(classify(n), " ")
	}
	fmt.Println()
	for _, s := range []string{"10", "x", "200", "-3", ""} {
		v, err := parse(s)
		fmt.Println(s, v, err)
	}
	x := 1
	if x := x + 10; x > 5 {
		fmt.Println("inner x", x)
		if x := x * 2; x > 20 {
			fmt.Println("inner inner x", x)
		} else {
			fmt.Println("else inner inner x", x)
		}
		x++
		fmt.Println("inner x after", x)
	}
	fmt.Println("outer x", x)
	if a, b := next(), next(); a < b {
		fmt.Println("a<b", a, b)
	}
	if v := next(); v == 99 {
		fmt.Println("never")
	} else if w := next(); v+w == 7 {
		fmt.Println("v+w", v, w)
	} else {
		fmt.Println("else", v, w)
	}
	fmt.Println("calls", calls)
	var p *int
	if p != nil && *p > 0 {
		fmt.Println("never")
	} else {
		fmt.Println("nil guarded")
	}
	m := map[string]int{"a": 1}
	if v, ok := m["a"]; ok {
		fmt.Println("found", v)
	}
	if v, ok := m["b"]; !ok {
		fmt.Println("missing", v, ok)
	}
	var i interface{} = 3.5
	if f, ok := i.(float64); ok && f > 3 {
		fmt.Println("float", f)
	}
	if s, ok := i.(string); !ok {
		fmt.Printf("not string %q\n", s)
	}
	b := true
	if !b {
		fmt.Println("no")
	} else if b = !b; b {
		fmt.Println("no2")
	} else {
		fmt.Println("b now", b)
	}
}
