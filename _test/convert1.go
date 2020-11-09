package main

import "strconv"

type atoidef func(s string) (int, error)

func main() {
	stdatoi := atoidef(strconv.Atoi)
	n, err := stdatoi("7")
	if err != nil {
		panic(err)
	}
	println(n)
}

// Output:
// 7
