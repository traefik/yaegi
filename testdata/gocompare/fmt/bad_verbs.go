// fmt error output: wrong verbs (%!d(string=...)), missing and extra
// arguments, bad argument indexes, %[n] reordering, nil values with
// every verb, unknown verbs, a lone % at the end, and bad * arguments.
package main

import (
	"errors"
	"fmt"
)

type T struct {
	A int
	B string
}

func main() {
	fmt.Printf("%d|%s|%t|%x|%c|%U|%e|%q\n", "str", 42, 1, true, "s", 2.5, "x", 3.5)
	fmt.Printf("%d %d\n", 1)
	fmt.Printf("%d\n", 1, 2, "three")
	fmt.Printf("no verbs\n", 1)
	fmt.Printf("%z %! %ä\n", 1, 2, 3)
	fmt.Printf("end%")
	fmt.Println()
	fmt.Printf("%")
	fmt.Println()
	fmt.Printf("%-")
	fmt.Println()
	fmt.Printf("%[2]d %[1]d\n", 1, 2)
	fmt.Printf("%[3]*.[2]*[1]f|\n", 12.0, 2, 6)
	fmt.Printf("%d %d %#[1]x %#x\n", 16, 17)
	fmt.Printf("%[5]d\n", 1, 2)
	fmt.Printf("%[0]d %[-1]d %[x]d %[2d %[]d\n", 1, 2)
	fmt.Printf("%[2]d %d %[1]d %d\n", 1, 2)
	fmt.Printf("%[1]d %[1]q %[1]x %[1]v\n", 'A')
	fmt.Printf("%.[2]d|%[2]*[1]d|\n", 7, 4)
	fmt.Printf("%[2]*d\n", 1)
	fmt.Printf("%*[2]d\n", 3, 4)
	fmt.Printf("%!d(bad)\n")
	var np *T
	var ne error
	var ni any
	var ns []int
	var nm map[string]int
	nils := []any{nil, np, ne, ni, ns, nm}
	for _, v := range nils {
		fmt.Printf("%v|%+v|%#v|%T|%d|%s|%q|%x|%t|%f|\n", v, v, v, v, v, v, v, v, v, v)
	}
	fmt.Println(nil, np, ne, ni, ns, nm)
	fmt.Print(nil, np, "\n")
	fmt.Printf("%s %v %d\n", errors.New("boom"), errors.New("boom"), errors.New("boom"))
	fmt.Printf("%d|%s|%x|%t\n", T{1, "b"}, T{1, "b"}, T{10, "b"}, T{1, "b"})
	fmt.Printf("%s %d\n", []any{1, "a"}, []any{1, "a"})
	fmt.Printf("%s\n", map[string]int{"a": 1})
	fmt.Printf("%t %t\n", "true", 1)
	fmt.Printf("%c %c\n", "c", 1.5)
	fmt.Printf("%.2d %.2s %.2x %.2q\n", 5, "abc", "abc", "abc")
	fmt.Printf("%#s %#d %+s\n", "a", 1, "b")
	s := fmt.Sprintf("%d", "x")
	fmt.Println(s, len(s))
	fmt.Println(fmt.Sprintf("%v %v", 1))
	fmt.Println(fmt.Sprintf("%v", 1, 2))
	fmt.Println(fmt.Sprintf(""))
	fmt.Println(fmt.Sprintf("%w", errors.New("not errorf")))
	fmt.Println(fmt.Sprintf("%v %w", 1, 2))
	fmt.Printf("%-+# 0d|\n", 5)
	fmt.Printf("%08s|%-08s|\n", "pad", "pad")
	fmt.Printf("%v\n", []any{nil, (*T)(nil), error(nil)})
	fmt.Printf("%9.9999999999d\n", 1)
}
