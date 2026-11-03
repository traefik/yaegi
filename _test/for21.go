package main

import "fmt"

func main() {
	m := map[string]func() string{}
	for _, name := range []string{"Host", "PathPrefix"} {
		name := name
		m[name] = func() string { return name }
	}
	fmt.Println(len(m), m["Host"](), m["PathPrefix"]())

	var fs []func() int
	for i := 0; i < 3; i++ {
		i := i
		fs = append(fs, func() int { return i })
	}
	fmt.Println(fs[0](), fs[1](), fs[2]())

	n := 0
	for i := 0; i < 3; i++ {
		i := 10
		n += i
	}
	fmt.Println(n)

	for k, v := range []string{"a", "b"} {
		k, v := v, k
		fmt.Println(k, v)
	}
}

// Output:
// 2 Host PathPrefix
// 0 1 2
// 30
// a 0
// b 1
