package main

import (
	"fmt"
	"sort"
)

type Foo struct {
	Name string
}

func main() {
	m := map[string]Foo{
		"hello": {Name: "bar"},
		"world": {Name: "machin"},
	}

	var content []string

	for key, value := range m {
		content = append(content, key+value.Name)
	}

	sort.Strings(content)
	fmt.Println(content)
}

// Output:
// [hellobar worldmachin]
