package main

import "fmt"

func f() (host, port string, err error) {
	return "", "", nil
}

func main() {
	h, p, err := f()
	fmt.Println(h, p, err)
}

// Output:
//   <nil>
