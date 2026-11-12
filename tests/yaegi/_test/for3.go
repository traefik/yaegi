package main

import (
	"fmt"
	"sort"
)

func main() {
	m := map[string][]string{
		"hello": []string{"foo", "bar"},
		"world": []string{"truc", "machin"},
	}

	var content []string

	for key, values := range m {
		for _, value := range values {
			content = append(content, key+value)
		}
	}

	sort.Strings(content)
	fmt.Println(content)
}

// Output:
// [hellobar hellofoo worldmachin worldtruc]
