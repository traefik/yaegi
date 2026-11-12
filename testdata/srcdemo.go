package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type T struct{}

func (t *T) Error() string { return "T error" }

func main() {
	var b strings.Builder
	for i, w := range strings.Fields("  hello  wide world ") {
		b.WriteString(strings.ToUpper(w) + strconv.Itoa(i) + " ")
	}
	println(b.String(), strconv.Quote("a\tb"))

	xs := []string{"c", "a", "b"}
	sort.Strings(xs)
	println(xs[0], xs[1], xs[2])

	err := fmt.Errorf("wrap: %w", &T{})
	var target *T
	println(errors.As(err, &target), target != nil)
}
