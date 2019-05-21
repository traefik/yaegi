package main

import "fmt"

var m = map[string]float64{"foo": 1.0}

func f(s string) bool { return m[s] > 0.0 }

func main() {
	fmt.Println(f("foo"), f("bar"))
}

// Output:
// true false
