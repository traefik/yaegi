package main

import "fmt"

func f1(in []string) (out []string) {
	defer copy(out, in)

	return make([]string, len(in))
}

func main() {
	out := f1([]string{"foo", "bar"})

	fmt.Println(out)
}

// Output:
// [foo bar]
