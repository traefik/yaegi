package main

import "fmt"

func f1(in, out []string) {
	defer copy(out, in)
}

func main() {
	in := []string{"foo", "bar"}
	out := make([]string, 2)
	f1(in, out)

	fmt.Println(out)
}

// Output:
// [foo bar]
