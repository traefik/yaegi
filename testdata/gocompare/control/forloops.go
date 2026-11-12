// for loops: three clauses, while style, infinite with break, continue,
// several variables in the clauses, loops that never run, modifying the loop
// variable in the body, and the value of the variable after the loop.
package main

import "fmt"

func main() {
	sum := 0
	for i := 0; i < 10; i++ {
		sum += i
	}
	fmt.Println("sum", sum)
	for i, j := 0, 10; i < j; i, j = i+1, j-2 {
		fmt.Print(i, ":", j, " ")
	}
	fmt.Println()
	n := 27
	steps := 0
	for n != 1 {
		if n%2 == 0 {
			n /= 2
		} else {
			n = 3*n + 1
		}
		steps++
	}
	fmt.Println("collatz steps", steps)
	k := 0
	for {
		k++
		if k%3 == 0 {
			continue
		}
		if k > 10 {
			break
		}
		fmt.Print(k, " ")
	}
	fmt.Println("k", k)
	for i := 10; i < 5; i++ {
		fmt.Println("never")
	}
	var i int
	for i = 0; i < 5; i++ {
	}
	fmt.Println("i after", i)
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			i++ // skip
		}
		fmt.Print(i, " ")
	}
	fmt.Println()
	cnt := 0
	for cnt < 3 {
		cnt++
	}
	fmt.Println("cnt", cnt)
	x := 0
	for x < 4 {
		x += 2
	}
	fmt.Println("x", x)
	var u8 uint8
	loops := 0
	for u8 = 250; u8 >= 250; u8++ {
		loops++
	}
	fmt.Println("uint8 wrap loops", loops, u8)
	fib := []int{1, 1}
	for len(fib) < 15 {
		fib = append(fib, fib[len(fib)-1]+fib[len(fib)-2])
	}
	fmt.Println(fib)
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if j > i {
				break
			}
			fmt.Print(i, j, ";")
		}
	}
	fmt.Println()
	s := []int{1, 2, 3}
	for i := 0; i < len(s); i++ {
		if s[i] == 2 {
			s = append(s, 10)
		}
		if len(s) > 6 {
			break
		}
	}
	fmt.Println(s)
	f := 1.0
	for f < 100 {
		f *= 1.5
	}
	fmt.Println(f)
	var down int64
	for down = 5; down > -5; down -= 3 {
		fmt.Print(down, " ")
	}
	fmt.Println()
}
