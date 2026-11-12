package main

import "fmt"

func main() {
	users := make(map[string]string)

	v, exists := users["a"]
	fmt.Println(v, exists)

	if exists == true {
		//if exists {
		fmt.Println("exists", v)
	} else {
		fmt.Println("not exists")
	}
}

// Output:
//  false
// not exists
