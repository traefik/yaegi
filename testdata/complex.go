package main

import "fmt"

func f(a, b float64) any { return complex(a, b) }

func main() {
	a := complex(3, 2)
	fmt.Println(a, real(a), imag(a))
	var c complex128 = 1
	c += 1i
	c *= 2
	fmt.Printf("%T %v %v\n", c, c, c/2)
	fmt.Println(-c, c == 2+2i, c != 2+2i)
	var d complex64 = complex64(c) / 3
	fmt.Printf("%T %v %v\n", d, d, real(d))
	fmt.Println(f(1.5, -1).(complex128))
	m := map[complex128]int{1i: 1}
	m[1i]++
	fmt.Println(m[1i], m)
	var z complex128
	fmt.Println(z, z == 0)
}
