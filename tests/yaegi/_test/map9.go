package main

import (
	"fmt"
	"sort"
)

type Foo struct {
	Name string
}

func main() {
	m := map[string][]Foo{
		"hello": {{"foo"}, {"bar"}},
		"world": {{"truc"}, {"machin"}},
	}

	var content []string

	for key, values := range m {
		for _, value := range values {
			content = append(content, key+value.Name)
		}
	}

	sort.Strings(content)
	fmt.Println(content)
}

// Output:
// [hellobar hellofoo worldmachin worldtruc]
