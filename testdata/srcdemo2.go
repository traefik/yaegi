package main

import (
	"sort"
	"strconv"
	"strings"
)

func main() {
	out := ""
	for i, w := range strings.Fields("  hello  wide world ") {
		out += strings.ToUpper(w) + strconv.Itoa(i) + " "
	}
	println(out, strconv.Quote("a\tb"), strconv.FormatFloat(3.25, 'f', 2, 64))
	xs := []string{"c", "a", "b"}
	sort.Strings(xs)
	println(xs[0], xs[1], xs[2])
	n, err := strconv.Atoi("12x")
	println(n, err.Error())
}
