package main

import "fmt"

func getStr() string {
	return "test"
}

func main() {
	m := make(map[string]string, 0)
	m["a"] = fmt.Sprintf("%v", 0.1)
	m["b"] = string(fmt.Sprintf("%v", 0.1))
	m["c"] = getStr()

	fmt.Println(m)
}

// Output:
// map[a:0.1 b:0.1 c:test]
