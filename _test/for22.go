package main

import (
	"fmt"
	"strconv"
)

func main() {
	n := 0
	for i := 0; i < 3; i++ {
		i, err := strconv.Atoi("7")
		if err != nil {
			panic(err)
		}
		n += i
	}
	fmt.Println(n)

	m := 0
	for i := 0; i < 3; i++ {
		var i = 2
		m += i
	}
	fmt.Println(m)

	s := ""
	for _, v := range []string{"a", "b"} {
		v, ok := map[string]string{"a": "x", "b": "y"}[v]
		if ok {
			s += v
		}
	}
	fmt.Println(s)
}

// Output:
// 21
// 6
// xy
