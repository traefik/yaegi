package main

import "fmt"

func main() {
	s := ""
	for i := 0; ; {
		i++
		if i > 8 {
			break
		}
		if i%2 == 0 {
			continue
		}
		s += fmt.Sprint(i)
	}
	fmt.Println(s)
}

// Output:
// 1357
