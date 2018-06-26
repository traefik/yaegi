package main

import "fmt"

func main() {
	dict := map[string]string{"bidule": "machin", "truc": "bidule"}
	r, ok := dict["xxx"]
	fmt.Println(r, ok)
}

// Output:
//  false
