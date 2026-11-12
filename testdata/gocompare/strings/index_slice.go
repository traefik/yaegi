// String indexing, slicing (2 and 3 index forms on byte slices), concatenation,
// comparison operators and the panics of out-of-range indexes, as in Go.
package main

import "fmt"

func try(name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "panic:", r)
		}
	}()
	f()
}

func main() {
	s := "héllo, 世界"
	fmt.Println(len(s), s[0], s[1], s[2], s[len(s)-1])
	fmt.Println(s[:1], s[1:3], s[3:], s[len(s):], s[:0] == "")
	for i := 0; i < len(s); i++ {
		fmt.Print(s[i], " ")
	}
	fmt.Println()
	fmt.Printf("%q %q %q\n", s[1:2], s[9:11], s[2:])
	a, b := "abc", "abd"
	fmt.Println(a < b, a <= b, a > b, a >= b, a == b, a != b)
	fmt.Println("" < "a", "a" < "aa", "Z" < "a", "\xff" > "\x7f", "é" > "z")
	t := a
	t += b
	t += "!"
	fmt.Println(t, len(t), a+b+a, a+"", ""+"")
	parts := []string{}
	for i := 0; i < 5; i++ {
		parts = append(parts, fmt.Sprint(i))
	}
	acc := ""
	for _, p := range parts {
		acc = acc + p + "-"
	}
	fmt.Println(acc)
	const c = "const" + "ant"
	fmt.Println(c, len(c), c[2:5])
	var bs = []byte("abcdef")
	x := bs[1:3:4]
	fmt.Println(string(x), len(x), cap(x))
	x = append(x, 'Z')
	fmt.Println(string(bs))
	x = append(x, 'Y')
	fmt.Println(string(bs), string(x))
	i, j := 3, 2
	try("rev", func() { fmt.Println(s[i:j]) })
	k := 100
	try("idx", func() { fmt.Println(s[k]) })
	try("hi", func() { fmt.Println(s[:k]) })
	n := -1
	try("neg", func() { fmt.Println(s[n+0:]) })
	try("emptyidx", func() { e := ""; fmt.Println(e[0]) })
	var sp *string
	try("nilptr", func() { fmt.Println(*sp) })
	m := map[string]int{"a": 1, "ab": 2, "abc": 3}
	fmt.Println(m)
	key := "a"
	key += "b"
	fmt.Println(m[key], m[key+"c"], m[key[:1]])
	arr := [3]string{"x", "yy", "zzz"}
	fmt.Println(arr, arr[1][1], len(arr[2]))
	type S string
	var v S = "typed"
	fmt.Println(v+"!", v[1:3], len(v), v < "u", string(v) == "typed")
	fmt.Printf("%T %T\n", v[1:3], v[0])
	sw := func(s string) string {
		switch s {
		case "a", "b":
			return "ab"
		case "":
			return "empty"
		}
		return "other"
	}
	fmt.Println(sw("a"), sw("b"), sw(""), sw("c"))
	byteAt := s[7]
	byteAt++
	fmt.Println(byteAt, s[7])
	fmt.Println(string(s[7]), string(rune(s[1])))
}
