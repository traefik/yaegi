package main

// A value nested in interfaces a bit deeper than the bound of the native
// copy (maxBoxDepth, 100000), printed with %v in default mode. fmt goes to
// its AOT copy, which read the part past the bound through the VM cells
// and converted the rest again at each level: the time grew with the square
// of the levels past the bound (10000 levels: about a minute). The part
// past the bound is now converted once.

import (
	"fmt"
	"strings"
)

func main() {
	var v any = 0
	for i := 0; i < 110_000; i++ {
		v = []any{v}
	}
	s := fmt.Sprint(v)
	fmt.Println(len(s), strings.Count(s, "["), strings.Count(s, "]"), s[105_000:105_010], s[len(s)-5:])
	s = fmt.Sprintf("%v|%d", []any{v, "x"}, 7)
	fmt.Println(len(s), s[len(s)-10:])
}
