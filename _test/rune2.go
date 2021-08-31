package main

import "fmt"

const majorVersion = '2'

type hashed struct {
	major byte
}

func main() {
	fmt.Println(majorVersion)

	p := new(hashed)
	p.major = majorVersion

	fmt.Println(p)
}

// Output:
// 50
// &{50}
